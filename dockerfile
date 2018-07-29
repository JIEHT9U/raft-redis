FROM golang:1.10.0-alpine3.7 AS build

ARG LDFLAGS

WORKDIR /go/src/github.com/JIEHT9U/raft-redis/
COPY . .
RUN go build -ldflags="${LDFLAGS}" -o raft-redis main.go 

FROM alpine:3.7
RUN apk add --no-cache ca-certificates
COPY --from=build /go/src/github.com/JIEHT9U/raft-redis/raft-redis /opt/raft-redis 
ENTRYPOINT ["/opt/raft-redis"]
