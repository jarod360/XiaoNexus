#!/bin/sh
# Build-only LX04 candidate bundles. No ADB or device changes.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
VERSION=$(tr -d '\r\n' < "$ROOT/VERSION")
case "$VERSION" in V51.*|V52.0.0|V52.0.1|V52.0.2|V52.0.3|V52.0.4) ;; *) echo 'Expected V51.x or the V52.0 RC build identifier' >&2; exit 1 ;; esac
python3 "$ROOT/scripts/prepare-legacy-variants.py"
OUT="$ROOT/dist"
mkdir -p "$OUT"
cd "$ROOT/.build/legacy-agent-src/client"
go test ./cmd/xiaonexus-agent ./cmd/xiaonexus-supervisor ./miservice
go test -tags doorbell ./cmd/xiaonexus-agent
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags='-s -w' -o "$OUT/xiaonexus-supervisor-armv7" ./cmd/xiaonexus-supervisor
cp "$ROOT/runtime/frida/lx04_nexus_gate.js" "$OUT/lx04_nexus_gate.js"
for flavor in standard doorbell; do
  update_key=${XIAONEXUS_UPDATE_PUBLIC_KEY:-}
  case "$update_key" in
    *[!0-9a-f]*) echo 'Invalid update public key' >&2; exit 1 ;;
  esac
  if [ -n "$update_key" ] && [ "${#update_key}" -ne 64 ]; then echo 'Invalid update public key length' >&2; exit 1; fi
  agent_flags="-s -w -X main.installedVersion=$VERSION -X main.installedFlavor=$flavor -X main.updatePublicKey=$update_key"
  tag=''
  if [ "$flavor" = doorbell ]; then tag='doorbell'; fi
  if [ -n "$tag" ]; then
    CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -tags "$tag" -trimpath -ldflags="$agent_flags" -o "$OUT/xiaonexus-$flavor-armv7" ./cmd/xiaonexus-agent
  else
    CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags="$agent_flags" -o "$OUT/xiaonexus-$flavor-armv7" ./cmd/xiaonexus-agent
  fi
  PKG="$OUT/package-$flavor"
  mkdir -p "$PKG"
  mkdir -p "$PKG/bin" "$PKG/frida" "$PKG/resident"
  cp "$OUT/xiaonexus-$flavor-armv7" "$PKG/bin/xiaonexus-agent"
  cp "$OUT/xiaonexus-supervisor-armv7" "$PKG/bin/xiaonexus-supervisor"
  cp "$OUT/lx04_nexus_gate.js" "$PKG/frida/lx04_nexus_gate.js"
  cp "$ROOT/runtime/device-resident/"*.sh "$ROOT/runtime/device-resident/"*.js "$PKG/resident/"
  cp "$ROOT/scripts/Install_Resident_LX04.ps1" "$PKG/Install_Resident_LX04.ps1"
  cp "$ROOT/scripts/Retire_Legacy_LX04.ps1" "$PKG/Retire_Legacy_LX04.ps1"
  cp "$ROOT/LICENSE" "$ROOT/NOTICE" "$ROOT/THIRD_PARTY.md" "$PKG/"
  cp "$ROOT/scripts/Preflight_XiaoNexus.ps1" "$PKG/Preflight_XiaoNexus.ps1"
  cp "$ROOT/scripts/Audit_Legacy_XiaoSeek.ps1" "$PKG/Audit_Legacy_XiaoSeek.ps1"
  printf '%s\n' "$VERSION" > "$PKG/VERSION"
  printf '%s\n' \
    "XiaoNexus $VERSION $flavor ARMv7 experimental build" \
    '1. Do not uninstall the working XiaoSeek agent.' \
    '2. Windows PC: extract under D:\Codex and run the read-only Preflight_XiaoNexus.ps1 first.' \
    '3. Native attach needs a separate OFFICIAL frida-inject 16.6.6 android-arm binary.' \
    '4. Resident service is OPT-IN; review Install_Resident_LX04.ps1 and use -Activate.' \
    '5. Preserve old install until reboot, failure recovery and rollback are independently verified.' \
    '6. On the same LAN, open http://<LX04-IP>:8765/. Initial password: XiaoNexus-LX04. Use a trusted LAN.' \
    > "$PKG/README_TEST.txt"
  (cd "$PKG" && sha256sum bin/xiaonexus-agent bin/xiaonexus-supervisor frida/lx04_nexus_gate.js resident/* Install_Resident_LX04.ps1 Retire_Legacy_LX04.ps1 > SHA256SUMS)
  tar -czf "$OUT/XiaoNexus-$VERSION-LX04-$flavor-armv7.tar.gz" -C "$PKG" .
done
cd "$OUT"
sha256sum "XiaoNexus-$VERSION-LX04-standard-armv7.tar.gz" "XiaoNexus-$VERSION-LX04-doorbell-armv7.tar.gz" > SHA256SUMS
echo "BUILD_OK: $VERSION standard and doorbell experimental bundles (two self-checking archives)"
