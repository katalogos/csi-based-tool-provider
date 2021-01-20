
FROM registry.access.redhat.com/ubi8/go-toolset as builder
WORKDIR /opt/app-root/src
RUN echo "package main; func main() { }" > noop.go && go build noop.go
RUN curl -L https://mirrors.ircam.fr/pub/apache/maven/maven-3/3.6.3/binaries/apache-maven-3.6.3-bin.tar.gz | tar xz

FROM scratch
WORKDIR /
COPY --from=builder /opt/app-root/src/apache-maven-3.6.3 .
COPY --from=builder /opt/app-root/src/noop ./.csi-tool-provider/noop
CMD [ "/.csi-tool-provider/noop" ]