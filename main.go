package main

import (
	"geerpc/Discovery"
	"geerpc/client"
	register_center "geerpc/register-center"
	"geerpc/server"
	"geerpc/service"
	"log"
	"net"
	http "net/http"
	"sync"
)

func StartServer(addr chan string)  {
	Server := server.NewServer()
	var foo service.Foo
	if err := Server.RegisterService(&foo); err != nil {
		log.Fatal("rpc server register: ", err)
	}

	// pick a free port
	nl, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("network listen fatal:", err)
		return
	}
	Server.HandledHTTP()
	addr <- nl.Addr().String()
	//Server.AcceptConn(nl)
	err = http.Serve(nl, nil)

	if err != nil {
		log.Fatal("rpc server: http serve net listener, ",err)
	}
}

func StartServer1(addr chan string)  {
	var foo service.Foo
	l, _ := net.Listen("tcp", ":0")
	Server := server.NewServer()
	_ = Server.RegisterService(&foo)
	addr <- l.Addr().String()
	Server.AcceptConn(l)
}


//day1-5
//func main() {
//
//	addr := make(chan string)
//	go StartServer(addr)
//
//	c, err := client.XDial("http", <-addr)
//	if err != nil {
//		log.Fatal("client dial error:", err)
//	}
//
//	defer func() {
//		_ = c.Close()
//	}()
//
//	time.Sleep(time.Second)
//
//	var wg sync.WaitGroup
//
//	fmt.Println("rpc start")
//	for i := 0; i < 10; i++ {
//		wg.Add(1)
//		go func(i int) {
//			defer wg.Done()
//			args := &service.Args{i, i}
//			var reply int
//			if err := c.Call("Foo.Sum", args, &reply, 10, time.Second * 2); err != nil {
//				log.Println(fmt.Sprintf("call fail, call code: %d, %s", i, err))
//			}
//			log.Println("reply:", reply)
//		}(i)
//	}
//
//	wg.Wait()
//
//}

func simplecall(registry string){
	//
	d := Discovery.NewRegisterDiscovery(registry, 0)
	xc := client.NewXClient(d, Discovery.RANDOM_SELECT, nil)
	defer func() {
		_ = xc.Close()
	}()
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			var reply int
			defer wg.Done()
			if err := xc.Call("Foo.Sum", &service.Args{Num1: i, Num2: i}, &reply); err == nil{
				log.Println("reply: ", reply)
			} else {
				log.Println("rpc xclient simple call Err:", err)
			}
		}()
	}
	wg.Wait()
}

func broadcastcall(registry string){
	//
	d := Discovery.NewRegisterDiscovery(registry, 0)
	xc := client.NewXClient(d, Discovery.RANDOM_SELECT, nil)
	defer func() {
		_ = xc.Close()
	}()
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			var reply int
			defer wg.Done()
			if err := xc.BroadCast("Foo.Sum", &service.Args{Num1: i, Num2: i}, &reply); err == nil{
				log.Println("reply: ", reply)
			} else {
				log.Println("rpc xclient simple call Err:", err)
			}
		}()
	}
	wg.Wait()
}

//func day6demo(){
//	ch1 := make(chan string)
//	ch2 := make(chan string)
//
//	go StartServer1(ch1)
//	go StartServer1(ch2)
//
//	addr1 := <-ch1
//	addr2 := <-ch2
//
//	simplecall(addr1, addr2)
//	broadcastcall(addr1, addr2)
//}

func startRegistry(wg *sync.WaitGroup) {
	l, _ := net.Listen("tcp", ":9999")
	regs := register_center.NewRegister(0)
	regs.HandleHTTP(register_center.DEFAULT_PATH)
	wg.Done()
	_ = http.Serve(l, nil)
}

func StartServer2(regisgerAddr string, wg *sync.WaitGroup) {
	var foo service.Foo
	l, _ := net.Listen("tcp", ":0")
	Server := server.NewServer()
	_ = Server.RegisterService(&foo)
	register_center.HeartBeat(regisgerAddr, "tcp "+l.Addr().String(), 0)
	wg.Done()
	Server.AcceptConn(l)
}



func day7demo()  {
	registryAddr := "http://localhost:9999/_geerpc_/registry"
	var wg sync.WaitGroup

	// start registry
	wg.Add(1)
	go startRegistry(&wg)
	wg.Wait()

	// start two server
	wg.Add(2)
	go StartServer2(registryAddr, &wg)
	go StartServer2(registryAddr, &wg)
	wg.Wait()

	//start call
	simplecall(registryAddr)
	broadcastcall(registryAddr)
}

func main() {
	day7demo()
}