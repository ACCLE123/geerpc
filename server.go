package geerpc

import (
	"encoding/json"
	"errors"
	"gee-rpc/codec"
	"io"
	"log"
	"net"
	"reflect"
	"strings"
	"sync"
)

type Server struct {
	serviceMap sync.Map
}

type request struct {
	h            *codec.Header
	argv, result reflect.Value
	mtype        *methodType
	svc          *service
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
	req.svc, req.mtype, err = server.findService(header.ServiceMethod)
	req.argv = req.mtype.newArgv()
	req.result = req.mtype.newReplyv()

	argvi := req.argv.Interface()
	if req.argv.Type().Kind() != reflect.Ptr {
		argvi = req.argv.Addr().Interface()
	}

	if err := cc.ReadBody(argvi); err != nil {
		log.Println("rpc server: read body err:", err)
		return req, err
	}
	//log.Println("argvi: ", argvi)
	//log.Println("argv :", req.argv.Interface())

	return req, nil
}

func (server *Server) sendResponse(cc codec.Codec, h *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	cc.Write(h, body)
}

func (server *Server) handleRequest(cc codec.Codec, req *request, sending *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()
	err := req.svc.call(req.mtype, req.argv, req.result)

	if err != nil {
		req.h.Error = err.Error()
		server.sendResponse(cc, req.h, struct{}{}, sending)
		return
	}
	server.sendResponse(cc, req.h, req.result.Interface(), sending)
}

func (server *Server) Register(rcvr interface{}) error {
	s := newService(rcvr)
	if _, dup := server.serviceMap.LoadOrStore(s.name, s); dup {
		return errors.New("rpc: service already defined: " + s.name)
	}
	return nil
}

func (server *Server) findService(serviceMethod string) (svc *service, mtype *methodType, err error) {
	dot := strings.LastIndex(serviceMethod, ".")
	if dot < 0 {
		err = errors.New("rpc server: service/method request ill-formed: " + serviceMethod)
		return
	}
	serviceName, methodName := serviceMethod[:dot], serviceMethod[dot+1:]
	svci, ok := server.serviceMap.Load(serviceName)
	if !ok {
		err = errors.New("rpc server: service/method request ill-formed: " + serviceMethod)
		return
	}
	svc = svci.(*service)
	mtype = svc.method[methodName]
	if mtype == nil {
		err = errors.New("rpc server: service/method request ill-formed: " + serviceMethod)
	}
	return
}
