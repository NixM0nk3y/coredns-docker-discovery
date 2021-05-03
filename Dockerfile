FROM golang:1-buster

RUN go mod download github.com/coredns/coredns@v1.8.3

WORKDIR $GOPATH/pkg/mod/github.com/coredns/coredns@v1.8.3
RUN go mod download

RUN echo "docker:github.com/rb-coredns/coredns-docker-discovery" >> plugin.cfg
ENV CGO_ENABLED=0
RUN go generate coredns.go && go build -mod=mod -o=/usr/local/bin/coredns

FROM alpine:3

RUN apk --no-cache add ca-certificates
COPY --from=0 /usr/local/bin/coredns /usr/local/bin/coredns

ENTRYPOINT ["/usr/local/bin/coredns"]