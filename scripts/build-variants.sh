#!/bin/sh
# Builds two isolated ARMv7 previews; never contacts, stops, or flashes a device.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
python3 "$ROOT/scripts/prepare-legacy-variants.py"
OUT="$ROOT/dist"
mkdir -p "$OUT"
cd "$ROOT/.build/legacy-agent-src/client"
go test ./cmd/lx04-agent
go test -tags doorbell ./cmd/lx04-agent
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags="-s -w" \
    -o "$OUT/XiaoNexus-legacy-LX04-standard-armv7" ./cmd/lx04-agent
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -tags doorbell \
    -trimpath -ldflags="-s -w" -o "$OUT/XiaoNexus-legacy-LX04-doorbell-armv7" ./cmd/lx04-agent
cd "$OUT"
sha256sum XiaoNexus-legacy-LX04-standard-armv7 XiaoNexus-legacy-LX04-doorbell-armv7 > SHA256SUMS
for variant in standard doorbell; do
    tar -czf "XiaoNexus-legacy-LX04-$variant-armv7.tar.gz" \
        "XiaoNexus-legacy-LX04-$variant-armv7"
done
echo "BUILT: two legacy-agent ARMv7 preview variants (not v50.2 integrated runtime)"
