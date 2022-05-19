PLATFORMS := windows/amd64 linux/amd64 darwin/amd64 linux/arm64 darwin/arm64

plat_temp = $(subst /, ,$@)
os = $(word 1, $(plat_temp))
arch = $(word 2, $(plat_temp))

release: $(PLATFORMS)

$(PLATFORMS):
	GOOS=$(os) GOARCH=$(arch) go build -tags 'osusergo netgo' -ldflags "-s -w -extldflags -static" -o bin/specter-server-$(os)-$(arch) ./cmd/server
	GOOS=$(os) GOARCH=$(arch) go build -tags 'osusergo netgo' -ldflags "-s -w -extldflags -static" -o bin/specter-client-$(os)-$(arch) ./cmd/client

proto:
	protoc --go_opt=paths=source_relative --go_out=. ./spec/protocol/*.proto

build:
	go build -tags 'osusergo netgo' -ldflags "-s -w -extldflags -static" -a -o bin/server ./cmd/server
	go build -tags 'osusergo netgo' -ldflags "-s -w -extldflags -static" -a -o bin/client ./cmd/client

test:
	go test -v -race -cover ./...

.PHONY: release $(PLATFORMS)