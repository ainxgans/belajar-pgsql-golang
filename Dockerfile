FROM golang:alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/gen ./cmd/gen

FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata curl

WORKDIR /app

COPY --from=builder /bin/api /app/api
COPY --from=builder /bin/gen /app/gen

EXPOSE 8080

ENTRYPOINT ["/app/api"]
