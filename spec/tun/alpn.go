package tun

import (
	"github.com/zllovesuki/specter/spec/protocol"

	"google.golang.org/protobuf/proto"
)

var alpnNameMap map[int32]string = make(map[int32]string)

func init() {
	m := &protocol.Link{}
	len := m.GetAlpn().Enum().Descriptor().Values().Len()
	for i := 0; i < len; i++ {
		num := m.GetAlpn().Enum().Descriptor().Values().Get(i).Number()
		name := proto.GetExtension(m.GetAlpn().Descriptor().Values().ByNumber(num).Options(), protocol.E_AlpnName).(string)
		alpnNameMap[int32(num)] = name
	}
}

func ALPN(a protocol.Link_ALPN) string {
	return alpnNameMap[int32(a.Number())]
}
