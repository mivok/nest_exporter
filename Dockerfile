FROM alpine:latest

EXPOSE 9264

RUN apk update; apk add go git musl-dev
RUN go get github.com/mivok/nest_exporter; \
    mv ~/go/bin/nest_exporter /usr/local/bin; \
    rm -rf ~/go

USER nobody
ENTRYPOINT ["/usr/local/bin/nest_exporter"]
CMD ["-config", "/etc/nest_exporter.toml"]
