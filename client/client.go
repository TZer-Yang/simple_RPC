package client

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"geerpc/codec"
	"geerpc/server"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"
)

// constant
var CLIENT_CONNECTION_CLOSED = errors.New("Client connection was closed")

const (
	CONNECTED = "200 Connected to Gee RPC"
	DEFAULT_RPC_PATH = "/_geerpc_"
	DEFAULT_DEBUG_PATH = "/debug/geerpc"
)

// interface assert
var _ io.Closer = (*Client)(nil)

type ClientResult struct {
	client *Client
	Err error
}

// struct
type Call struct {
	Sqe uint64
	ServerMethod string
	Args interface{}
	Reply interface{}
	Error error
	Done chan *Call
}

func NewCall(ServerMethod string, args interface{}, reply interface{}, buf uint) (*Call) {
	return &Call{
		ServerMethod: ServerMethod,
		Args: args,
		Reply: reply,
		Done: make(chan *Call, buf),
	}
}

func (c *Call) done()  {
	c.Done <- c
}

type Client struct {
	CC codec.Codec
	Opt server.Option
	Sending sync.Mutex
	Mu sync.Mutex
	Sqe uint64
	Pending map[uint64]*Call
	Closed bool
	ShutDown bool
}

func NewClient(conn net.Conn, opt server.Option) (*Client, error) {
	f := codec.CodecFuncMap[opt.CodecType]
	if f == nil {
		return nil, errors.New("Invalid codec type ")
	}
	if err := json.NewEncoder(conn).Encode(&opt); err != nil {
		_ = conn.Close()
		return nil, err
	}
	client := &Client{
		CC: f(conn),
		Opt: opt,
		Sqe: uint64(1),
		Pending: make(map[uint64]*Call),
	}
	go client.receive()
	return client, nil
}

func NewHTTPClient(conn net.Conn, opt server.Option) (*Client, error) {
	_, _ = io.WriteString(conn, fmt.Sprintf("CONNECT %s HTTP/1.0\n\n", DEFAULT_RPC_PATH))

	req, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	if err != nil {
		return nil, err
	}

	if req.Status == CONNECTED {
		return NewClient(conn, opt)
	} else {
		return nil, errors.New("client http request status:" + req.Status)
	}
}

func (c *Client) Close() error {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	if c.Closed {
		return CLIENT_CONNECTION_CLOSED
	}
	c.Closed = true
	return c.CC.Close()
}

func (c *Client) IsAvailable() bool {
	return !c.Closed && !c.ShutDown
}

func (c *Client) registerCall(call *Call) (uint64, error) {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	if c.Closed || c.ShutDown {
		return 0, errors.New("register call fail, client closed or client shutdown")
	}
	call.Sqe = c.Sqe
	c.Pending[call.Sqe] = call
	c.Sqe ++
	return call.Sqe, nil
}

func (c *Client) removeCall(seq uint64) *Call {
	call := c.Pending[seq]
	delete(c.Pending, seq)
	return call
}

func (c *Client) terminateCalls(err error)  {
	c.Sending.Lock()
	defer c.Sending.Unlock()
	c.Mu.Lock()
	defer c.Mu.Unlock()
	c.ShutDown = true
	for _, call := range c.Pending {
		call.Error = err
		call.done()
	}
}

func (c *Client) receive() {
	var err error
	for err == nil {
		var h = &codec.Header{}
		if err = c.CC.ReadHeader(h); err != nil {
			log.Println("read head error:", err)
			break
		}
		call := c.removeCall(h.Seq)
		switch {
		case call == nil:
			err = errors.New("receive invalid call")
		case h.Error != "":
			call.Error = errors.New(h.Error)
			err = errors.New("receive invalid call")
			call.done()
		default:
			err = c.CC.ReadBody(call.Reply)
			if err != nil {
				fmt.Println("read body error:", err)
			}
			call.done()
		}
	}
	c.terminateCalls(err)
}

