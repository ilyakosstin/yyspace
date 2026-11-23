FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go openapi.json ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /focus-helper .

FROM alpine:3.23
RUN apk add --no-cache ca-certificates
COPY --from=build /focus-helper /usr/local/bin/focus-helper
USER 65532:65532
EXPOSE 8000
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s \
  CMD wget -q -O /dev/null http://127.0.0.1:8000/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/focus-helper"]
