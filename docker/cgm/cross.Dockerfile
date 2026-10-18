FROM golang:1.25.12-bookworm
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
      gcc-aarch64-linux-gnu libc6-dev-arm64-cross \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /source
