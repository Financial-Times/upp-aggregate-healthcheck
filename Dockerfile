FROM alpine:3.5

ADD . /upp-aggregate-healthcheck/

RUN apk --update add go git musl-dev \
  && export GOPATH=/.gopath \
  && go version \
  && mkdir -p $GOPATH/src/github.com/Financial-Times/upp-aggregate-healthcheck \
  && cd $GOPATH/src/github.com/Financial-Times/upp-aggregate-healthcheck \
  && git clone https://github.com/Financial-Times/upp-aggregate-healthcheck.git . \
  && git fetch \
  && git checkout kubernetes-version \
  && go get github.com/jawher/mow.cli \ 
  && go get github.com/gorilla/mux \
  && go get github.com/Financial-Times/go-fthealth/v1a \
  && go build github.com/Financial-Times/upp-aggregate-healthcheck \
  && mv healthcheck-template.html /healthcheck-template.html \
  && mv add-ack-message-form-template.html /add-ack-message-form-template.html \
  && mv upp-aggregate-healthcheck /upp-aggregate-healthcheck-app \
  && apk del go git musl-dev \
  && rm -rf $GOPATH /var/cache/apk/*

EXPOSE 8080

CMD [ "/upp-aggregate-healthcheck-app" ]
