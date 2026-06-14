# syntax=docker/dockerfile:1
# f4kvs-ffi and f4kvs-lexical are supplied as additional build contexts:
#   docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build
#   docker build --build-context f4kvs=./f4kvs-ffi --build-context f4kvs-lexical=../f4kvs-lexical .

# Debian apt rustc is too old for f4kvs transitive deps (edition 2024).
FROM rust:1-bookworm AS f4kvs-lib
COPY --from=f4kvs . /opt/f4kvs-ffi
WORKDIR /opt/f4kvs-ffi
RUN cargo build -p f4kvs-ffi --release --target-dir /opt/f4kvs-ffi/target/ffi-release

FROM debian:bookworm-slim AS tantivy-lib
ARG TARGETARCH
ARG TANTIVY_VERSION=v1.0.6
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*
RUN case "${TARGETARCH}" in \
      amd64) TANTIVY_PLATFORM=linux-amd64-musl ;; \
      arm64) TANTIVY_PLATFORM=linux-arm64-musl ;; \
      *) echo "unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
    esac \
    && curl -fsSL -o /tmp/tantivy.tar.gz \
      "https://github.com/anyproto/tantivy-go/releases/download/${TANTIVY_VERSION}/${TANTIVY_PLATFORM}.tar.gz" \
    && mkdir -p /opt/tantivy/lib \
    && tar -xzf /tmp/tantivy.tar.gz -C /opt/tantivy/lib \
    && rm -f /tmp/tantivy.tar.gz

FROM golang:1.25-bookworm AS go-build
WORKDIR /src

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    clang \
    pkg-config \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
# go.mod replace => ../f4kvs-lexical (sibling of /src)
COPY --from=f4kvs-lexical . /f4kvs-lexical
RUN go mod download

COPY . .

RUN mkdir -p lib
COPY --from=f4kvs-lib /opt/f4kvs-ffi/target/ffi-release/release/libf4kvs_ffi.a lib/
COPY --from=f4kvs-lib /opt/f4kvs-ffi/target/ffi-release/release/libf4kvs_ffi.so lib/
COPY --from=tantivy-lib /opt/tantivy/lib/libtantivy_go.a lib/

ENV CGO_ENABLED=1
ENV CGO_LDFLAGS=-L/src/lib
RUN go build -tags tantivy -o /out/agent ./agent

FROM debian:bookworm-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

RUN mkdir -p lib
COPY --from=go-build /out/agent /app/bin/agent
COPY --from=f4kvs-lib /opt/f4kvs-ffi/target/ffi-release/release/libf4kvs_ffi.so lib/

ENV LD_LIBRARY_PATH=/app/lib
ENV RAG_LISTEN=:8080
ENV RAG_LLM_PROVIDER=ollama
ENV RAG_LLM_BASE_URL=http://host.docker.internal:11434
ENV RAG_LLM_MODEL=qwen2.5:7b-instruct
ENV RAG_EMBEDDING_MODEL=nomic-embed-text
ENV RAG_DATA_DIR=/data

EXPOSE 8080

CMD ["/app/bin/agent"]
