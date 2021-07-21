package register_center

import (
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Register struct {
	TimeOut time.Duration
	Mu sync.Mutex
	Servers map[string]*RegisterServerItem
}

type RegisterServerItem struct {
	Addr string
	StartTime time.Time
}

const(
	DEFAULT_PATH = "/_geerpc_/registry"
	DEFAULT_TIME_OUT = time.Minute * 5
)

func NewRegister(timeout time.Duration) *Register{
	return &Register{
		TimeOut: timeout,
		Servers: make(map[string]*RegisterServerItem),
	}
}

func (rg *Register) putServer(addr string)  {
	rg.Mu.Lock()
	defer rg.Mu.Unlock()
	s := rg.Servers[addr]
	if s == nil {
		rg.Servers[addr] = &RegisterServerItem{Addr: addr, StartTime: time.Now()}
	} else {
		s.StartTime = time.Now()
	}
}

func (rg *Register) getAliveServer() []string {
	rg.Mu.Lock()
	defer rg.Mu.Unlock()
	var alive []string
	for addr, serveritem := range rg.Servers {
		if rg.TimeOut == 0 || serveritem.StartTime.Add(rg.TimeOut).After(time.Now()) {
			alive = append(alive, addr)
		} else {
			delete(rg.Servers, addr)
		}
	}
	return alive
}

func (rg *Register) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET":
		w.Header().Set("X-Geerpc-Servers", strings.Join(rg.getAliveServer(), ","))
	case "POST":
		addr := req.Header.Get("X-Geerpc-Server")
		if addr == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		rg.putServer(addr)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (rg *Register) HandleHTTP(registerPath string) {
	http.Handle(registerPath, rg)
	log.Println("rpc register path:", registerPath)
}

func HeartBeat(registry, addr string, duration time.Duration)  {
	if duration == 0 {
		//default duration
		duration = DEFAULT_TIME_OUT - time.Minute
	}

	err := sendHeartBeat(registry, addr)

	go func() {
		t := time.NewTicker(duration)
		for err == nil {
			<-t.C
			if err = sendHeartBeat(registry, addr); err != nil {
				log.Println("rpc register heart beat: ", err)
			}
		}
	}()
}

func sendHeartBeat(registry, addr string) error {
	log.Println(addr, "send heart beat to registry", registry)
	httpClient := &http.Client{}
	req, _ := http.NewRequest("POST", registry, nil)
	req.Header.Set("X-Geerpc-Server", addr)
	_, err := httpClient.Do(req)
	return err
}

