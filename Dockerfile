FROM golang:1.22-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o server cmd/server/main.go

FROM alpine:latest

RUN apk add --no-cache ffmpeg tzdata ca-certificates wget
ENV TZ=Asia/Tokyo

WORKDIR /app

COPY --from=builder /build/server .
COPY --from=builder /build/web ./web
COPY --from=builder /build/storage/keys ./storage/keys

EXPOSE 8080

CMD ["./server"]
