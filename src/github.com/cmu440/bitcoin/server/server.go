package main

import (
	"fmt"
    "github.com/cmu440/bitcoin"
    "github.com/cmu440/lsp"
    "os"
    "strconv"
    "encoding/json"
	"container/list"
	"log"
)

const (
    name = "log.txt"
    flag = os.O_RDWR | os.O_CREATE 
    perm = os.FileMode(0666)
)

func main() {

	file, err := os.OpenFile(name, flag, perm)
  	if err != nil {
        return
  	}
	const numArgs = 2
	if len(os.Args) != numArgs {
		fmt.Println("Usage: ./server <port>")
		return
	}
	LOGF := log.New(file, "", log.Lshortfile|log.Lmicroseconds)
	LOGF.Println("New Server")
	port, _ := strconv.Atoi(os.Args[1])
	s, _ := lsp.NewServer(port, lsp.NewParams())
	miners := make(map[int]bool)
	clients := make(map[int]bool)
	requestsQueue := list.New()
	requestsClients := make(map[bitcoin.Message]int)
	minersRequests := make(map[int]bitcoin.Message)
	inProcessRequests := make(map[bitcoin.Message]bool)
	var message bitcoin.Message
	for {
		connID, messageBuf, readErr := s.Read()
		LOGF.Println("Server read: "+strconv.Itoa(connID))
		if readErr != nil {
			if _, ok := clients[connID]; ok {
				delete(clients, connID)
				for e := requestsQueue.Front(); e != nil; e = e.Next() {
					if clientID := requestsClients[e.Value.(bitcoin.Message)]; clientID == connID {
						delete(requestsClients, e.Value.(bitcoin.Message))
						requestsQueue.Remove(e)
					}
				}
			} else if _, ok := miners[connID]; ok {
				LOGF.Println("Delete miner: "+strconv.Itoa(connID))
				delete(miners, connID)
				resendRequest := minersRequests[connID]
				LOGF.Println("Resend request: "+resendRequest.String())
				delete(minersRequests, connID)
				delete(inProcessRequests, resendRequest)
				for k, v := range miners {
				    LOGF.Print(k)
					LOGF.Println(v)
				}
				for minerID, occupied := range miners {
					if !occupied {
						resendRequestBuf, _:= json.Marshal(resendRequest)
						s.Write(minerID, resendRequestBuf)
						LOGF.Println("Give miner: "+strconv.Itoa(minerID))
						LOGF.Println(resendRequest.Data)
						miners[minerID] = true
						minersRequests[minerID] = resendRequest
						inProcessRequests[resendRequest]=true
						break
					}
				}
			}
		} else {
			json.Unmarshal(messageBuf[:], &message)
			//LOGF.Println("Get message: "+ message.String())
			if message.Type == bitcoin.Join {
				miners[connID] = false
				LOGF.Println("Miner join: "+strconv.Itoa(connID))
				for minerID, occupied := range miners {
					//LOGF.Println("find miner: "+strconv.Itoa(minerID))
					if !occupied {
						for e := requestsQueue.Front(); e != nil; e = e.Next() {
							if _, ok := inProcessRequests[e.Value.(bitcoin.Message)]; !ok {
								requestBuf, _:= json.Marshal(e.Value.(bitcoin.Message))
								s.Write(minerID, requestBuf)
								//LOGF.Println("Give miner: "+strconv.Itoa(minerID))
								//LOGF.Println(e.Value.(bitcoin.Message).Data)
								miners[minerID] = true
								minersRequests[minerID] = e.Value.(bitcoin.Message)
								inProcessRequests[e.Value.(bitcoin.Message)]=true
								break
							}
						}
					}
				}
			} else if message.Type == bitcoin.Request {
				LOGF.Println("Get reqeust: "+message.String())
				clients[connID] = true
				requestsClients[message] = connID
				requestsQueue.PushBack(message)
				for minerID, occupied := range miners {
					if !occupied {
						for e := requestsQueue.Front(); e != nil; e = e.Next() {
							if _, ok := inProcessRequests[e.Value.(bitcoin.Message)]; !ok {
								requestBuf, _:= json.Marshal(e.Value.(bitcoin.Message))
								s.Write(minerID, requestBuf)
								//LOGF.Println("Give miner: "+strconv.Itoa(minerID))
								//LOGF.Println(e.Value.(bitcoin.Message).Data)
								miners[minerID] = true
								minersRequests[minerID] = e.Value.(bitcoin.Message)
								inProcessRequests[e.Value.(bitcoin.Message)]=true
								break
							}
						}
					}
				}
			} else if message.Type == bitcoin.Result {
				LOGF.Println("Get result: "+strconv.Itoa(connID)+" "+message.String())
				request := minersRequests[connID]
				clientID := requestsClients[request]
				LOGF.Println("From request: "+request.String())
				miners[connID] = false
				delete(inProcessRequests, request)
				delete(minersRequests, connID)
				delete(requestsClients, request)
				// LOGF.Println("Print inProcessRequests")
				// for k, v := range inProcessRequests {
				// 	LOGF.Print(k)
				// 	LOGF.Print(" ")
				// 	LOGF.Println(v)
				// }
				// LOGF.Println("Print minersRequests")
				// for k, v := range minersRequests {
				// 	LOGF.Print(k)
				// 	LOGF.Print(" ")
				// 	LOGF.Println(v)
				// }
				// LOGF.Println("Print requestsClients")
				// for k, v := range requestsClients {
				// 	LOGF.Print(k)
				// 	LOGF.Print(" ")
				// 	LOGF.Println(v)
				// }
				for e := requestsQueue.Front(); e != nil; e = e.Next() {
						if e.Value.(bitcoin.Message).Data == request.Data { 
							requestsQueue.Remove(e)
							break
						}
					}
				if _, ok := clients[clientID]; ok {
					resultBuf, _:= json.Marshal(message)
					s.Write(clientID, resultBuf)
					delete(clients, clientID)
					//s.CloseConn(clientID)
				}
				for e := requestsQueue.Front(); e != nil; e = e.Next() {
					if _, ok := inProcessRequests[e.Value.(bitcoin.Message)]; !ok {
						requestBuf, _:= json.Marshal(e.Value.(bitcoin.Message))
						s.Write(connID, requestBuf)
						miners[connID] = true
						minersRequests[connID] = e.Value.(bitcoin.Message)
						inProcessRequests[e.Value.(bitcoin.Message)]=true
						break
					}
				}
			}
		}
	}
}

	
