#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
export PATH="/d/Program Files/Go/bin:/mingw64/bin:/usr/bin:$PATH"
export CGO_ENABLED=1
export CC=/mingw64/bin/gcc.exe
export CGO_CFLAGS="-I/mingw64/include"
export CGO_LDFLAGS="-L/mingw64/lib -lopus"
go build -o bin/server.exe ./cmd/server
echo "[build-opus.sh] compile OK"
