package codec

import (
	"bufio"
	"encoding/gob"
	"io"
	"log"
)

type GobCodec struct {
	conn io.ReadWriteCloser
	buf *bufio.Writer
	enc *gob.Encoder
	dec *gob.Decoder
}

func (g *GobCodec) Close() error {
	return g.conn.Close()
}

func (g *GobCodec) ReadHeader(h *Header) error {
	return g.dec.Decode(h)
}

func (g *GobCodec) ReadBody(b interface{}) error {
	return g.dec.Decode(b)
}

func (g *GobCodec) Write(h *Header, b interface{}) (err error) {
	defer func() {
		_ = g.buf.Flush()
		if err != nil{
			_ = g.conn.Close()
		}
	}()
	if err := g.enc.Encode(h); err != nil{
		log.Println("gob write header err:", err)
		return err
	}
	if err := g.enc.Encode(b); err != nil{
		log.Println("gob write body error:", err)
		return err
	}
	return nil
}

func NewGobCodec(conn io.ReadWriteCloser) Codec{
	buf := bufio.NewWriter(conn)
	return &GobCodec{
		conn: conn,
		buf:  buf,
		enc:  gob.NewEncoder(buf),
		dec:  gob.NewDecoder(conn),
	}
}



