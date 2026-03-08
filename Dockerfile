FROM golang:1.22 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
COPY cmd ./cmd
COPY internal ./internal
COPY admin ./admin
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/subserver ./

FROM alpine:3.20
RUN apk add --no-cache ca-certificates \
 && addgroup -S subserver \
 && adduser -S -D -H -h /app -G subserver subserver \
 && mkdir -p /app/data /app/configs \
 && chown -R subserver:subserver /app

WORKDIR /app
COPY --from=builder /out/subserver /app/subserver
COPY --from=builder /src/admin /app/admin

USER subserver:subserver

EXPOSE 8080
CMD ["/app/subserver"]
