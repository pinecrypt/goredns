FROM golang:1.16-alpine
RUN apk add libcap git
WORKDIR $GOPATH/src/git.k-space.ee/pinecrypt/goredns
COPY go.mod .
RUN go mod download
COPY main.go .
RUN go build -o goredns main.go
RUN setcap 'cap_net_bind_service=+ep' goredns
CMD ["./goredns"]
