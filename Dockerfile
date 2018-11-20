FROM golang:1.11-alpine

ENV PROJECT=upp-aggregate-healthcheck
ENV ORG_PATH="github.com/Financial-Times"
ENV SRC_FOLDER="${GOPATH}/src/${ORG_PATH}/${PROJECT}"

# Set up our extra bits in the image
RUN apk --no-cache --upgrade add \
    git \
    curl \
    ca-certificates \
    && update-ca-certificates

RUN curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh

WORKDIR ${SRC_FOLDER}

# Install dependancies
COPY Gopkg.* ${SRC_FOLDER}/
RUN "$GOPATH/bin/dep" ensure -vendor-only

# Build app
COPY . ${SRC_FOLDER}
RUN BUILDINFO_PACKAGE="${ORG_PATH}/${PROJECT}/vendor/${ORG_PATH}/service-status-go/buildinfo." \
    && VERSION="version=$(git describe --tag --always 2> /dev/null)" \
    && DATETIME="dateTime=$(date -u +%Y%m%d%H%M%S)" \
    && REPOSITORY="repository=$(git config --get remote.origin.url)" \
    && REVISION="revision=$(git rev-parse HEAD)" \
    && BUILDER="builder=$(go version)" \
    && LDFLAGS="-s -w -X '"${BUILDINFO_PACKAGE}$VERSION"' -X '"${BUILDINFO_PACKAGE}$DATETIME"' -X '"${BUILDINFO_PACKAGE}$REPOSITORY"' -X '"${BUILDINFO_PACKAGE}$REVISION"' -X '"${BUILDINFO_PACKAGE}$BUILDER"'" \
    && CGO_ENABLED=0 go build -a -o /artifacts/${PROJECT} -ldflags="${LDFLAGS}" \
    && cp -R resources /artifacts/resources \
    && cp -R html-templates /artifacts/html-templates


# Multi-stage build - copy only the certs and the binary into the image
FROM scratch
WORKDIR /
COPY --from=0 /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=0 /artifacts/* /
COPY --from=0 /artifacts/resources /resources
COPY --from=0 /artifacts/html-templates /html-templates

CMD [ "/upp-aggregate-healthcheck" ]
