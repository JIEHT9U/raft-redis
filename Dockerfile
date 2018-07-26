FROM golang:1.10.0-alpine3.7 AS build

ARG LDFLAGS

WORKDIR /go/src/github.com/JIEHT9U/raft-redis/
COPY . .
RUN go build -ldflags="${LDFLAGS}" -o go-redis-raft main.go 

FROM alpine:3.7
RUN apk add --no-cache ca-certificates
COPY --from=build /go/src/github.com/JIEHT9U/raft-redis/go-redis-raft /opt/go-redis-raft 
ENTRYPOINT ["/opt/go-redis-raft"]
