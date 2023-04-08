# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.20.3-alpine as builder
RUN apk --no-cache add ca-certificates git
WORKDIR /app
COPY . .

ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=7 GOAMD64=v3 \
    go build -tags 'osusergo netgo urfave_cli_no_docs no_mocks' \
    -ldflags "-s -w -extldflags -static -X=kon.nect.sh/specter/cmd/specter.Build=`git rev-parse --short HEAD`" \
    -o bin/specter .

FROM scratch
WORKDIR /app
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/bin/specter /app/specter

ENTRYPOINT ["/app/specter"]