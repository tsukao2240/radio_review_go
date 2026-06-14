FROM golang:1.22-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o server cmd/server/main.go
RUN go build -o batch cmd/batch/main.go

FROM alpine:latest

RUN apk add --no-cache ffmpeg tzdata ca-certificates wget
ENV TZ=Asia/Tokyo

WORKDIR /app
RUN mkdir -p /app/storage/keys /app/storage/recordings

COPY --from=builder /build/server .
COPY --from=builder /build/batch .
COPY --from=builder /build/web ./web

EXPOSE 8080

CMD ["./server"]
