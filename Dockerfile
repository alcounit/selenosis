
FROM --platform=$BUILDPLATFORM golang:1.25.0 AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/selenosis \
    ./cmd/selenosis


FROM gcr.io/distroless/static:nonroot

WORKDIR /

COPY --from=builder /out/selenosis /selenosis

USER 65532:65532

ENTRYPOINT ["/selenosis"]
