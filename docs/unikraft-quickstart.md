# Unikraft quickstart (local kraft run)

Run the RAG agent as a **Unikraft unikernel** on your machine with [Kraftkit](https://unikraft.org/docs/cli). This complements the Docker workflow ([`docker-compose-quickstart.md`](docker-compose-quickstart.md)): same API on host port **8081**, persistent data in `./rag-data`, and Ollama on the host.

Unikraft uses a minimal scratch rootfs (see [`Dockerfile.unikraft`](../Dockerfile.unikraft) + [`Kraftfile`](../Kraftfile)) instead of a full Linux userspace.

## Prerequisites

1. **Kraftkit** — install from [Unikraft docs](https://unikraft.org/docs/cli).
2. **Docker Desktop** — must be **running** before `make unikraft-build` or `make unikraft-run`. Kraftkit uses Docker’s BuildKit to build the rootfs from `Dockerfile.unikraft`. If you see `could not start BuildKit ephemeral container`, start Docker Desktop and retry.
3. **BuildKit daemon** (optional; Kraftkit can use Docker’s built-in BuildKit when Docker Desktop is running):

```bash
docker run -d --name buildkitd --privileged moby/buildkit:latest
export KRAFTKIT_BUILDKIT_HOST=docker-container://buildkitd
```

Add `export KRAFTKIT_BUILDKIT_HOST=docker-container://buildkitd` to your shell profile if you use Unikraft often.

4. **Ollama** on the host with models pulled (see Docker quickstart §2).
5. **f4kvs-ffi** — prepared automatically by `make unikraft-prepare` (clone or symlink from `F4KVS_ROOT` in `.env`).

## 1) Prepare and run

```bash
make unikraft-prepare
make unikraft-run
```

`make unikraft-run` starts QEMU with the `unikraft.org/base:latest` bincompat runtime (`x86_64`). On Apple Silicon, it uses software emulation (`-W`) because the catalog base image is not yet published for `qemu/arm64`. Override with `UNIKRAFT_ARCH=arm64` when that package becomes available.

- Port `8081:8080` (same as Docker Compose)
- Volume `./rag-data:/data`
- Default Ollama URL `http://10.0.2.2:11434` (QEMU user-network gateway to the host)

First run builds the rootfs via BuildKit (Rust + Go + CGO); expect several minutes.

### Optional overrides

```bash
# More memory for large corpora
make unikraft-run UNIKRAFT_MEMORY=1Gi

# Force x86_64 under QEMU software emulation (slow on Apple Silicon)
kraft run --plat qemu --arch x86_64 -W -p 8081:8080 -M 512Mi .

# Custom Ollama URL
kraft run -e RAG_LLM_BASE_URL=http://10.0.2.2:11434 -p 8081:8080 -M 512Mi .
```

## 2) Smoke test API

```bash
curl http://127.0.0.1:8081/stats
```

## 3) Seed demo corpora

Same commands as the Docker quickstart, using port **8081**:

```bash
./scripts/eval_setup_legal.sh http://127.0.0.1:8081

go run ./client -mode ingest-dir \
  -dir ./eval/fixtures/docs \
  -server http://127.0.0.1:8081 \
  -corpus eval-public \
  -finalize=true
```

Data is stored under `./rag-data` on the host and survives unikernel restarts.

## Host networking (Ollama)

| Runtime | Reach host Ollama at |
|---------|-------------------|
| Docker Compose | `http://host.docker.internal:11434` |
| Unikraft (QEMU user net) | `http://10.0.2.2:11434` |

`10.0.2.2` is the default gateway from the QEMU user network to your Mac/Linux host. Override with `-e RAG_LLM_BASE_URL=...` if your setup differs.

## Build only (debug initrd)

```bash
make unikraft-build
ls -la .unikraft/build/
```

## Troubleshooting

### BuildKit not running

```
error: failed to connect to buildkit
```

Start BuildKit and set `KRAFTKIT_BUILDKIT_HOST` (see Prerequisites §3).

### Missing shared library at runtime

The scratch rootfs bundles libs discovered via `ldd` on the agent and `libf4kvs_ffi.so`. If a new dependency appears, rebuild:

```bash
make unikraft-build
```

Inspect the initrd contents under `.unikraft/build/`.

### f4kvs-ffi not found during Docker build

```bash
make unikraft-prepare
# or set F4KVS_ROOT in .env to your local checkout
```

### Agent exits immediately / syscall errors

Unikraft **binary compatibility** on `arm64` is newer than `x86_64`. If you hit syscall gaps, try `x86_64` with `-W` (software emulation) to compare, or report upstream. Increase memory: `UNIKRAFT_MEMORY=1Gi`.

### CGO note

The agent links **f4kvs-ffi** and **tantivy-go** via CGO. `Dockerfile.unikraft` builds a **static PIE** binary (`-buildmode=pie -extldflags '-static-pie'`) with only the `.a` archives — no runtime `.so` or `ld-linux` loader. Unikraft's app-elfloader requires PIE and cannot reliably run dynamically linked CGO binaries that depend on `ld-linux-x86-64.so.2`.

### `ELF executable is not position-independent`

Rebuild after pulling this fix:

```bash
make unikraft-build
make unikraft-run
```

### `Failed to load program interpreter /lib64/ld-linux-x86-64.so.2`

The agent is built as a **static PIE** binary (no dynamic linker at runtime). If you still see this, clear Kraftkit's rootfs cache and rebuild:

```bash
rm -rf .unikraft/build
make unikraft-build
make unikraft-run
```

## Architecture support

| Target | Status |
|--------|--------|
| `qemu/x86_64` | **Default** — `unikraft.org/base:latest` is published for this target |
| `qemu/arm64` | Declared in [`Kraftfile`](../Kraftfile); use `UNIKRAFT_ARCH=arm64 make unikraft-run` once the catalog ships `base:latest` for arm64 |

On Apple Silicon today, `make unikraft-run` uses `x86_64` with `-W` (QEMU TCG). First run is slow while the rootfs builds and the emulator starts.
