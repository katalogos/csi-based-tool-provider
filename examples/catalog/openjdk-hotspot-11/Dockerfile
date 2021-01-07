FROM registry.access.redhat.com/ubi8/ubi as builder
WORKDIR /build
RUN curl -L https://github.com/AdoptOpenJDK/openjdk11-binaries/releases/download/jdk-11.0.9.1%2B1/OpenJDK11U-jdk_x64_linux_hotspot_11.0.9.1_1.tar.gz | tar xz

FROM scratch
WORKDIR /
COPY --from=builder /build/jdk-11.0.9.1+1 .
