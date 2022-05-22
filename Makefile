PLATFORMS := windows/amd64/.exe linux/amd64 darwin/amd64 illumos/amd64 windows/arm64/.exe linux/arm64 darwin/arm64 linux/arm freebsd/amd64

GOARM=7
GOAMD64=v2
GOTAGS=-tags 'osusergo netgo'
LDFLAGS=-ldflags "-s -w -extldflags -static -X=main.Build=$(BUILD)"
NDK_PATH=${ANDROID_NDK_HOME}
BUILD=`git rev-parse --short HEAD`
PROTOC_GO=`which protoc-gen-go`
PROTOC_VTPROTO=`which protoc-gen-go-vtproto`

plat_temp = $(subst /, ,$@)
os = $(word 1, $(plat_temp))
arch = $(word 2, $(plat_temp))
ext = $(word 3, $(plat_temp))

all: proto test clean release

release: $(PLATFORMS)

$(PLATFORMS):
	GOOS=$(os) GOARCH=$(arch) GOARM=$(GOARM) GOAMD64=$(GOAMD64) go build $(GOTAGS) $(LDFLAGS) -o bin/specter-server-$(os)-$(arch)$(ext) ./cmd/server
	GOOS=$(os) GOARCH=$(arch) GOARM=$(GOARM) GOAMD64=$(GOAMD64) go build $(GOTAGS) $(LDFLAGS) -o bin/specter-client-$(os)-$(arch)$(ext) ./cmd/client

android:
	CC=$(NDK_PATH)/toolchains/llvm/prebuilt/linux-x86_64/bin/aarch64-linux-android28-clang CXX=$(NDK_PATH)/toolchains/llvm/prebuilt/linux-x86_64/bin/aarch64-linux-android28-clang++ CGO_ENABLED=1 GOARCH=arm64 GOOS=android go build $(GOTAGS) $(LDFLAGS) -o bin/specter-client-android-arm64 ./cmd/client

proto:
	protoc \
		--go_opt=module=github.com/zllovesuki/specter \
		--go-vtproto_opt=module=github.com/zllovesuki/specter \
		--go_out=. --plugin protoc-gen-go="$(PROTOC_GO)" \
		--go-vtproto_out=. --plugin protoc-gen-go-vtproto="$(PROTOC_VTPROTO)" \
		--go-vtproto_opt=features=marshal+unmarshal+size+pool \
		--go-vtproto_opt=pool=github.com/zllovesuki/specter/spec/protocol.RPC \
		./spec/proto/*.proto

 dep:
	go install github.com/planetscale/vtprotobuf/cmd/protoc-gen-go-vtproto@latest
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

test:
	go test -v -race -cover -count=1 ./...

extended_test:
	go test -race -count=5 ./...

clean:
	rm bin/*

.PHONY: all