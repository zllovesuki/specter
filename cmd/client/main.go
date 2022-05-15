package main

import (
	"fmt"

	"specter/spec/protocol"

	"google.golang.org/protobuf/proto"
)

func main() {
	xd := &protocol.Link{
		Alpn: protocol.Link_HTTP,
	}

	fmt.Printf("%+v\n", xd)
	fmt.Printf("%+v\n", proto.GetExtension(xd.GetAlpn().Descriptor().Values().ByNumber(xd.GetAlpn().Number()).Options(), protocol.E_AlpnName))
}
