package main

import (
	"fmt"
	geerpc "gee-rpc"
	"log"
	"net"
	"sync"
	"time"
)

func startServer(addr chan string) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	addr <- l.Addr().String()
	geerpc.DefaultServer.Accept(l)
}

func main() {
	addr := make(chan string)
	go startServer(addr)
	client, _ := geerpc.Dial("tcp", <-addr)

	time.Sleep(time.Second)

	wg := &sync.WaitGroup{}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go handle(client, wg)
	}
	wg.Wait()
}

func handle(client *geerpc.Client, wg *sync.WaitGroup) {
	defer wg.Done()
	argv := fmt.Sprintf("hello world %d", 1)
	var reply string
	if err := client.Call("Foo.Sum", argv, &reply); err != nil {
		panic(err)
	}
	log.Println("reply: ", reply)
}
