#!/bin/sh

set -e
set -x

GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o nexus_proxy *.go
