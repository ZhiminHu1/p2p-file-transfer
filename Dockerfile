FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o p2p-transfer ./cmd/p2p-transfer

FROM alpine:latest
COPY --from=builder /app/p2p-transfer /usr/local/bin/
EXPOSE 8000 8001
ENTRYPOINT ["p2p-transfer"]