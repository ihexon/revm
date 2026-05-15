FROM golang:1.25.5 AS go

FROM ubuntu:25.10

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    gcc \
    g++ \
    git \
    libacl1-dev \
    libarchive-dev \
    libarchive-tools \
    libblkid-dev \
    libbz2-dev \
    libext2fs-dev \
    libicu-dev \
    liblz4-dev \
    liblzma-dev \
    libxml2-dev \
    libxxhash-dev \
    libzstd-dev \
    nettle-dev \
    patchelf \
    uuid-dev \
    wget \
    zlib1g-dev \
    zstd \
    && rm -rf /var/lib/apt/lists/*

COPY --from=go /usr/local/go /usr/local/go

ENV PATH=/usr/local/go/bin:$PATH
