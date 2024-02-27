# Builder
FROM golang:1.21-alpine as builder

# RUN apk update && apk add protoc
RUN mkdir /app
WORKDIR /app
RUN export GOPATH=/app

#RUN go get    github.com/prometheus/client_golang@v1.12.2 \
#              golang.org/x/sys@v0.0.0-20220114195835-da31bd327af9 \
#              github.com/prometheus/procfs@v0.7.3

COPY go.mod ./
COPY go.sum ./

RUN go mod download

RUN export PATH="$PATH:$(go env GOPATH)/bin"

ADD *.go /app/src/
WORKDIR /app/src
# GOOS=linux GOARCH=amd64
RUN env CGO_ENABLED=0 go build -o /nexus_proxy *.go

# Final image
FROM alpine:latest as app

EXPOSE 8080

WORKDIR /
COPY --from=builder /nexus_proxy /nexus_proxy
ENTRYPOINT ["/nexus_proxy"]
CMD ["--help"]
