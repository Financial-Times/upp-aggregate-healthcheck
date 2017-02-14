FROM golang:1.7-alpine3.5

RUN mkdir -p /upp-aggregate-healthcheck

ADD . "$GOPATH/src/upp-aggregate-healthcheck"

RUN apk --no-cache --virtual .build-dependencies add git \
  && cd $GOPATH/src/upp-aggregate-healthcheck \
  && go-wrapper download \
  && go-wrapper install \
  && pwd \
  && ls -la $GOPATH \
  && cp healthcheck-template.html /upp-aggregate-healthcheck/ \
  && cp add-ack-message-form-template.html /upp-aggregate-healthcheck/ \
  && cp -R resources /upp-aggregate-healthcheck/ \
  && apk del .build-dependencies \
  && rm -rf $GOPATH/src $GOPATH/pkg

WORKDIR /upp-aggregate-healthcheck

EXPOSE 8080

CMD ["go-wrapper", "run"]
