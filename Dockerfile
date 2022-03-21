FROM golang:1.18-alpine as build-backend

ARG REVISION_INFO

ADD . /build
WORKDIR /build

ENV CGO_ENABLED 0

RUN go get -v -t -d ./...

RUN apk add --update-cache \
    git \
  && rm -rf /var/cache/apk/*

RUN export GOPATH=$(go env GOPATH) && \
    echo "Building..." && \
      version="${REVISION_INFO:-unknown}" && \
    echo "--- Build app version=$version ---" && \
      go build -o tobym -ldflags "-X main.revision=${version} -s -w" ./app

RUN go test -timeout=60s ./...

FROM alpine:3.15

WORKDIR /srv

COPY --from=build-backend /build/tobym /srv/tobym

CMD ["/srv/tobym"]
