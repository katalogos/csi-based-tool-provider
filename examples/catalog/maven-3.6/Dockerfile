FROM registry.access.redhat.com/ubi8/ubi as builder
WORKDIR /build
RUN curl -L https://mirrors.ircam.fr/pub/apache/maven/maven-3/3.6.3/binaries/apache-maven-3.6.3-bin.tar.gz | tar xz

FROM scratch
WORKDIR /
COPY --from=builder /build/apache-maven-3.6.3 .
