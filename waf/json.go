package waf

import (
	"io"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// MarshalJSON emits the WAF API's field names and explicit default values.
func MarshalJSON(message proto.Message) ([]byte, error) {
	return (protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}).Marshal(message)
}

// DecodeJSON reads one complete message and rejects unknown fields.
// HTTP callers must bound the reader before decoding.
func DecodeJSON(input io.Reader, message proto.Message) error {
	data, err := io.ReadAll(input)
	if err != nil {
		return err
	}
	return protojson.Unmarshal(data, message)
}
