package main

import (
	"fmt"
    "github.com/cmu440/bitcoin"
    "github.com/cmu440/lsp"
    "os"
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
	const numArgs = 2
	if len(os.Args) != numArgs {
		fmt.Println("Usage: ./miner <hostport>")
		return
	}
	LOGF := log.New(file, "", log.Lshortfile|log.Lmicroseconds)
	hostport := os.Args[1]
	params := lsp.NewParams()
	m, _ := lsp.NewClient(hostport, params)
	join := bitcoin.NewJoin()
	joinBuf, _ := json.Marshal(join)
	joinWriteErr := m.Write(joinBuf)
	LOGF.Println("New Miner")
	if joinWriteErr != nil {
		return
	}
	var request bitcoin.Message
	for {
		requestBuf, readErr := m.Read()
		if readErr != nil {
			m.Close()
			return
		}
		json.Unmarshal(requestBuf[:], &request)
		data := request.Data
		lower := request.Lower
		upper := request.Upper
		curr := bitcoin.Hash(data, lower)
		min := curr
		minNonce := lower
		// LOGF.Print("Min: ")
		// LOGF.Println(min)
		for i := lower + 1; i <= upper; i++ {
			curr = bitcoin.Hash(data, i)
			// LOGF.Print("Curr: ")
			// LOGF.Println(curr)
			if curr < min {
				min = curr
				minNonce = i			
				// LOGF.Print("New Min: ")
				// LOGF.Println(min)
				// LOGF.Print("New Min Nonce: ")
				// LOGF.Println(minNonce)
			}
		}
		result := bitcoin.NewResult(min, minNonce)
        resultBuf, _ := json.Marshal(result)
        LOGF.Println(result.String())
		resultWriteErr := m.Write(resultBuf)
		if resultWriteErr != nil {
			m.Close()
			return
		}
	}
}
