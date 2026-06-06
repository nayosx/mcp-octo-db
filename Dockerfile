FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/octo-db .

FROM alpine:3.22

WORKDIR /app

COPY --from=builder /out/octo-db /usr/local/bin/octo-db
COPY .env.example /app/.env.example
COPY config.yaml.example /app/config.yaml.example

ENTRYPOINT ["/usr/local/bin/octo-db"]
