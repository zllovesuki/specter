module kon.nect.sh/specter

go 1.19

require (
	github.com/CAFxX/atomic128 v0.2.0
	github.com/TheZeroSlave/zapsentry v1.12.0
	github.com/avast/retry-go/v4 v4.3.2
	github.com/caddyserver/certmagic v0.17.2
	github.com/getsentry/sentry-go v0.17.0
	github.com/go-chi/chi/v5 v5.0.8
	github.com/golang/protobuf v1.5.2
	github.com/gorilla/websocket v1.5.0
	github.com/libp2p/go-buffer-pool v0.1.0
	github.com/libp2p/go-yamux/v3 v3.1.2
	github.com/mholt/acmez v1.0.4
	github.com/montanaflynn/stats v0.7.0
	github.com/olekukonko/tablewriter v0.0.5
	github.com/orisano/wyhash v1.1.0
	github.com/quic-go/quic-go v0.31.1
	github.com/sethvargo/go-diceware v0.3.0
	github.com/stretchr/testify v1.8.1
	github.com/tidwall/wal v1.1.7
	github.com/twitchtv/twirp v8.1.3+incompatible
	github.com/urfave/cli/v2 v2.24.2
	github.com/zhangyunhao116/skipmap v0.10.1
	github.com/zhangyunhao116/skipset v0.13.0
	go.uber.org/atomic v1.10.0
	go.uber.org/goleak v1.2.0
	go.uber.org/zap v1.24.0
	golang.org/x/net v0.5.0
	google.golang.org/protobuf v1.28.1
	gopkg.in/yaml.v3 v3.0.1
	kon.nect.sh/challenger v1.0.1
	kon.nect.sh/httprate v0.7.2
	moul.io/zapfilter v1.7.0
)

replace github.com/libp2p/go-yamux/v3 => github.com/zllovesuki/go-yamux/v3 v3.1.3-0.20230123001349-cf75f3123f5e

replace github.com/quic-go/quic-go => github.com/lucas-clemente/quic-go v0.31.2-0.20230122035357-58cedf7a4f47

require (
	github.com/benbjohnson/clock v1.3.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-task/slim-sprig v0.0.0-20210107165309-348f09dbbbc0 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/google/pprof v0.0.0-20230111200839-76d1ae5aea2b // indirect
	github.com/klauspost/cpuid/v2 v2.2.3 // indirect
	github.com/libdns/libdns v0.2.1 // indirect
	github.com/mattn/go-runewidth v0.0.14 // indirect
	github.com/miekg/dns v1.1.50 // indirect
	github.com/onsi/ginkgo/v2 v2.8.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/quic-go/qpack v0.4.0 // indirect
	github.com/quic-go/qtls-go1-18 v0.2.0 // indirect
	github.com/quic-go/qtls-go1-19 v0.2.0 // indirect
	github.com/quic-go/qtls-go1-20 v0.1.0-rc.1 // indirect
	github.com/rivo/uniseg v0.4.3 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/stretchr/objx v0.5.0 // indirect
	github.com/tidwall/gjson v1.14.4 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/tinylru v1.1.0 // indirect
	github.com/xrash/smetrics v0.0.0-20201216005158-039620a65673 // indirect
	github.com/zhangyunhao116/fastrand v0.3.0 // indirect
	go.uber.org/multierr v1.9.0 // indirect
	golang.org/x/crypto v0.5.0 // indirect
	golang.org/x/exp v0.0.0-20230130200758-8bd7c9d05862 // indirect
	golang.org/x/lint v0.0.0-20210508222113-6edffad5e616 // indirect
	golang.org/x/mod v0.7.0 // indirect
	golang.org/x/sys v0.4.0 // indirect
	golang.org/x/text v0.6.0 // indirect
	golang.org/x/tools v0.5.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)
