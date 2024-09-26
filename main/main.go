package main

import (
	geerpc "gee-rpc"
	"log"
	"net"
	"sync"
	"time"
)

type Foo int

type Args struct {
	Num1 int
	Num2 int
}

func (f Foo) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

func startServer(addr chan string) {
	var foo Foo
	geerpc.DefaultServer.Register(&foo)
	l, _ := net.Listen("tcp", ":0")
	addr <- l.Addr().String()
	geerpc.DefaultServer.Accept(l)
}

func main() {
	addr := make(chan string)
	go startServer(addr)
	c, _ := geerpc.Dial("tcp", <-addr)
	defer c.Close()

	time.Sleep(time.Second)

	var wg sync.WaitGroup

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := &Args{Num1: i, Num2: i * i}
			var reply int
			c.Call("Foo.Sum", args, &reply)
			log.Printf("%d + %d = %d", args.Num1, args.Num2, reply)
		}(i)
	}
	wg.Wait()
}
