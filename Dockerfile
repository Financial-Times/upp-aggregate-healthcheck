FROM alpine:edge

ADD . /upp-aggregate-healthcheck/

RUN apk --update add go git musl-dev \
  && export GOPATH=/.gopath \
  && go version \
  && go get github.com/Financial-Times/upp-aggregate-healthcheck \
  && cd $GOPATH/src/github.com/Financial-Times/upp-aggregate-healthcheck \
  && git fetch \
  && git checkout kubernetes-version \
  && go build github.com/Financial-Times/upp-aggregate-healthcheck \
  && mv upp-aggregate-healthcheck /upp-aggregate-healthcheck-app \
  && mv html-templates/services-healthcheck-template.html /html-templates/services-healthcheck-template.html \
  && apk del go git musl-dev \
  && rm -rf $GOPATH /var/cache/apk/*

EXPOSE 8080

CMD [ "/upp-aggregate-healthcheck-app" ]