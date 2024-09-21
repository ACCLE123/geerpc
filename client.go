package geerpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"gee-rpc/codec"
	"io"
	"log"
	"net"
	"sync"
)

type Call struct {
	Seq           uint64
	ServiceMethod string
	Argv          interface{}
	Result        interface{}
	Error         error
	Done          chan *Call
}

func (call *Call) done() {
	call.Done <- call
}

type Client struct {
	cc       codec.Codec
	opt      *Option
	sending  sync.Mutex
	header   codec.Header
	mu       sync.Mutex
	seq      uint64
	pending  map[uint64]*Call
	closing  bool
	shutdown bool
}

var ErrShutdown = errors.New("connection is shut down")

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closing {
		return ErrShutdown
	}
	c.closing = true
	return c.cc.Close()
}

func (c *Client) IsAvailable() bool {
	// 不可重入锁导致死锁
	//c.mu.Lock()
	//defer c.mu.Unlock()
	return !c.closing && !c.shutdown
}

var _ io.Closer = (*Client)(nil)

func (c *Client) addCall(call *Call) (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.IsAvailable() {
		return 0, ErrShutdown
	}
	call.Seq = c.seq
	c.pending[call.Seq] = call
	c.seq++
	return call.Seq, nil
}

func (c *Client) removeCall(seq uint64) *Call {
	c.mu.Lock()
	defer c.mu.Unlock()
	call := c.pending[seq]
	delete(c.pending, seq)
	return call
}

func (c *Client) terminateCalls(err error) {
	c.sending.Lock()
	defer c.sending.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, call := range c.pending {
		call.Error = err
		call.done()
	}
}

func (c *Client) receive() {
	var err error
	for err == nil {
		var h codec.Header

		if err = c.cc.ReadHeader(&h); err != nil {
			log.Println(err)
			break
		}
		call := c.removeCall(h.Seq)
		switch {
		case call == nil:
			err = c.cc.ReadBody(nil)
		case h.Error != "":
			call.Error = fmt.Errorf(h.Error)
			err = c.cc.ReadBody(nil)
			call.done()
		default:
			err = c.cc.ReadBody(call.Result)
			if err != nil {
				call.Error = errors.New("reading body " + err.Error())
			}
			call.done()
		}
	}
	c.terminateCalls(err)
}

func NewClient(conn net.Conn, opt *Option) (*Client, error) {
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		err := fmt.Errorf("invalid codec type %s", opt.CodecType)
		log.Println("rpc client: codec error:", err)
		return nil, err
	}
	if err := json.NewEncoder(conn).Encode(opt); err != nil {
		log.Println("rpc client: options error: ", err)
		_ = conn.Close()
		return nil, err
	}
	return newClientCodec(f(conn), opt), nil
}

func newClientCodec(cc codec.Codec, opt *Option) *Client {
	client := &Client{
		seq:     1,
		cc:      cc,
		opt:     opt,
		pending: make(map[uint64]*Call),
	}
	go client.receive()
	return client
}

func Dial(network, address string, sets ...SetOption) (client *Client, err error) {
	for _, set := range sets {
		set(DefaultOption)
	}
	// client: dial->conn
	conn, err := net.Dial(network, address)
	if err != nil {
		return nil, err
	}

	defer func() {
		if client == nil {
			conn.Close()
		}
	}()

	return NewClient(conn, DefaultOption)
}

func (c *Client) send(call *Call) {
	c.sending.Lock()
	defer c.sending.Unlock()

	seq, err := c.addCall(call)
	if err != nil {
		call.Error = err
		call.done()
		return
	}

	c.header = codec.Header{
		ServiceMethod: call.ServiceMethod,
		Seq:           seq,
	}

	if err := c.cc.Write(&c.header, call.Argv); err != nil {
		call := c.removeCall(seq)
		if call != nil {
			call.Error = err
			call.done()
		}
	}
}

func (c *Client) Go(serviceMethod string, argv, result interface{}, done chan *Call) *Call {
	if done == nil {
		done = make(chan *Call, 10)
	} else if cap(done) == 0 {
		log.Panic("rpc client: done channel is unbuffered")
	}

	call := &Call{
		ServiceMethod: serviceMethod,
		Argv:          argv,
		Result:        result,
		Done:          done,
	}
	c.send(call)
	return call
}

func (c *Client) Call(serviceMethod string, argv, result interface{}) error {
	call := <-c.Go(serviceMethod, argv, result, make(chan *Call, 1)).Done
	return call.Error
}
