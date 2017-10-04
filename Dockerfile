FROM golang:latest as builder
WORKDIR /go/src/dubclan/api/
COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o api ./api.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /go/src/dubclan/api/api .
CMD ["./api"]