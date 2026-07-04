#!/usr/bin/env bash
# Size-optimized build. Cross-compile by exporting GOOS/GOARCH first; cross
# outputs get per-target names (bin/nanogitdb-<os>-<arch>[.exe]), the native
# build stays bin/nanogitdb. Pure-Go only: CGO_ENABLED=0 keeps cross-compiles
# toolchain-free.
set -euo pipefail
cd "$(dirname "$0")/.."  ## repo root; this script lives in cicd/

out="nanogitdb"
if [[ -n "${GOOS:-}${GOARCH:-}" ]] && [[ "${GOOS:-$(go env GOHOSTOS)}-${GOARCH:-$(go env GOHOSTARCH)}" != "$(go env GOHOSTOS)-$(go env GOHOSTARCH)" ]]; then
	out="nanogitdb-${GOOS:-$(go env GOHOSTOS)}-${GOARCH:-$(go env GOHOSTARCH)}"
	[[ "${GOOS:-}" == "windows" ]]  &&  out="${out}.exe"
fi

# The module root lives under source/ (so vendored deps sit under source/ too).
# -mod=vendor: build only from the committed vendor/ tree, never the network.
mkdir -p bin
( cd source && CGO_ENABLED=0 go build -mod=vendor -trimpath -ldflags="-s -w" -o "../bin/${out}" ./cmd/nanogitdb )
# ls, not du: on compressing filesystems (zfs) du reports allocated, not real, size.
echo "built: $(ls -lh "bin/${out}" | awk '{print $5}')	bin/${out}"
