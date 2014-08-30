// Contains the implementation of a LSP client.

package lsp

import (
	"container/list"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cmu440/lspnet"
	//"strconv"
	"time"
)

type signalChan chan struct{}

type client struct {
	epochTimes         int
	epochLimit         int
	epochMillis        int
	windowSize         int
	connID             int
	goingToWriteSeqNum int
	successWriteSeqNum int
	successReadSeqNum  int
	hasBeenReadSeqNum  int
	conn               *lspnet.UDPConn
	addr               *lspnet.UDPAddr
	receiveWindow      map[int]bool
	sendWindow         map[int]bool
	sendQueue          *list.List
	receiveQueue       *list.List
	receiveChan        chan Message
	readMethodChan     chan Message
	sendChan           chan Message
	writeMethodChan    chan []byte
	epochChan          signalChan
	shutdownChan       signalChan
	successConnectChan signalChan
	closeEventChan     signalChan
	closeResponseChan  chan bool
	connectSuccess     bool
	alreadyShutDown    bool
	goingToClose       bool
	outReadMethodChan  chan Message
}

// NewClient creates, initiates, and returns a new client. This function
// should return after a connection with the server has been established
// (i.e., the client has received an Ack message from the server in response
// to its connection request), and should return a non-nil error if a
// connection could not be made (i.e., if after K epochs, the client still
// hasn't received an Ack message from the server in response to its K
// connection requests).
//
// hostport is a colon-separated string identifying the server's host address
// and port number (i.e., "localhost:9999").
func NewClient(hostport string, params *Params) (Client, error) {
	UDPAddr, err := lspnet.ResolveUDPAddr("udp", hostport)
	if err != nil {
		return nil, err
	}
	UDPConn, err := lspnet.DialUDP("udp", nil, UDPAddr)
	if err != nil {
		return nil, err
	}

	conn := UDPConn
	addr := UDPAddr
	receiveWindow := make(map[int]bool)
	sendWindow := make(map[int]bool)
	sendQueue := list.New()
	receiveQueue := list.New()
	receiveChan := make(chan Message)
	readMethodChan := make(chan Message)
	sendChan := make(chan Message)
	writeMethodChan := make(chan []byte)
	epochChan := make(signalChan)
	shutdownChan := make(signalChan)
	successConnectChan := make(signalChan)
	closeEventChan := make(signalChan)
	closeResponseChan := make(chan bool)
	outReadMethodChan := make(chan Message)

	c := &client{0, params.EpochLimit, params.EpochMillis, params.WindowSize, 0, 1, 0, 0, 0,
		conn, addr, receiveWindow, sendWindow, sendQueue, receiveQueue, receiveChan, readMethodChan, sendChan, writeMethodChan,
		epochChan, shutdownChan, successConnectChan, closeEventChan, closeResponseChan, false, false, false, outReadMethodChan}

	go c.ReadFromServer()
	go c.FireEpoch()
	go c.WriteToServer()
	go c.EventHandler()
	go c.HandleBuffer(c.readMethodChan, c.outReadMethodChan)

	msg := NewConnect()
	sendChan <- *msg

	for {
		select {
		case <-successConnectChan:
			c.connectSuccess = true
			return c, nil
		case <-shutdownChan:
			return nil, nil
		}
	}
}

