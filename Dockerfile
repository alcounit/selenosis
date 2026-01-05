
FROM golang:1.25.0 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

RUN go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/selenosis \
    ./cmd/selenosis


FROM gcr.io/distroless/static:nonroot

WORKDIR /

COPY --from=builder /out/selenosis /selenosis

USER 65532:65532

ENTRYPOINT ["/selenosis"]
