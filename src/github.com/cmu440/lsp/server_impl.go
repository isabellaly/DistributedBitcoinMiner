// Contains the implementation of a LSP server.

package lsp

import (
	"container/list"
	"encoding/json"
	"errors"
	"github.com/cmu440/lspnet"
	"strconv"
	"time"
)

type clientBool struct {
	connID  int
	flushed bool
}

type clientInfo struct {
	clientAddr            *lspnet.UDPAddr
	connID                int
	epoch                 int
	successReadSeqNum     int
	successWriteSeqNum    int
	goingToWriteSeqNum    int
	hasBeenReadSeqNum     int
	sendQueue             *list.List
	receiveQueue          *list.List
	sendWindow            map[int]bool
	receiveWindow         map[int]bool
	sendChan              chan Message
	receiveChan           chan Message
	writeMethodChan       chan Message
	clientCloseChan       signalChan
	clientShutDownChan    signalChan
	clientEpochChan       signalChan
	clientGointToClose    bool
	clientAlreadyShutDown bool
}

type server struct {
	conn                        *lspnet.UDPConn
	connIDs                     int
	epochLimit                  int
	epochMillis                 int
	windowSize                  int
	clients                     map[int]*clientInfo
	readMethodChan              chan Message
	clientConnectChan           chan *lspnet.UDPAddr
	clientAlreadyShutDownChan   signalChan
	serverCloseChan             signalChan
	serverShutDownChan          signalChan
	serverCloseResponseChan     chan bool
	allServerCloseResponseChan  chan bool
	serverWriteChan             chan Message
	serverWriteResponseChan     chan bool
	serverCloseConnChan         chan int
	serverCloseConnResponseChan chan bool
	serverGoingToClose          bool
	serverAlreadyShutDown       bool
	outReadMethodChan           chan Message
	deleteMap                   chan int
}

