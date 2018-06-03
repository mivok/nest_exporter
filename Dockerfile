FROM golang:1.10 as builder
WORKDIR /go/src/github.com/mivok/nest_exporter
COPY . .
RUN go get -d -v
RUN go install -v


FROM alpine:latest
EXPOSE 9264
COPY --from=builder /go/bin/nest_exporter /usr/local/bin/
USER nobody
ENTRYPOINT ["/usr/local/bin/nest_exporter"]
CMD ["-config", "/etc/nest_exporter.toml"]
