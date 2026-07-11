#!/usr/bin/env bash
# Size-optimized build. Cross-compile by exporting GOOS/GOARCH first; cross
# outputs get per-target names (bin/ngdb-<os>-<arch>[.exe]), the native
# build stays bin/ngdb. Pure-Go only: CGO_ENABLED=0 keeps cross-compiles
# toolchain-free.
set -euo pipefail
source "$(dirname "$0")/utility/include/cpu-limit.bash"  ## NGDB_JOBS + GOMAXPROCS (<=half cores)
cd "$(dirname "$0")/.."  ## repo root; this script lives in cicd/

out="ngdb"
if [[ -n "${GOOS:-}${GOARCH:-}" ]] && [[ "${GOOS:-$(go env GOHOSTOS)}-${GOARCH:-$(go env GOHOSTARCH)}" != "$(go env GOHOSTOS)-$(go env GOHOSTARCH)" ]]; then
	out="ngdb-${GOOS:-$(go env GOHOSTOS)}-${GOARCH:-$(go env GOHOSTARCH)}"
	[[ "${GOOS:-}" == "windows" ]]  &&  out="${out}.exe"
fi

# app.Version is authoritative in source; we only stamp app.Build with provenance
# (short commit + -dirty), empty-safe so a release binary from a clean tree reads clean.
build="$(git rev-parse --short HEAD 2>/dev/null || true)"
[[ -n "${build}" && -n "$(git status --porcelain 2>/dev/null)" ]]  &&  build="${build}-dirty"

# The module root lives under source/ (so vendored deps sit under source/ too).
# -mod=vendor: build only from the committed vendor/ tree, never the network.
mkdir -p bin
( cd source && CGO_ENABLED=0 go build -mod=vendor -p "${NGDB_JOBS}" -trimpath -ldflags="-s -w -X github.com/jim-collier/nano-git-db/app.Build=${build}" -o "../bin/${out}" ./cmd/ngdb )
# ls, not du: on compressing filesystems (zfs) du reports allocated, not real, size.
echo "built: $(ls -lh "bin/${out}" | awk '{print $5}')	bin/${out}"
