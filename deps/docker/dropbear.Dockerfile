FROM alpine:3.23.3

RUN apk add --no-cache \
    bash \
    gcc \
    git \
    make \
    musl-dev \
    tar \
    zlib-dev \
    zstd
