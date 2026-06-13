#!/usr/bin/env bash
# Bundle a dynamic PIE binary and its Debian/glibc dependencies into /rootfs.
set -euo pipefail

ROOTFS="${1:-/rootfs}"
AGENT="${2:-/agent}"
F4KVS_SO="${3:-/lib/libf4kvs_ffi.so}"

mkdir -p "${ROOTFS}/etc/ssl/certs" "${ROOTFS}/lib" "${ROOTFS}/src/lib"
cp "${AGENT}" "${ROOTFS}/agent"
cp "${F4KVS_SO}" "${ROOTFS}/lib/libf4kvs_ffi.so"
cp "${F4KVS_SO}" "${ROOTFS}/src/lib/libf4kvs_ffi.so"
cp -a /etc/ssl/certs/. "${ROOTFS}/etc/ssl/certs/"

copy_lib() {
  local path="$1"
  [ -n "${path}" ] || return 0
  [ -e "${path}" ] || return 0
  # Keep the path layout expected by the ELF/interpreter (e.g. /lib64/ld-linux-x86-64.so.2),
  # not readlink -f targets under /usr/lib.
  local dest="${ROOTFS}${path}"
  mkdir -p "$(dirname "${dest}")"
  cp -L "${path}" "${dest}"
}

case "$(uname -m)" in
  x86_64)
    seed=( \
      /lib64/ld-linux-x86-64.so.2 \
      /lib/x86_64-linux-gnu/libc.so.6 \
      /lib/x86_64-linux-gnu/libm.so.6 \
      /lib/x86_64-linux-gnu/libgcc_s.so.1 \
      /lib/x86_64-linux-gnu/libpthread.so.0 \
      /lib/x86_64-linux-gnu/libdl.so.2 \
      /lib/x86_64-linux-gnu/librt.so.1 \
      /lib/x86_64-linux-gnu/libresolv.so.2 \
      /lib/x86_64-linux-gnu/libnss_dns.so.2 \
      /lib/x86_64-linux-gnu/libnss_files.so.2 \
    )
    ;;
  aarch64)
    seed=( \
      /lib/ld-linux-aarch64.so.1 \
      /lib/aarch64-linux-gnu/libc.so.6 \
      /lib/aarch64-linux-gnu/libm.so.6 \
      /lib/aarch64-linux-gnu/libgcc_s.so.1 \
      /lib/aarch64-linux-gnu/libpthread.so.0 \
      /lib/aarch64-linux-gnu/libdl.so.2 \
      /lib/aarch64-linux-gnu/librt.so.1 \
      /lib/aarch64-linux-gnu/libresolv.so.2 \
      /lib/aarch64-linux-gnu/libnss_dns.so.2 \
      /lib/aarch64-linux-gnu/libnss_files.so.2 \
    )
    ;;
  *)
    echo "unsupported bundle arch: $(uname -m)" >&2
    exit 1
    ;;
esac

for path in "${seed[@]}"; do
  copy_lib "${path}"
done

while IFS= read -r path; do
  copy_lib "${path}"
done < <(
  LD_LIBRARY_PATH=/lib:/src/lib ldd "${AGENT}" "${F4KVS_SO}" 2>/dev/null \
    | awk '/=> \// { print $3 }' \
    | sort -u
)

interp="$(readelf -l "${AGENT}" | awk '/Requesting program interpreter/ { gsub(/\[|\]/, "", $NF); print $NF }')"
copy_lib "${interp}"

if [ -f /etc/ld.so.cache ]; then
  mkdir -p "${ROOTFS}/etc"
  cp /etc/ld.so.cache "${ROOTFS}/etc/ld.so.cache"
fi

echo "rootfs bundled:"
find "${ROOTFS}" -type f | sort
