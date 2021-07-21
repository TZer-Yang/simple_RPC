package codec

import (
	"io"
)

type CodecType string
type GobCodecFunc func(conn io.ReadWriteCloser) Codec

const (
	GOB_TYPE  CodecType = "gob"
)

type Header struct {
	ServiceMethod string
	Seq uint64
	Error string
}

type Codec interface {
	io.Closer
	ReadHeader(h *Header) error
	ReadBody(b interface{}) error
	Write(h *Header, b interface{}) error
}

var CodecFuncMap map[CodecType]GobCodecFunc

func init() {
	CodecFuncMap = make(map[CodecType]GobCodecFunc)
	CodecFuncMap[GOB_TYPE] = NewGobCodec
}
