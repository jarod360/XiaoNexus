#!/usr/bin/env python3
"""Stage the pinned XiaoSeek agent in a disposable workspace; never alter the submodule."""
from pathlib import Path
import shutil
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / "legacy" / "XiaoSeek"
DEST = ROOT / ".build" / "legacy-agent-src"
PIN = "5c05b9ad090f21265a440160345ed54d95e24783"

def main() -> None:
    if not (SOURCE / "client" / "go.mod").is_file():
        sys.exit("MISSING_SUBMODULE: git submodule update --init --recursive")
    sha = subprocess.check_output(
        ["git", "-C", str(SOURCE), "rev-parse", "HEAD"], text=True
    ).strip()
    if sha != PIN:
        sys.exit("SUBMODULE_COMMIT_MISMATCH: expected " + PIN + ", got " + sha)
    if DEST.exists():
        shutil.rmtree(DEST)
    shutil.copytree(SOURCE, DEST, ignore=shutil.ignore_patterns(".git", "dist", "*.log"))
    agent = DEST / "client" / "cmd" / "lx04-agent"
    for filename in ("doorbell.go", "doorbell_test.go"):
        file = agent / filename
        original = file.read_text(encoding="utf-8")
        if original.startswith("//go:build"):
            sys.exit("UNEXPECTED_SOURCE_TAG: " + filename)
        file.write_text("//go:build doorbell\n\n" + original, encoding="utf-8")
    stub = agent / "doorbell_disabled.go"
    stub.write_text(
        "//go:build !doorbell\n\n"
        "// SPDX-License-Identifier: Apache-2.0\n"
        "package main\n\n"
        "import \"context\"\n\n"
        "type doorbellGuard struct{}\n"
        "func newDoorbellGuard(_ string) *doorbellGuard { return &doorbellGuard{} }\n"
        "func (*doorbellGuard) check(context.Context) bool { return false }\n"
        "func (*doorbellGuard) watch(context.Context, func()) {}\n",
        encoding="utf-8",
    )

    account = DEST / "client" / "miservice" / "account.go"
    original = account.read_text(encoding="utf-8")
    signature = "func (ma *Account) Login(sid string) error {"
    if original.count(signature) != 1:
        sys.exit("UNEXPECTED_ACCOUNT_LOGIN_SIGNATURE")
    modified = original.replace(signature, signature + "\n return ma.login(sid,true)\n}\nfunc (ma *Account) login(sid string, allowPassword bool) error {")
    marker = "if resp.Code != 0 {\n\t\tdata := url.Values{"
    if modified.count(marker) != 1:
        sys.exit("UNEXPECTED_ACCOUNT_PASSWORD_BRANCH")
    modified = modified.replace(marker, 'if resp.Code != 0 {\n\t\tif !allowPassword { return errors.New("cached passToken rejected; no password login") }\n\t\tdata := url.Values{')
    modified = modified.replace("ma.token.PassToken = resp.PassToken", 'if allowPassword || resp.PassToken != "" { ma.token.PassToken = resp.PassToken }')
    account.write_text(modified, encoding="utf-8")
    shutil.copy2(ROOT / "runtime/auth/cached_sid.go", DEST / "client/miservice/xiaonexus_cached_sid.go")
    shutil.copy2(ROOT / "runtime/auth/cached_sid_test.go", DEST / "client/miservice/xiaonexus_cached_sid_test.go")
    shutil.copy2(ROOT / "runtime/auth/xiaomi_cached_auth.go", DEST / "client/xiaonexus_cached_auth.go")

    nexus = DEST / "client" / "cmd" / "xiaonexus-agent"
    nexus.mkdir(parents=True, exist_ok=True)
    for filename in ("main.go", "main_test.go", "admin.go", "admin.html", "admin_test.go", "admin_rules.go", "admin_xiaomi.go", "update.go", "update_verify.go", "update_verify_test.go"):
        shutil.copy2(ROOT / "runtime" / "device-agent" / filename, nexus / filename)
    # Guard source remains from the pinned XiaoSeek version, not a rewritten implementation.
    shutil.copy2(agent / "doorbell.go", nexus / "doorbell.go")
    shutil.copy2(stub, nexus / "doorbell_disabled.go")
    sup = DEST / "client" / "cmd" / "xiaonexus-supervisor"
    sup.mkdir(parents=True, exist_ok=True)
    for filename in ("main.go", "main_test.go", "admin_rules_test.go", "update_slot.go", "update_slot_test.go", "resident_recovery_test.go"):
        shutil.copy2(ROOT / "runtime" / "device-supervisor" / filename, sup / filename)
    (sup / "testdata").mkdir(exist_ok=True)
    shutil.copy2(ROOT / "runtime/device-resident/resident.sh", sup / "testdata/resident.sh")
    print("NEXUS_SUPERVISOR: on-device official Frida 16.6.6 ARM injector")
    print("NEXUS_AGENT: independent answer receiver with optional doorbell guard")
    print("PREPARED: " + str(DEST))
    print("STANDARD: doorbell logic excluded by Go build tag")
    print("DOORBELL: original XiaoSeek guard retained under -tags doorbell")
    print("PINNED_SOURCE: " + PIN)

if __name__ == "__main__":
    main()
