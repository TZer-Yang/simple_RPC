package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"geerpc/codec"
	"geerpc/service"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"
)

const (
	CONNECTED = "200 Connected to Gee RPC"
	DEFAULT_RPC_PATH = "/_geerpc_"
	DEFAULT_DEBUG_PATH = "/debug/geerpc"
)

const debugText = `<html>
	<body>
	<title>GeeRPC Services</title>
	{{range .}}
	<hr>
	Service {{.Name}}
	<hr>
		<table>
		<th align=center>Method</th><th align=center>Calls</th>
		{{range $name, $mtype := .Method}}
			<tr>
			<td align=left font=fixed>{{$name}}({{$mtype.ArgType}}, {{$mtype.ReplyType}}) error</td>
			<td align=center>{{$mtype.NumCalls}}</td>
			</tr>
		{{end}}
		</table>
	{{end}}
	</body>
	</html>`

var debug = template.Must(template.New("RPC debug").Parse(debugText))

type DebugHTTP struct {
	*Server
}

type DebugService struct {
	Name   string
	Method map[string]*service.MethodType
}

// Runs at /debug/geerpc
func (server DebugHTTP) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Build a sorted version of the data.
	var services []DebugService
	server.ServiceMap.Range(func(namei, svci interface{}) bool {
		svc := svci.(*service.Service)
		services = append(services, DebugService{
			Name:   namei.(string),
			Method: svc.Method,
		})
		return true
	})
	err := debug.Execute(w, services)
	if err != nil {
		_, _ = fmt.Fprintln(w, "rpc: error executing template:", err.Error())
	}
}

type Server struct {
	ServiceMap sync.Map
}

type request struct {
	h *codec.Header
	args, reply reflect.Value
	svc *service.Service
	mtype *service.MethodType
}

func NewServer() *Server {
	return &Server{}
}

//这部分感觉上应该在另一台代理服务器
func (server *Server) ServeHTTP(w http.ResponseWriter, req *http.Request)  {
	if req.Method != "CONNECT" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = io.WriteString(w, "405 must CONNECT\n")
		return
	}

	//take over(掌管) 此connection
	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		log.Println("rpc server: hijack ", err)
		return
	}

	_, _ = io.WriteString(conn, "HTTP/1.0 " + CONNECTED + "\n\n")

	server.serveConn(conn)
}

func (server *Server) HandledHTTP() {
	http.Handle(DEFAULT_RPC_PATH, server)
	http.Handle(DEFAULT_DEBUG_PATH, DebugHTTP{server})
	log.Println("rpc server debug path:" + DEFAULT_DEBUG_PATH)
}

func (server *Server) AcceptConn(nl net.Listener) {
	for {
		conn, err := nl.Accept()
		if err != nil{
			log.Println("listener accept error:", err)
			break
		}
		go server.serveConn(conn)
	}
}

func (server *Server) serveConn(conn io.ReadWriteCloser) {
	defer func() {
		 _ = conn.Close()
	}()
	var opt Option
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Println("decode option error:", err)
		return
	}
	if opt.TypeNumber != GobTypeNumber{
		log.Println("option type-number error:", opt.TypeNumber)
		return
	}
	CodecConstructor := codec.CodecFuncMap[opt.CodecType]
	server.serverCodec(CodecConstructor(conn), opt.HandleTimeOut)
}

func (server *Server) serverCodec(cc codec.Codec, timeout time.Duration)  {
	sending := new(sync.Mutex)  //
	wg := new(sync.WaitGroup)
	for { // 一个conn可能有多个请求，请求持久化
		req, err := server.readRequest(cc)
		if err != nil{
			if req == nil {
				break
			}
			server.sendResponse(cc, req.h, fmt.Sprintf("read request error: %s", err.Error()), sending)
			continue
		}
		wg.Add(1)
		go server.handleRequest(cc, req, sending, wg, timeout)
	}
	wg.Wait()
}

func (server *Server) readRequest(cc codec.Codec) (*request, error)  {
	h, err := server.readRequestHeader(cc)
	if err != nil {
		log.Println("read request header error:", err)
		return nil, err
	}
	req := &request{h: h}
	if req.svc, req.mtype, err = server.findService(h.ServiceMethod); err != nil {
		return nil, err
	}
	req.args = req.mtype.NewArgv()
	req.reply = req.mtype.NewReplyv()

	var arggvi interface{}
	if req.args.Type().Kind() != reflect.Ptr {
		arggvi = req.args.Addr().Interface()
	} else {
		arggvi = req.args.Interface()
	}

	if err = cc.ReadBody(arggvi); err != nil {
		log.Println("rpc server: read body ", err)
		return nil, err
	}
	return req, nil
}

func (server *Server) readRequestHeader(cc codec.Codec) (*codec.Header, error) {
	var h codec.Header
	err := cc.ReadHeader(&h)
	return &h, err
}

func (server *Server) sendResponse(cc codec.Codec, h *codec.Header, body interface{},sending *sync.Mutex)  {
	sending.Lock()
	defer sending.Unlock()
	if err := cc.Write(h, body); err != nil {
		log.Println("send response error:", err)
	}
}

func (server *Server) handleRequest(cc codec.Codec, req *request, sending *sync.Mutex, wg *sync.WaitGroup, timeout time.Duration)  {
	defer wg.Done()
	log.Println("work for:", req.h, ", request Service:", req.svc.Name, ", method:", req.mtype.Method.Name)

	send := make(chan string)
	go func() {
		if err := req.svc.MethodCall(req.mtype, req.args, req.reply); err != nil {
			log.Println("rpc server: method call ", err)
			req.h.Error = err.Error()
			server.sendResponse(cc, req.h, fmt.Sprintf("rpc server: %s", err.Error()), sending)
			return
		}
		server.sendResponse(cc, req.h, req.reply.Interface(), sending)
		send <- "done"
	}()

	if timeout == 0 {
		send <- "done"
		return
	}

	select {
	case <-time.After(timeout):
		req.h.Error = "rpc server: handle and send request timeout"
	case <-send:

	}

}

func (server *Server) RegisterService(rcvr interface{}) error {
	ns := service.NewService(rcvr)
	if _, dup := server.ServiceMap.LoadOrStore(ns.Name, ns); dup {
		return errors.New("rpc server: service has already registered: " + ns.Name)
	}
	return nil
}

func (server * Server) findService(ServiceMethod string) (svc *service.Service, mtype *service.MethodType, err error) {
	DotIndex := strings.LastIndex(ServiceMethod, ".")
	if DotIndex < 0 {
		err = errors.New(fmt.Sprintf("illeage form of service method: %s", ServiceMethod))
		return nil, nil, err
	}
	ServiceName, MethodName := ServiceMethod[:DotIndex], ServiceMethod[DotIndex+1:]
	sv, ok := server.ServiceMap.Load(ServiceName)
	if !ok {
		err = errors.New("find service: "+ ServiceMethod +" error, no such service")
		return nil, nil, err
	}
	svc = sv.(*service.Service)
	mtype = svc.Method[MethodName]
	if mtype == nil {
		err = errors.New("can't find method: " + MethodName + ", no such method")
	}
	return
}

var DefaultServer = NewServer()

func Accept(nl net.Listener)  {
	DefaultServer.AcceptConn(nl)
}