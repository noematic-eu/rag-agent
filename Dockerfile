# syntax=docker/dockerfile:1
# f4kvs-v2 is not on public GitHub. Supply it as an additional build context:
#   docker compose build   (set F4KVS_ROOT in .env)
#   docker build --build-context f4kvs=$F4KVS_ROOT .

# Debian apt rustc is too old for f4kvs transitive deps (edition 2024).
FROM rust:1-bookworm AS f4kvs-lib
COPY --from=f4kvs . /opt/f4kvs-v2
WORKDIR /opt/f4kvs-v2
RUN cargo build -p f4kvs-ffi --release --target-dir /opt/f4kvs-v2/target/ffi-release

FROM golang:1.24-bookworm

WORKDIR /src

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    build-essential \
    clang \
    pkg-config \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN mkdir -p lib
COPY --from=f4kvs-lib /opt/f4kvs-v2/target/ffi-release/release/libf4kvs_ffi.a lib/
COPY --from=f4kvs-lib /opt/f4kvs-v2/target/ffi-release/release/libf4kvs_ffi.so lib/

RUN CGO_ENABLED=1 go build -o /src/bin/agent ./agent

ENV RAG_LISTEN=:8080
ENV RAG_LLM_PROVIDER=ollama
ENV RAG_LLM_BASE_URL=http://ollama:11434
ENV RAG_LLM_MODEL=qwq
ENV RAG_EMBEDDING_MODEL=nomic-embed-text
ENV RAG_DATA_DIR=/data

EXPOSE 8080

CMD ["/src/bin/agent"]
