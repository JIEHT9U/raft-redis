FROM alpine:3.7
RUN apk add --no-cache ca-certificates
COPY raft-redis /opt/raft-redis
EXPOSE 9087
ENTRYPOINT ["/opt/raft-redis"]
