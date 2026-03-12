# Building stage
FROM docker.m.daocloud.io/golang:1.24

RUN apk add --update make

WORKDIR /go/src/github.com/guyuxiang/projectc-wallet

COPY . /go/src/github.com/guyuxiang/projectc-wallet

ENV CGO_ENABLED 0
ENV GOOS linux
ENV GO111MODULE=on

ENV TIMEZONE "Asia/Shanghai"

RUN make src.build
RUN mv build/pkg/cmd/projectc-wallet/projectc-wallet /go/bin

# Production stage
FROM alpine:3.9
WORKDIR /go/bin

LABEL maintainer="guyuxiang <guyuxiang@qq.com>"

# copy the go binaries from the building stage
COPY --from=0 /go/bin /go/bin

COPY etc/projectc-wallet.yaml /etc/projectc-wallet/projectc-wallet.yaml

EXPOSE 80

ENTRYPOINT ["./projectc-wallet"]
