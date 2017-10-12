FROM golang:latest as builder
WORKDIR /go/src/dubclan/api/
COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o qitup main.go api.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /api/
COPY --from=builder /go/src/dubclan/api/qitup .
CMD ["./qitup"]