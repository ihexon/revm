FROM ubuntu:25.10

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    bash \
    bc \
    bison \
    ca-certificates \
    clang \
    cpio \
    curl \
    elfutils \
    file \
    flex \
    gcc \
    g++ \
    git \
    libcap-ng-dev \
    libclang-dev \
    libelf-dev \
    llvm \
    make \
    patch \
    perl \
    python3-pyelftools \
    tar \
    wget \
    xz-utils \
    zstd \
    && rm -rf /var/lib/apt/lists/*

ENV RUSTUP_HOME=/usr/local/rustup
ENV CARGO_HOME=/usr/local/cargo
ENV PATH=/usr/local/cargo/bin:$PATH

RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \
    | sh -s -- -y --profile minimal --default-toolchain stable \
    && chmod -R a+w "$RUSTUP_HOME" "$CARGO_HOME"
