FROM golang:1.15-alpine3.12 as builder
RUN mkdir /src
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories
ADD . /src
WORKDIR /src
RUN  GOPROXY=https://goproxy.cn go build -o jtproxy test.go  && chmod +x jtproxy


FROM alpine:3.12
ENV ZONEINFO=/app/zoneinfo.zip
RUN mkdir /app
WORKDIR /app

COPY --from=builder /usr/local/go/lib/time/zoneinfo.zip /app

COPY --from=builder /src /app

ENTRYPOINT  ["./jtproxy"]
EXPOSE 80