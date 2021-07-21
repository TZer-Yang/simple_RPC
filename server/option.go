package server

import (
	"geerpc/codec"
	"time"
)

var GobTypeNumber = 0x123456

type Option struct {
	TypeNumber int
	CodecType codec.CodecType
	ConnectionTimeOut time.Duration
	HandleTimeOut time.Duration
}

func NewGobOption() *Option {
	return &Option{
		TypeNumber: GobTypeNumber,
		CodecType: codec.GOB_TYPE,
		ConnectionTimeOut: time.Second * 0,
		HandleTimeOut: 0,
	}
}
