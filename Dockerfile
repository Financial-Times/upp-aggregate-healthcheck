FROM golang:1

ENV PROJECT=upp-aggregate-healthcheck
ENV ORG_PATH="github.com/Financial-Times"
ENV SRC_FOLDER="${GOPATH}/src/${ORG_PATH}/${PROJECT}"
ENV BUILDINFO_PACKAGE="github.com/Financial-Times/service-status-go/buildinfo."

ARG GITHUB_USERNAME
ARG GITHUB_TOKEN

WORKDIR ${SRC_FOLDER}

# Build app
COPY . ${SRC_FOLDER}
RUN VERSION="version=$(git describe --tag --always 2> /dev/null)" \
    && DATETIME="dateTime=$(date -u +%Y%m%d%H%M%S)" \
    && REPOSITORY="repository=$(git config --get remote.origin.url)" \
    && REVISION="revision=$(git rev-parse HEAD)" \
    && BUILDER="builder=$(go version)" \
    && LDFLAGS="-X '"${BUILDINFO_PACKAGE}$VERSION"' -X '"${BUILDINFO_PACKAGE}$DATETIME"' -X '"${BUILDINFO_PACKAGE}$REPOSITORY"' -X '"${BUILDINFO_PACKAGE}$REVISION"' -X '"${BUILDINFO_PACKAGE}$BUILDER"'" \
    && CGO_ENABLED=0 go install -ldflags "-s -w -extldflags '-static'" github.com/go-delve/delve/cmd/dlv@latest \
    && CGO_ENABLED=0 go build -gcflags "all=-N -l" -mod=readonly -o /artifacts/${PROJECT} -ldflags="${LDFLAGS}" \
    && cp -R resources /artifacts/resources \
    && cp -R html-templates /artifacts/html-templates


# Multi-stage build - copy only the certs and the binary into the image
FROM scratch
WORKDIR /
COPY --from=0 /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=0 /go/bin/dlv /usr/local/bin/dlv
COPY --from=0 /artifacts/* /
COPY --from=0 /artifacts/resources /resources
COPY --from=0 /artifacts/html-templates /html-templates

CMD ["dlv", "exec", "--continue", "--headless", "--listen=0.0.0.0:2345", "--api-version=2", "--accept-multiclient", "/upp-aggregate-healthcheck"]
