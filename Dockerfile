FROM centos:centos7
LABEL maintainers="David Festal"
LABEL description="Tool Provider Driver"

RUN \
  yum install -y epel-release && \
  yum install -y buildah && \
  yum clean all

COPY ./bin/toolproviderplugin /toolproviderplugin
ENTRYPOINT ["/toolproviderplugin"]
