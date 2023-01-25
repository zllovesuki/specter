PLATFORMS := windows/amd64/.exe linux/amd64 darwin/amd64 illumos/amd64 windows/arm64/.exe android/arm64 linux/arm64 darwin/arm64 linux/arm freebsd/amd64

BUILD=$(shell git rev-parse --short HEAD)
PROTOC_GO=`which protoc-gen-go`
PROTOC_VTPROTO=`which protoc-gen-go-vtproto`

COUNT=5
GOARM=7
GOAMD64=v3
GOTAGS=-tags 'osusergo netgo urfave_cli_no_docs no_mocks'
LDFLAGS=-ldflags "-s -w -extldflags -static -X=kon.nect.sh/specter/cmd/specter.Build=$(BUILD)"
TIMEOUT=180s

plat_temp = $(subst /, ,$@)
os = $(word 1, $(plat_temp))
arch = $(word 2, $(plat_temp))
ext = $(word 3, $(plat_temp))

.DEFAULT_GOAL := all

# ==========================DEV===========================

buildx: certs
	docker buildx build -t specter -f Dockerfile.dev .

dev-server: buildx
	SKIP=" " docker compose -f compose-server.yaml up

dev-server-acme: buildx
	docker compose -f compose-server.yaml up

dev-server-logs:
	docker compose -f compose-server.yaml logs -f

dev-client: buildx
	docker compose -f compose-client.yaml up

yeet-server:
	docker compose -f compose-server.yaml down
	docker compose -f compose-server.yaml stop
	docker compose -f compose-server.yaml rm
	docker image prune -f
	docker volume prune -f

yeet-client:
	docker compose -f compose-client.yaml down
	docker compose -f compose-client.yaml stop
	docker compose -f compose-client.yaml rm
	docker image prune -f
	docker volume prune -f

buildx-validator:
	docker buildx build -t validator -f Dockerfile.validator .

dev-validate: buildx-validator
	docker compose -f compose-validator.yaml up

# ========================================================

all: proto test clean release

release: $(PLATFORMS)

compat: GOAMD64 = v1
compat: GOARM = 6
compat: ext := -compat
compat: release

$(PLATFORMS):
	CGO_ENABLED=0 GOOS=$(os) GOARCH=$(arch) GOARM=$(GOARM) GOAMD64=$(GOAMD64) go build $(GOTAGS) $(LDFLAGS) -o bin/specter-$(os)-$(arch)$(ext) .
ifdef wal
	CGO_ENABLED=0 GOOS=$(os) GOARCH=$(arch) GOARM=$(GOARM) GOAMD64=$(GOAMD64) go build $(GOTAGS) $(LDFLAGS) -o bin/wal-$(os)-$(arch)$(ext) ./cmd/wal
endif

upx: release
	-find ./bin -type f -exec upx --best --lzma {} +

docker:
	docker buildx build -t specter:$(BUILD) -f Dockerfile .

proto:
	protoc \
		--go_opt=module=kon.nect.sh/specter \
		--go-vtproto_opt=module=kon.nect.sh/specter \
		--go_out=. --plugin protoc-gen-go="$(PROTOC_GO)" \
		--go-vtproto_out=. --plugin protoc-gen-go-vtproto="$(PROTOC_VTPROTO)" \
		--go-vtproto_opt=features=marshal+unmarshal+size+pool \
		--go-vtproto_opt=pool=kon.nect.sh/specter/spec/protocol.RPC \
		./spec/proto/*.proto

proto_aof_kv:
	protoc \
		--go_opt=module=kon.nect.sh/specter \
		--go-vtproto_opt=module=kon.nect.sh/specter \
		--go_out=. --plugin protoc-gen-go="$(PROTOC_GO)" \
		--go-vtproto_out=. --plugin protoc-gen-go-vtproto="$(PROTOC_VTPROTO)" \
		--go-vtproto_opt=features=marshal+unmarshal+size+pool \
		--go-vtproto_opt=pool=kon.nect.sh/specter/kv/aof/proto.Mutation \
		--go-vtproto_opt=pool=kon.nect.sh/specter/kv/aof/proto.LogEntry \
		./kv/aof/proto/*.proto

benchmark_kv:
	go test -benchmem -bench=. -benchtime=10s -cpu 1,2,4 ./kv

dep:
	go install golang.org/x/tools/cmd/stringer@latest
	go install github.com/planetscale/vtprotobuf/cmd/protoc-gen-go-vtproto@latest
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install honnef.co/go/tools/cmd/staticcheck@2022.1.3

vet:
	go vet ./...
	staticcheck ./...

full_test: test extended_test long_test concurrency_test

test:
	go test -short -cover -count=1 -timeout 60s ./...
	go test -short -race -cover -count=1 -timeout 60s ./...

coverage:
	go test -short -race -coverprofile cover.out ./...
	go tool cover -func cover.out
	-rm cover.out

extended_test:
	go test -short -count=$(COUNT) -timeout 120s ./...
	go test -short -race -count=$(COUNT) -timeout 120s ./...

long_test:
	go test -timeout $(TIMEOUT) -run ^TestLotsOfNodes -race -count=$(COUNT) ./chord/...

concurrency_test:
	go test -timeout $(TIMEOUT) -run ^TestConcurrentJoin -race -count=1 ./chord/...
	go test -timeout $(TIMEOUT) -run ^TestConcurrentLeave -race -count=1 ./chord/...

clean:
	-rm bin/*
	-rm cover.out

certs:
	mkdir certs
	# Create CA key
	openssl ecparam -name prime256v1 -genkey -noout -out certs/ca.key
	# Generate CA CSR
	openssl req -new -key certs/ca.key -out certs/ca.csr -subj "/C=US/ST=California/L=San Francisco/O=Dev/OU=Dev/CN=ca.dev"
	# Verify CA CSR
	openssl req -text -in certs/ca.csr -noout -verify
	# Generate self-signed CA
	openssl x509 -signkey certs/ca.key -in certs/ca.csr -req -days 365 -out certs/ca.crt
	# Generate node key
	openssl ecparam -name prime256v1 -genkey -noout -out certs/node.key
	# Generate node CSR
	openssl req -new -key certs/node.key -out certs/node.csr -subj "/C=US/ST=California/L=San Francisco/O=Dev/OU=Dev/CN=node.ca.dev"
	# Verify node CSR
	openssl req -text -in certs/node.csr -noout -verify
	# Sign and generate node certificate
	openssl x509 -req -CA certs/ca.crt -CAkey certs/ca.key -in certs/node.csr -out certs/node.crt -days 365 -CAcreateserial -extfile dev/openssl.txt

licenses:
	go-licenses save kon.nect.sh/specter --save_path=./licenses
	find ./licenses -type f -exec tail -n +1 {} + > ThirdPartyLicenses.txt
	-rm -rf ./licenses

.PHONY: all