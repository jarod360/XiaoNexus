#!/bin/sh
# Two independent XiaoNexus ARMv7 receivers; no ADB or device mutations.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
python3 "$ROOT/scripts/prepare-legacy-variants.py"
OUT="$ROOT/dist"
mkdir -p "$OUT"
cd "$ROOT/.build/legacy-agent-src/client"
go test ./cmd/xiaonexus-agent
go test -tags doorbell ./cmd/xiaonexus-agent
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags="-s -w" \
    -o "$OUT/XiaoNexus-LX04-bridge-standard-armv7" ./cmd/xiaonexus-agent
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -tags doorbell -trimpath -ldflags="-s -w" \
    -o "$OUT/XiaoNexus-LX04-bridge-doorbell-armv7" ./cmd/xiaonexus-agent
cd "$OUT"
sha256sum XiaoNexus-LX04-bridge-standard-armv7 XiaoNexus-LX04-bridge-doorbell-armv7 > SHA256SUMS
for flavor in standard doorbell; do
  tar -czf "XiaoNexus-LX04-bridge-$flavor-armv7.tar.gz" "XiaoNexus-LX04-bridge-$flavor-armv7"
done
echo "BUILD_OK: independent host-assisted receivers; not an unattended speaker release"
