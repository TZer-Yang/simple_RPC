package service

import (
	"fmt"
	"go/ast"
	"log"
	"reflect"
	"sync/atomic"
)


type MethodType struct {
	Method    reflect.Method
	ArgsType  reflect.Type
	ReplyType reflect.Type
	NumCalls  uint64
}

func (mt *MethodType) CallNums() uint64 {
	return atomic.LoadUint64(&mt.NumCalls)
}

func (mt *MethodType) NewArgv() reflect.Value {
	var argv reflect.Value
	if mt.ArgsType.Kind() == reflect.Ptr {
		argv = reflect.New(mt.ArgsType.Elem())
	} else {
		argv = reflect.New(mt.ArgsType).Elem()
	}
	return argv
}

func (mt *MethodType) NewReplyv() reflect.Value {
	reply := reflect.New(mt.ReplyType.Elem())
	switch mt.ReplyType.Elem().Kind() {
	case reflect.Map:
		reply.Elem().Set(reflect.MakeMap((mt.ReplyType.Elem())))
	case reflect.Slice:
		reply.Elem().Set(reflect.MakeSlice(mt.ReplyType.Elem(), 0, 0))
	}
	return reply
}

type Service struct {
	Name   string
	Typ    reflect.Type
	Rcvr   reflect.Value
	Method map[string]*MethodType
}

func NewService(rcvr interface{}) *Service {
	s := new(Service)
	s.Rcvr = reflect.ValueOf(rcvr)
	s.Name = reflect.Indirect(s.Rcvr).Type().Name()
	s.Typ = reflect.TypeOf(rcvr)
	s.Method = make(map[string]*MethodType)
	if !ast.IsExported(s.Name) {
		log.Fatal("rpc server: %s is not a valid service", s.Name)
	}
	s.registerMethods()
	return s
}

func (s *Service) registerMethods()  {
	for i := 0; i < s.Typ.NumMethod(); i++ {
		method := s.Typ.Method(i)
		mType := method.Type
		if mType.NumIn() != 3 || mType.NumOut() != 1 {
			//log.Println("rpc server: method %s args or reply is invalid", mType.Name())
			continue
		}
		if mType.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
			continue
		}
		argType, replyType := mType.In(1), mType.In(2)
		if !isExportedOrBultinType(argType) || !isExportedOrBultinType(replyType) {
			continue
		}
		s.Method[method.Name] = &MethodType{
			Method:    method,
			ArgsType:  argType,
			ReplyType: replyType,
		}
		log.Println(fmt.Sprintf("rpc server: register %s.%s", s.Name, method.Name))
	}
}

func isExportedOrBultinType(t reflect.Type) bool {
	return ast.IsExported(t.Name()) || t.PkgPath() == ""
}

func (s *Service) MethodCall(mt *MethodType, argv, replyv reflect.Value) error {
	atomic.AddUint64(&mt.NumCalls, 1)
	f := mt.Method.Func
	returnValues := f.Call([]reflect.Value{s.Rcvr, argv, replyv})
	if err := returnValues[0].Interface(); err != nil {
		return err.(error)
	}
	return nil
}

// test function

func TestNewService() {
	var foo Foo
	s := NewService(&foo)
	_assert(len(s.Method) == 1, "wrong service Method, expect 1, but got %d", len(s.Method))
	mType := s.Method["Sum"]
	_assert(mType != nil, "wrong Method, Sum shouldn't nil")
}

func TestMethodType_Call() {
	var foo Foo
	s := NewService(&foo)
	mType := s.Method["Sum"]

	argv := mType.NewArgv()
	replyv := mType.NewReplyv()
	argv.Set(reflect.ValueOf(Args{Num1: 1, Num2: 3}))
	err := s.MethodCall(mType, argv, replyv)
	_assert(err == nil && *replyv.Interface().(*int) == 4 && mType.CallNums() == 1, "failed to call Foo.Sum")
}