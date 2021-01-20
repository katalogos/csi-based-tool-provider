
FROM registry.access.redhat.com/ubi8/go-toolset as builder
WORKDIR /opt/app-root/src
RUN echo "package main; func main() { }" > noop.go && go build noop.go
RUN curl -L https://github.com/AdoptOpenJDK/openjdk11-binaries/releases/download/jdk-11.0.9.1%2B1/OpenJDK11U-jdk_x64_linux_hotspot_11.0.9.1_1.tar.gz | tar xz

FROM scratch
WORKDIR /
COPY --from=builder /opt/app-root/src/jdk-11.0.9.1+1 .
COPY --from=builder /opt/app-root/src/noop ./.csi-tool-provider/noop
CMD [ "/.csi-tool-provider/noop" ]