func (c *client) HandleBuffer(in <-chan Message, out chan<- Message) {
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

func flush(buffer *list.List, out chan<- Message) {
	for e := buffer.Front(); e != nil; e = e.Next() {
		out <- (e.Value).(Message)
	}
}

func (c *client) EventHandler() {
	for {
		select {
		case buf := <-c.writeMethodChan:
			msg := NewData(c.connID, c.goingToWriteSeqNum, buf)
			c.sendQueue.PushBack(*msg)
			c.goingToWriteSeqNum++
			if len(c.sendWindow) < c.windowSize {
				c.sendChan <- *msg
				c.sendWindow[msg.SeqNum] = false
			}
		case msg := <-c.receiveChan:

			c.epochTimes = 0
			if msg.Type == MsgAck {
				if msg.SeqNum == 0 && !c.connectSuccess {
					c.connID = msg.ConnID
					c.successConnectChan <- struct{}{}
				} else if msg.SeqNum > c.successWriteSeqNum {
					c.sendWindow[msg.SeqNum] = true
					i := c.successWriteSeqNum + 1
					for c.sendWindow[i] {
						delete(c.sendWindow, i)
						c.sendQueue.Remove(c.sendQueue.Front())
						i++
					}
					c.successWriteSeqNum = i - 1
					j := c.successWriteSeqNum + len(c.sendWindow) + 1
					for ; j <= c.successWriteSeqNum+c.windowSize; j++ {
						if j-c.successWriteSeqNum <= c.sendQueue.Len() {
							for e := c.sendQueue.Front(); e != nil; e = e.Next() {
								if e.Value.(Message).SeqNum == j {
									c.sendWindow[j] = false
									c.sendChan <- e.Value.(Message)
									break
								}
							}
						} else {
							break
						}
					}
				}
			} else if msg.Type == MsgData {
				ack := NewAck(c.connID, msg.SeqNum)
				buf, _ := json.Marshal(*ack)
				c.conn.Write(buf)
				if received, ok := c.receiveWindow[msg.SeqNum]; !received || !ok {
					if msg.SeqNum > (c.successReadSeqNum + c.windowSize) {
						c.receiveQueue.PushBack(msg)
						c.receiveWindow[msg.SeqNum] = true
						for i := msg.SeqNum - 1; i > c.successReadSeqNum+c.windowSize; i-- {
							c.receiveWindow[i] = false
						}
						if msg.SeqNum == c.hasBeenReadSeqNum+1 {
							if c.readMethodChan != nil {
								c.receiveQueue.Remove(c.receiveQueue.Back())
								c.readMethodChan <- msg
								c.hasBeenReadSeqNum++
							}
						}
						for j := msg.SeqNum - c.windowSize; j > c.successReadSeqNum; j-- {
							delete(c.receiveWindow, j)
							for e := c.receiveQueue.Front(); e != nil; e = e.Next() {
								if e.Value.(Message).SeqNum == j {
									c.receiveQueue.Remove(e)
									break
								}
							}
						}
						c.successReadSeqNum = msg.SeqNum - c.windowSize
					} else if msg.SeqNum > c.successReadSeqNum {
						c.receiveQueue.PushBack(msg)
						c.receiveWindow[msg.SeqNum] = true
						for i := msg.SeqNum - 1; i > c.hasBeenReadSeqNum; i-- {
							if _, ok := c.receiveWindow[i]; !ok {
								c.receiveWindow[i] = false
							}
						}
						j := c.hasBeenReadSeqNum + 1
						for ; j <= c.successReadSeqNum+c.windowSize; j++ {
							if !c.receiveWindow[j] {
								break
							} else {
								for e := c.receiveQueue.Front(); e != nil; e = e.Next() {
									if e.Value.(Message).SeqNum == j {
										if c.readMethodChan != nil {
											c.readMethodChan <- e.Value.(Message)
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
		case <-c.epochChan:
			c.epochTimes++
			if len(c.sendWindow) == 0 && c.sendQueue.Len() == 0 && c.goingToClose {
				c.closeResponseChan <- true
			} else {
				if c.epochTimes >= c.epochLimit {
					if c.goingToClose {
						c.closeResponseChan <- false
					} else {
						c.ShutDownEverything()
					}
				} else {
					if c.connID == 0 {
						msg := NewConnect()
						c.sendChan <- *msg
					} else {
						if len(c.receiveWindow) == 0 {
							ack := NewAck(c.connID, 0)
							c.sendChan <- *ack
						}
						for i, boo := range c.receiveWindow {
							if boo {
								ack := NewAck(c.connID, i)
								buf, _ := json.Marshal(*ack)
								c.conn.Write(buf)
							}
						}
						for i, boo := range c.sendWindow {
							if !boo {
								for e := c.sendQueue.Front(); e != nil; e = e.Next() {
									if e.Value.(Message).SeqNum == i {
										c.sendChan <- e.Value.(Message)
										break
									}
								}
							}
						}
					}
				}
			}

		case <-c.closeEventChan:
			c.goingToClose = true
		case <-c.shutdownChan:
			return
		}
	}
}

func (c *client) ReadFromServer() error {
	var buf [1500]byte
	var msg Message
	for {
		n, err := c.conn.Read(buf[0:])
		if err != nil {
			if c.connectSuccess {
				if !c.alreadyShutDown && !c.goingToClose {
					c.ShutDownEverything()
					return nil
				} else {
					return nil
				}
			} else {
				continue
			}
		}
		json.Unmarshal(buf[0:n], &msg)
		if c.receiveChan != nil {
			c.receiveChan <- msg
		} else {
			break
		}
	}
	return nil
}

func (c *client) WriteToServer() {
	for {
		select {
		case message := <-c.sendChan:
			buf, err := json.Marshal(message)
			if err != nil {
				fmt.Println(err)
			}
			c.conn.Write(buf)
		case <-c.shutdownChan:
			return
		}
	}
}

func (c *client) FireEpoch() error {
	tick := time.Tick(time.Duration(c.epochMillis) * time.Millisecond)
	for {
		select {
		case <-tick:
			c.epochChan <- struct{}{}
		case <-c.shutdownChan:
			return nil
		}
	}
}

func (c *client) ConnID() int {
	return c.connID
}

func (c *client) Read() ([]byte, error) {
	msg, ok := <-c.outReadMethodChan
	if ok {
		if !c.alreadyShutDown {
			return msg.Payload, nil
		} else {
			return nil, errors.New("the connection is terminate")
		}
	} else {
		return nil, errors.New("the connection is terminate")
	}
}

func (c *client) Write(payload []byte) error {
	if !c.alreadyShutDown {
		c.writeMethodChan <- payload
		return nil
	} else {
		return errors.New("Client write error: connection closed")
	}
}

func (c *client) Close() error {
	if len(c.sendWindow) != 0 && c.sendQueue.Len() != 0 {
		c.closeEventChan <- struct{}{}
		select {
		case boo := <-c.closeResponseChan:
			if boo {
				c.ShutDownEverything()
				return nil
			} else {
				return errors.New("The connection is closed before all messages are sent out")
			}
		}
	} else {
		if !c.alreadyShutDown {
			c.ShutDownEverything()
		}
		return nil
	}
}

func (c *client) ShutDownEverything() {
	c.alreadyShutDown = true
	close(c.readMethodChan)
	close(c.shutdownChan)
	c.conn.Close()
	return
}
