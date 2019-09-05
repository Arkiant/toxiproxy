FROM alpine

COPY tmp/build/toxiproxy-server-linux-amd64 /go/bin/toxiproxy
COPY tmp/build/toxiproxy-cli-linux-amd64 /go/bin/toxiproxy-cli

RUN mkdir /share

COPY share/ /share

VOLUME ["/share"]

EXPOSE 8474
ENTRYPOINT ["/go/bin/toxiproxy"]
CMD ["-host=0.0.0.0", "-config=/share/proxy_config.json", "-toxics=/share/toxic_config.json"]
