PLATFORMS := windows/amd64/.exe linux/amd64 darwin/amd64 illumos/amd64 linux/arm64 darwin/arm64 linux/arm

GOTAGS=-tags 'osusergo netgo'
LDFLAGS=-ldflags "-s -w -extldflags -static"
NDK_PATH=${ANDROID_NDK_HOME}

plat_temp = $(subst /, ,$@)
os = $(word 1, $(plat_temp))
arch = $(word 2, $(plat_temp))
ext = $(word 3, $(plat_temp))

all: proto test clean release

release: $(PLATFORMS)

$(PLATFORMS):
	GOOS=$(os) GOARCH=$(arch) GOARM=7 go build $(GOTAGS) $(LDFLAGS) -o bin/specter-server-$(os)-$(arch)$(ext) ./cmd/server
	GOOS=$(os) GOARCH=$(arch) GOARM=7 go build $(GOTAGS) $(LDFLAGS) -o bin/specter-client-$(os)-$(arch)$(ext) ./cmd/client

android:
	CC=$(NDK_PATH)/toolchains/llvm/prebuilt/linux-x86_64/bin/aarch64-linux-android28-clang CXX=$(NDK_PATH)/toolchains/llvm/prebuilt/linux-x86_64/bin/aarch64-linux-android28-clang++ CGO_ENABLED=1 GOARCH=arm64 GOOS=android go build $(GOTAGS) $(LDFLAGS) -o bin/specter-client-android-arm64 ./cmd/client

proto:
	protoc --go_opt=module=github.com/zllovesuki/specter --go_out=. ./spec/proto/*.proto

test:
	go test -v -race -cover -count=1 ./...

extended_test:
	go test -race -count=5 ./...

clean:
	rm bin/*

.PHONY: all