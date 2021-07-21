package client

import (
	"geerpc/Discovery"
	"geerpc/server"
	"io"
	"reflect"
	"strings"
	"sync"
)

type XClient struct {
	Dsc Discovery.DiscoveryI
	Model Discovery.SelectModel
	Opt *server.Option
	Mu sync.Mutex
	clients map[string]*Client
}

var _ io.Closer = (*XClient)(nil)

func (xc *XClient)Close() error {
	xc.Mu.Lock()
	defer xc.Mu.Unlock()
	for key, client := range xc.clients{
		_ = client.Close()
		delete(xc.clients, key)
	}
	return nil
}

func NewXClient(d Discovery.DiscoveryI, model Discovery.SelectModel, opt *server.Option) *XClient{
	return &XClient{Dsc: d, Model: model, Opt: opt, clients: make(map[string]*Client)}
}

func (xc *XClient)dial(rpcAddr string) (*Client,error) {
	xc.Mu.Lock()
	defer xc.Mu.Unlock()
	client, ok := xc.clients[rpcAddr]
	if ok && !client.IsAvailable(){
		_ = client.Close()
		delete(xc.clients, rpcAddr)
		client = nil
	}

	if client == nil{
		var err error
		protocol := strings.Split(rpcAddr, " ")[0]
		addr := strings.Split(rpcAddr, " ")[1]
		client, err = XDial(protocol, addr, xc.Opt)
		if err != nil{
			return nil, err
		}
		xc.clients[rpcAddr] = client
	}
	return client, nil
}

func (xc *XClient)call(rpcAddr string, serviceMethod string, args, reply interface{}) error {
	client, err := xc.dial(rpcAddr)
	if err != nil {
		return err
	}
	return client.Call(serviceMethod, args, reply, 10, 0)
}

func (xc *XClient)Call(serviceMethod string, args, reply interface{}) error {
	rpcAddr, err := xc.Dsc.Get(xc.Model)
	if err != nil{
		return err
	}
	return xc.call(rpcAddr, serviceMethod, args, reply)
}

func (xc *XClient)BroadCast(serviceMethod string, args, reply interface{}) error {
	servers, err := xc.Dsc.GetAll()
	if err != nil{
		return err
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	replyDone := reply == nil
	var e error
	for _, rpcAddr := range servers{
		wg.Add(1)
		go func(rpcAddr string) {
			defer wg.Done()
			var cloneReply interface{}
			if reply != nil {
				cloneReply = reflect.New(reflect.ValueOf(reply).Elem().Type()).Interface()
			}
			err := xc.call(rpcAddr, serviceMethod, args, cloneReply)
			mu.Lock()
			if err != nil && e == nil{
				e = err
			}
			if err == nil && !replyDone {
				reflect.ValueOf(reply).Elem().Set(reflect.ValueOf(cloneReply).Elem())
				replyDone = true
			}
			mu.Unlock()
		}(rpcAddr)
	}
	wg.Wait()
	return e
}