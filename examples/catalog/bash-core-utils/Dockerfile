
FROM registry.access.redhat.com/ubi8/go-toolset as builder
WORKDIR /opt/app-root/src
RUN echo "package main; func main() { }" > noop.go && go build noop.go
RUN curl -L https://github.com/robxu9/bash-static/releases/download/5.1.004-1.2.2/bash-linux-x86_64 > bash && chmod a+x bash
RUN curl -L https://raw.githubusercontent.com/luciusmagn/coreutils-static/master/build.sh > build.sh && chmod a+x build.sh && ./build.sh

FROM scratch
WORKDIR /
COPY --from=builder /opt/app-root/src/releases/* ./
COPY --from=builder /opt/app-root/src/bash ./
COPY --from=builder /opt/app-root/src/bash ./sh
COPY --from=builder /opt/app-root/src/noop ./.csi-tool-provider/noop
CMD [ "/.csi-tool-provider/noop" ]