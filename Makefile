proto:
	protoc --go_opt=paths=source_relative --go_out=. ./spec/protocol/*.proto

build:
	go build -tags 'osusergo netgo' -ldflags "-s -w -extldflags -static" -a -o bin/server ./cmd/server
	go build -tags 'osusergo netgo' -ldflags "-s -w -extldflags -static" -a -o bin/client ./cmd/client