#!/bin/sh
# Build two XiaoNexus ARMv7 bundles: receiver + supervisor + approved script.
# This is build-only: no ADB, no installation, no changes to a speaker.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
python3 "$ROOT/scripts/prepare-legacy-variants.py"
OUT="$ROOT/dist"
mkdir -p "$OUT"
cd "$ROOT/.build/legacy-agent-src/client"
go test ./cmd/xiaonexus-agent ./cmd/xiaonexus-supervisor
go test -tags doorbell ./cmd/xiaonexus-agent
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags="-s -w" -o "$OUT/XiaoNexus-LX04-bridge-standard-armv7" ./cmd/xiaonexus-agent
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -tags doorbell -trimpath -ldflags="-s -w" -o "$OUT/XiaoNexus-LX04-bridge-doorbell-armv7" ./cmd/xiaonexus-agent
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags="-s -w" -o "$OUT/xiaonexus-supervisor-armv7" ./cmd/xiaonexus-supervisor
cp "$ROOT/runtime/frida/lx04_nexus_gate.js" "$OUT/lx04_nexus_gate.js"
cd "$OUT"
sha256sum XiaoNexus-LX04-bridge-standard-armv7 XiaoNexus-LX04-bridge-doorbell-armv7 xiaonexus-supervisor-armv7 lx04_nexus_gate.js > SHA256SUMS
for flavor in standard doorbell; do
  tar -czf "XiaoNexus-LX04-bridge-$flavor-armv7.tar.gz" "XiaoNexus-LX04-bridge-$flavor-armv7" "xiaonexus-supervisor-armv7" "lx04_nexus_gate.js" "SHA256SUMS"
done
echo "BUILD_OK: separate agent variants plus shared ARMv7 supervisor"
echo "CAUTION: autonomous injection requires physical acceptance; default OBSERVE_ONLY"
