package main

import (
	"fmt"
	"github.com/cmu440/bitcoin"
	"github.com/cmu440/lsp"
	"os"
  "strconv"
  "encoding/json"
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
  LOGF := log.New(file, "", log.Lshortfile|log.Lmicroseconds)
  	const numArgs = 4
  	if len(os.Args) != numArgs {
  		fmt.Println("Usage: ./client <hostport> <message> <maxNonce>")
  		return
  	}
  	hostport := os.Args[1]
  	data := os.Args[2]
  	maxNonceStr := os.Args[3]
  	params := lsp.NewParams()
    upper, _ := strconv.ParseUint(maxNonceStr, 10, 64)
  	request:= bitcoin.NewRequest(data, 0, upper)
  	c, newClientErr := lsp.NewClient(hostport, params)
    LOGF.Println("New Client")
    if newClientErr != nil {
      printDisconnected()
      return
    }
  	requestBuf, _ := json.Marshal(request)
  	writeErr := c.Write(requestBuf)
  	if writeErr != nil {
  		printDisconnected()
  		return
  	}
  	resultBuf, readErr := c.Read()
  	if readErr != nil {
  		printDisconnected()
      c.Close()
  		return
  	} else {
  		var result bitcoin.Message
  		json.Unmarshal(resultBuf[:], &result)
  		printResult(strconv.FormatUint(result.Hash, 10), strconv.FormatUint(result.Nonce, 10))
      c.Close()
  		return
  	}
}

// printResult prints the final result to stdout.
func printResult(hash, nonce string) {
	 fmt.Println("Result", hash, nonce)
}

// printDisconnected prints a disconnected message to stdout.
func printDisconnected() {
	 fmt.Println("Disconnected")
}
