FROM quay.io/buildah/stable:v1.17.0
LABEL maintainers="David Festal"
LABEL description="Tool Provider Driver"

COPY ./bin/toolproviderplugin /toolproviderplugin
ENTRYPOINT ["/toolproviderplugin"]
