package overlay

import (
	"encoding/binary"
	"errors"
	"io"

	"github.com/lucas-clemente/quic-go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func RequestReply(stream quic.Stream, req proto.Message, resp protoreflect.ProtoMessage) error {
	rr, err := proto.Marshal(req)
	if err != nil {
		return err
	}

	buf := make([]byte, 8+len(rr))
	binary.BigEndian.PutUint64(buf[0:7], uint64(len(rr)))
	copy(buf[7:], rr)

	n, err := stream.Write(buf)
	if err != nil {
		return err
	}
	if n != len(buf) {
		return errors.New("wtf")
	}

	// then we read
	sb := make([]byte, 8)
	n, err = io.ReadFull(stream, sb)
	if err != nil {
		return err
	}
	if n != len(buf) {
		return errors.New("wtf")
	}

	l := binary.BigEndian.Uint64(sb)
	nb := make([]byte, l)
	n, err = io.ReadFull(stream, nb)
	if err != nil {
		return err
	}
	if n != len(buf) {
		return errors.New("wtf")
	}

	if err := proto.Unmarshal(nb, resp); err != nil {
		return err
	}

	return nil
}
