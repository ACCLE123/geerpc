package geerpc

import (
	"encoding/json"
	"fmt"
	"gee-rpc/codec"
	"io"
	"log"
	"net"
	"reflect"
	"sync"
)

type Server struct{}

type request struct {
	h            *codec.Header
	argv, result reflect.Value
}

func NewServer() *Server {
	return &Server{}
}

var DefaultServer = NewServer()

// server: listen("tcp", "8080")-->accept(net.listener)---->handle(net.Conn)

func (server *Server) Accept(lis net.Listener) {
	for {
		if conn, err := lis.Accept(); err != nil {
			log.Println("rpc server: accept error:", err)
			return
		} else {
			go server.handleConn(conn)
		}
	}
}

func (server *Server) handleConn(conn io.ReadWriteCloser) {
	defer conn.Close()
	var opt Option
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Println("rpc server: decode option error:", err)
		return
	}
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		log.Printf("rpc server: invalid codec type %s", opt.CodecType)
		return
	}
	server.handleCodec(f(conn), &opt)
}

func (server *Server) handleCodec(cc codec.Codec, opt *Option) {
	sending := &sync.Mutex{}
	wg := &sync.WaitGroup{}

	for {
		req, err := server.readRequest(cc)
		if err != nil {
			if req == nil {
				break
			}
			req.h.Error = err.Error()
			server.sendResponse(cc, req.h, struct{}{}, sending)
		}
		wg.Add(1)
		go server.handleRequest(cc, req, sending, wg)
	}
	wg.Wait()
	cc.Close()
}

func (server *Server) readRequestHeader(cc codec.Codec) (*codec.Header, error) {
	var header codec.Header
	cc.ReadHeader(&header)
	return &header, nil
}

func (server *Server) readRequest(cc codec.Codec) (*request, error) {
	header, err := server.readRequestHeader(cc)
	if err != nil {
		return nil, err
	}
	req := &request{h: header}
	req.argv = reflect.New(reflect.TypeOf(""))
	cc.ReadBody(req.argv.Interface())
	return req, nil
}

func (server *Server) sendResponse(cc codec.Codec, h *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	cc.Write(h, body)
}

func (server *Server) handleRequest(cc codec.Codec, req *request, sending *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()
	log.Println(req.h, req.argv.Elem())
	req.result = reflect.ValueOf(fmt.Sprintf("geerpc resp %d", req.h.Seq))
	server.sendResponse(cc, req.h, req.result.Interface(), sending)
}
