FROM golang:1.8 AS builder

# Install v0.3.2 of dep
RUN wget -O dep https://github.com/golang/dep/releases/download/v0.3.2/dep-linux-amd64 && \
    echo '322152b8b50b26e5e3a7f6ebaeb75d9c11a747e64bbfd0d8bb1f4d89a031c2b5  dep' | sha256sum -c - && \
    chmod +x dep && \
    mv dep /usr/bin

RUN mkdir -p /go/src/dubclan/api
WORKDIR /go/src/dubclan/api

COPY Gopkg.toml Gopkg.lock ./

# Install dependencies
RUN dep ensure -vendor-only
COPY . .

# Build the api
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o qitup main.go api.go

# Build the release image
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/
COPY --from=builder /go/src/dubclan/api/qitup .
CMD ["./qitup"]