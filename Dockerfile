FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/nil-loader ./cmd/server/
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/testservice ./cmd/testservice/

FROM alpine:3.20

RUN apk add --no-cache ca-certificates

COPY --from=builder /bin/nil-loader /usr/local/bin/nil-loader
COPY --from=builder /bin/testservice /usr/local/bin/testservice

EXPOSE 8081 50051

ENTRYPOINT ["nil-loader"]