func checkOption(opt ...*server.Option) (*server.Option, error) {
	if len(opt) == 0 || opt[0] == nil {
		return server.NewGobOption(), nil
	}
	if len(opt) != 1 {
		return nil, errors.New("option length can't exceed 2")
	}
	return opt[0], nil
}

func HTTPDial(network, address string, opt *server.Option) (client *Client, err error) {
	conn, err := net.Dial(network, address)
	if err != nil {
		return nil, err
	}
	defer func() {
		if client == nil {
			_ = conn.Close()
		}
	}()
	return NewHTTPClient(conn, *opt)
}

func XDial(protocol, address string, opt ...*server.Option) (client *Client, err error) {
	op, err := checkOption(opt...)
	if err != nil {
		return nil, err
	}
	TimeChan := make(chan ClientResult)
	switch protocol {
	case "http":
		go func() {
			client, err := HTTPDial("tcp", address, op)
			TimeChan <- ClientResult{client: client, Err: err}
		}()
	default:
		go func() {
			client, err := dial(protocol, address, op)
			TimeChan <- ClientResult{client: client, Err: err}
		}()
	}

	if op.ConnectionTimeOut == 0 {
		result := <-TimeChan
		return result.client, result.Err
	}

	select {
	case <-time.After(op.ConnectionTimeOut):
		return nil, errors.New("client dial time out")
	case result := <-TimeChan:
		return result.client, result.Err
	}
}

func Dial(network, address string, opt ...*server.Option) (client *Client, err error) {
	// Dial with timeout
	op, err := checkOption(opt...)
	if err != nil {
		return nil, err
	}
	timeOutChan := make(chan ClientResult)
	go func(){
		client, err := dial(network, address, op)
		timeOutChan <- ClientResult{client: client, Err: err}
	}()
	select {
	case <-time.After(op.ConnectionTimeOut):
		return nil, errors.New("serve client time out")
	case result := <- timeOutChan:
		return result.client, result.Err
	}
}

func dial(network, address string, opt *server.Option) (client *Client, err error) {
	//basic dial, only handle dial, no timeout
	conn, err := net.Dial(network, address)
	if err != nil {
		return nil, err
	}
	defer func() {
		if client == nil {
			_ = conn.Close()
		}
	}()

	return NewClient(conn, *opt)
}

func (c *Client) send(call *Call)  {
	c.Sending.Lock()
	defer c.Sending.Unlock()

	seq, err :=c.registerCall(call)
	if err != nil {
		log.Println("register call error:", err)
		return
	}
	header := &codec.Header{
		ServiceMethod: call.ServerMethod,
		Seq: seq,
		Error: "",
	}

	if err := c.CC.Write(header, call.Args); err != nil {
		call := c.removeCall(seq)
		if call != nil {
			call.Error = err
			call.done()
		}
	}
}

func (c *Client) Call(ServerMethod string, Args interface{}, Reply interface{}, buf uint, CallTimeOut time.Duration) error {
	if buf == 0 {
		return errors.New("buffer size must larger than 1")
	}

	call := NewCall(ServerMethod, Args, Reply, buf)
	go func() {
		c.send(call)
	}()

	if CallTimeOut == 0{
		BackCall := <-call.Done
		return BackCall.Error
	}

	select {
	case <-time.After(CallTimeOut):
		c.removeCall(call.Sqe)
		return errors.New("client call time out")
	case BackCall := <-call.Done:
		return BackCall.Error
	}
}

// test function

func TestXDial() {
	if runtime.GOOS == "linux" {
		ch := make(chan struct{})
		addr := "/tmp/geerpc.sock"
		go func() {
			_ = os.Remove(addr)
			l, err := net.Listen("unix", addr)
			if err != nil {
				log.Fatal("failed to listen unix socket")
			}
			ch <- struct{}{}
			server.Accept(l)
		}()
		<-ch
		_, err := XDial("unix" , addr)
		if err != nil {
			fmt.Print(err)
		}
	}
}