func NewServer(port int, params *Params) (Server, error) {
	addr, err := lspnet.ResolveUDPAddr("udp", lspnet.JoinHostPort("localhost", strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	conn, err := lspnet.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}
	clients := make(map[int]*clientInfo)
	readMethodChan := make(chan Message)
	clientConnectChan := make(chan *lspnet.UDPAddr)
	clientAlreadyShutDownChan := make(signalChan)
	serverCloseChan := make(signalChan)
	serverShutDownChan := make(signalChan)
	serverWriteChan := make(chan Message)
	serverWriteResponseChan := make(chan bool)
	serverCloseConnChan := make(chan int)
	serverCloseConnResponseChan := make(chan bool)
	outReadMethodChan := make(chan Message)
	deleteMap := make(chan int)
	s := &server{conn, 1, params.EpochLimit, params.EpochMillis, params.WindowSize, clients, readMethodChan, clientConnectChan, clientAlreadyShutDownChan, serverCloseChan, serverShutDownChan, nil, nil, serverWriteChan, serverWriteResponseChan, serverCloseConnChan, serverCloseConnResponseChan, false, false, outReadMethodChan, deleteMap}

	go s.ServerEventHandler()
	go s.ReadFromAllClients()
	go s.handleBuffer(readMethodChan, outReadMethodChan)
	return s, nil
}

func (s *server) handleBuffer(in <-chan Message, out chan<- Message) {
	defer close(out)
	buffer := list.New()
	for {
		if buffer.Len() == 0 {
			v, ok := <-in
			if !ok {
				flush(buffer, out)
				return
			}
			buffer.PushBack(v)
		}
		select {
		case v, ok := <-in:
			if !ok {
				flush(buffer, out)
				return
			}
			buffer.PushBack(v)
		case out <- (buffer.Front().Value).(Message):
			buffer.Remove(buffer.Front())
		}
	}
}

func (c *clientInfo) FireEpoch(s *server) {
	tick := time.Tick(time.Duration(s.epochMillis) * time.Millisecond)
	for {
		select {
		case <-tick:
			c.clientEpochChan <- struct{}{}
		case <-c.clientShutDownChan:
			return
		}
	}
}

func (s *server) ReadFromAllClients() {
	var buf [1500]byte
	var msg Message
	for {
		n, addr, err := s.conn.ReadFromUDP(buf[0:])
		if err != nil {
			return
		}
		json.Unmarshal(buf[0:n], &msg)
		if msg.Type == MsgConnect {
			if !s.serverGoingToClose {
				s.clientConnectChan <- addr
			}
		} else {
			for _, clientInfo := range s.clients {
				if clientInfo.connID == msg.ConnID {
					clientInfo.receiveChan <- msg
					break
				}
			}
		}
	}
}

func (s *server) ServerEventHandler() {
	for {
		select {
		case addr := <-s.clientConnectChan:
			sendQueue := list.New()
			receiveQueue := list.New()
			sendWindow := make(map[int]bool)
			receiveWindow := make(map[int]bool)
			sendChan := make(chan Message)
			receiveChan := make(chan Message)
			writeMethodChan := make(chan Message)
			clientCloseChan := make(signalChan)
			clientShutDownChan := make(signalChan)
			clientEpochChan := make(signalChan)
			client := &clientInfo{addr, s.connIDs, 0, 0, 0, 1, 0, sendQueue, receiveQueue, sendWindow, receiveWindow, sendChan,
				receiveChan, writeMethodChan, clientCloseChan, clientShutDownChan, clientEpochChan, false, false}
			s.clients[s.connIDs] = client
			s.connIDs++
			go client.ClientEventHandler(s)
			go client.FireEpoch(s)
			msgConnect := NewAck(client.connID, 0)
			buf, _ := json.Marshal(*msgConnect)
			s.conn.WriteToUDP(buf, addr)
		case <-s.serverCloseChan:
			s.serverGoingToClose = true
			for _, c := range s.clients {
				c.clientCloseChan <- struct{}{}
			}
			allFlushed := true
			clientsLen := len(s.clients)
			for clientsLen > 0 {
				flushed, ok := <-s.serverCloseResponseChan
				if !ok {
					break
				}
				clientsLen--
				if !flushed {
					allFlushed = false
				}
			}
			s.allServerCloseResponseChan <- allFlushed
		case msg := <-s.serverWriteChan:
			if _, ok := s.clients[msg.ConnID]; ok {
				s.serverWriteResponseChan <- true
				s.clients[msg.ConnID].sendChan <- msg
			} else {
				s.serverWriteResponseChan <- false
			}
		case id := <-s.deleteMap:
			delete(s.clients, id)
		case connID := <-s.serverCloseConnChan:
			if c, ok := s.clients[connID]; ok {
				s.serverCloseConnResponseChan <- true
				delete(s.clients, c.connID)
				c.clientCloseChan <- struct{}{}
			} else {
				s.serverCloseConnResponseChan <- false
			}
		case <-s.serverShutDownChan:
			return
		}
	}
}

func (c *clientInfo) ClientEventHandler(s *server) {
	for {
		select {
		case sendMsg := <-c.sendChan:
			if sendMsg.Type == MsgAck {
				buf, _ := json.Marshal(sendMsg)
				s.conn.WriteToUDP(buf, c.clientAddr)
			} else {
				if sendMsg.SeqNum == -1 {
					sendMsg.SeqNum = c.goingToWriteSeqNum
					c.goingToWriteSeqNum++
				}
				c.sendQueue.PushBack(sendMsg)
				if len(c.sendWindow) < s.windowSize {
					buf, _ := json.Marshal(sendMsg)
					s.conn.WriteToUDP(buf, c.clientAddr)
					c.sendWindow[sendMsg.SeqNum] = false
				}
			}
		case msg := <-c.receiveChan:
			c.epoch = 0
			if msg.Type == MsgAck {
				if msg.SeqNum > c.successWriteSeqNum {
					c.sendWindow[msg.SeqNum] = true
					i := c.successWriteSeqNum + 1
					for c.sendWindow[i] {
						delete(c.sendWindow, i)
						c.sendQueue.Remove(c.sendQueue.Front())
						i++
					}
					c.successWriteSeqNum = i - 1
					j := c.successWriteSeqNum + len(c.sendWindow) + 1
					for ; j <= c.successWriteSeqNum+s.windowSize; j++ {
						if j-c.successWriteSeqNum <= c.sendQueue.Len() {
							for e := c.sendQueue.Front(); e != nil; e = e.Next() {
								if e.Value.(Message).SeqNum == j {
									//important update!!
									c.sendWindow[j] = false
									buf, _ := json.Marshal(e.Value.(Message))
									s.conn.WriteToUDP(buf, c.clientAddr)
									break
								}
							}
						} else {
							break
						}
					}

				}
			} else {
				ack := NewAck(c.connID, msg.SeqNum)
				buf, _ := json.Marshal(*ack)
				s.conn.WriteToUDP(buf, c.clientAddr)
				if receive, ok := c.receiveWindow[msg.SeqNum]; !receive || !ok {
					if msg.SeqNum > (c.successReadSeqNum + s.windowSize) {
						c.receiveQueue.PushBack(msg)
						c.receiveWindow[msg.SeqNum] = true
						for i := msg.SeqNum - 1; i > c.successReadSeqNum+s.windowSize; i-- {
							c.receiveWindow[i] = false
						}
						if msg.SeqNum == c.hasBeenReadSeqNum+1 {
							if s.readMethodChan != nil && !s.serverGoingToClose {
								c.receiveQueue.Remove(c.receiveQueue.Back())
								s.readMethodChan <- msg
								c.hasBeenReadSeqNum++
							}
						}
						for j := msg.SeqNum - s.windowSize; j > c.successReadSeqNum; j-- {
							delete(c.receiveWindow, j)
							for e := c.receiveQueue.Front(); e != nil; e = e.Next() {
								if e.Value.(Message).SeqNum == j {
									c.receiveQueue.Remove(e)
									break
								}
							}
						}
						c.successReadSeqNum = msg.SeqNum - s.windowSize
					} else if msg.SeqNum > c.successReadSeqNum {
						c.receiveQueue.PushBack(msg)
						c.receiveWindow[msg.SeqNum] = true
						for i := msg.SeqNum - 1; i > c.hasBeenReadSeqNum; i-- {
							if _, ok := c.receiveWindow[i]; !ok {
								c.receiveWindow[i] = false
							}
						}
						j := c.hasBeenReadSeqNum + 1
						for ; j <= c.successReadSeqNum+s.windowSize; j++ {
							if !c.receiveWindow[j] {
								break
							} else {
								for e := c.receiveQueue.Front(); e != nil; e = e.Next() {
									if e.Value.(Message).SeqNum == j {
										if s.readMethodChan != nil && !s.serverGoingToClose {
											s.readMethodChan <- e.Value.(Message)
											c.receiveQueue.Remove(e)
										}
										break
									}
								}
							}
						}
						c.hasBeenReadSeqNum = j - 1
					}
				}
			}
		case <-c.clientEpochChan:
			if len(c.sendWindow) == 0 && c.sendQueue.Len() == 0 && c.clientGointToClose {
				if s.serverGoingToClose {
					s.serverCloseResponseChan <- true
				}
				c.ClientShutDownEverything(s)
			} else {
				c.epoch++
				if c.epoch >= s.epochLimit {
					if s.serverGoingToClose {
						s.serverCloseResponseChan <- false
					}
					c.ClientShutDownEverything(s)
				} else {
					if len(c.receiveWindow) == 0 && c.successReadSeqNum == 0 && c.hasBeenReadSeqNum == 0 {
						msgConnect := NewAck(c.connID, 0)
						buf, _ := json.Marshal(*msgConnect)
						s.conn.WriteToUDP(buf, c.clientAddr)
					}
					for i, boo := range c.sendWindow {
						if !boo {
							for e := c.sendQueue.Front(); e != nil; e = e.Next() {
								if e.Value.(Message).SeqNum == i {
									buf, _ := json.Marshal(e.Value.(Message))
									s.conn.WriteToUDP(buf, c.clientAddr)
									break
								}
							}
						}
					}
					for i, boo := range c.receiveWindow {
						if boo {
							msgAck := NewAck(c.connID, i)
							buf, _ := json.Marshal(*msgAck)
							s.conn.WriteToUDP(buf, c.clientAddr)
						}
					}
				}
			}
		case <-c.clientCloseChan:
			c.clientGointToClose = true
		case <-c.clientShutDownChan:
			return
		}
	}
}

func (s *server) Read() (int, []byte, error) {
	select {
	case msg := <-s.outReadMethodChan:
		return msg.ConnID, msg.Payload, nil
	case <-s.clientAlreadyShutDownChan:
		return -1, nil, errors.New("One client closed")
	case <-s.serverShutDownChan:
		return -1, nil, errors.New("The connID doesn't exist")
	}
}

func (s *server) Write(connID int, payload []byte) error {
	msg := NewData(connID, -1, payload)
	s.serverWriteChan <- *msg
	select {
	case ok := <-s.serverWriteResponseChan:
		if ok {
			return nil
		} else {
			return errors.New("The connID doesn't exist")
		}
	case <-s.serverShutDownChan:
		return nil
	}
}

func (s *server) CloseConn(connID int) error {
	s.serverCloseConnChan <- connID
	select {
	case ok := <-s.serverCloseConnResponseChan:
		if ok {
			return nil
		} else {
			return errors.New("The connID doesn't exist")
		}
	case <-s.serverShutDownChan:
		return nil
	}
}

func (s *server) Close() error {
	serverCloseResponseChan := make(chan bool)
	s.serverCloseResponseChan = serverCloseResponseChan
	allServerCloseResponseChan := make(chan bool)
	s.allServerCloseResponseChan = allServerCloseResponseChan
	s.serverCloseChan <- struct{}{}
	allFlushed := <-s.allServerCloseResponseChan
	close(s.serverShutDownChan)
	s.conn.Close()
	close(s.readMethodChan)
	s.serverAlreadyShutDown = true
	if allFlushed {
		return nil
	} else {
		return errors.New("Not all clients are flushed")
	}
}

func (c *clientInfo) ClientShutDownEverything(s *server) {
	c.clientAlreadyShutDown = true
	close(c.clientShutDownChan)
	s.deleteMap <- c.connID
	s.clientAlreadyShutDownChan <- struct{}{}
}
