package Discovery

import (
	"errors"
	"log"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

type SelectModel int

const (
	RANDOM_SELECT SelectModel = iota
	ROUND_ROBIN_SELECT
	DEFAULT_TIME_OUT_UPDATE = time.Second * 10
)

type DiscoveryI interface {
	Refresh() error
	Update(servers []string) error
	Get(model SelectModel) (string,error)
	GetAll() ([]string,error)
}

type ManualServerDiscovery struct {
	R *rand.Rand
	Mu sync.Mutex
	Servers []string
	Position int // record the selected position for robin algorithm
}

func NewManualServerDiscovery(servers []string) *ManualServerDiscovery {
	d := &ManualServerDiscovery{
		Servers: servers,
		R: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	d.Position =  d.R.Intn(math.MaxInt32 - 1)
	return d
}

func (msd *ManualServerDiscovery)Refresh() error {
	return nil
}

func (msd *ManualServerDiscovery)Update(servers []string) error {
	msd.Mu.Lock()
	defer msd.Mu.Unlock()
	msd.Servers = servers
	return nil
}

func (msd *ManualServerDiscovery)Get(model SelectModel) (string, error) {
	msd.Mu.Lock()
	defer msd.Mu.Unlock()
	n := len(msd.Servers)
	if n == 0{
		return "", errors.New("empty server")
	}
	switch model {
	case RANDOM_SELECT:
		return msd.Servers[msd.R.Intn(n)], nil
	case ROUND_ROBIN_SELECT:
		s := msd.Servers[msd.Position]
		msd.Position = (msd.Position + 1) % n
		return s, nil
	default:
		return "", errors.New("no such select model")
	}
}

func (msd *ManualServerDiscovery)GetAll() ([]string,error) {
	msd.Mu.Lock()
	defer msd.Mu.Unlock()
	n := len(msd.Servers)
	if n == 0{
		return nil, errors.New("empty servers")
	} else {
		s := make([]string, n, n)
		copy(s, msd.Servers)
		return s, nil
	}
}

type RegisterDiscovery struct {
	*ManualServerDiscovery
	Registry string
	TimeOut time.Duration
	LastUpdate time.Time
}

func NewRegisterDiscovery(registry string, updatetimeout time.Duration) *RegisterDiscovery{
	if updatetimeout == 0 {
		updatetimeout = DEFAULT_TIME_OUT_UPDATE
	}
	return &RegisterDiscovery{
		ManualServerDiscovery: NewManualServerDiscovery(make([]string, 0)),
		Registry: registry,
		TimeOut: updatetimeout,
	}
}

func (rd *RegisterDiscovery)Refresh() error {
	rd.Mu.Lock()
	defer rd.Mu.Unlock()
	if rd.LastUpdate.Add(rd.TimeOut).After(time.Now()) {
		return nil
	}
	log.Println("rpc register refresh from registry:", rd.Registry)

	resp, err := http.Get(rd.Registry)
	if err != nil {
		return err
	}
	servers := strings.Split(resp.Header.Get("X-Geerpc-Servers"), ",")
	rd.Servers = make([]string, len(servers))
	copy(rd.Servers, servers)
	rd.LastUpdate = time.Now()
	return nil
}

func (rd *RegisterDiscovery)Update(servers []string) error {
	rd.Mu.Lock()
	defer rd.Mu.Unlock()
	rd.Servers = servers
	rd.LastUpdate = time.Now()
	return nil
}

func (rd *RegisterDiscovery) Get(model SelectModel) (string,error) {
	if err := rd.Refresh(); err != nil {
		return "", err
	}
	return rd.ManualServerDiscovery.Get(model)
}

func (rd *RegisterDiscovery) GetAll() ([]string,error) {
	if err := rd.Refresh(); err != nil {
		return nil, err
	}
	return rd.ManualServerDiscovery.GetAll()
}

