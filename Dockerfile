FROM golang:1.15-alpine AS builder

ARG BUILD_VERSION

RUN apk add --quiet --no-cache build-base git

WORKDIR /src

ENV GO111MODULE=on

ADD go.* ./

RUN go mod download

ADD . .

RUN cd cmd/selenosis && \
    go install -ldflags="-X main.buildVersion=$BUILD_VERSION -linkmode external -extldflags '-static' -s -w"


FROM scratch

COPY --from=builder /go/bin/selenosis /

ENTRYPOINT ["/selenosis"]