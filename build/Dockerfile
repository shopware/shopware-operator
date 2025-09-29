# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM --platform=$TARGETPLATFORM gcr.io/distroless/static:nonroot

ARG TARGETPLATFORM
ARG BUILDPLATFORM

ENV NAMESPACE=default

COPY $TARGETPLATFORM/manager /
COPY LICENSE /LICENSE
COPY third-party-licenses.md /third-party-licenses.md
USER 65532:65532

ENTRYPOINT ["/manager"]
