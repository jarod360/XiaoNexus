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

    nexus = DEST / "client" / "cmd" / "xiaonexus-agent"
    nexus.mkdir(parents=True, exist_ok=True)
    for filename in ("main.go", "main_test.go"):
        shutil.copy2(ROOT / "runtime" / "device-agent" / filename, nexus / filename)
    # Guard source remains from the pinned XiaoSeek version, not a rewritten implementation.
    shutil.copy2(agent / "doorbell.go", nexus / "doorbell.go")
    shutil.copy2(stub, nexus / "doorbell_disabled.go")
    print("NEXUS_AGENT: independent answer receiver with optional doorbell guard")
    print("PREPARED: " + str(DEST))
    print("STANDARD: doorbell logic excluded by Go build tag")
    print("DOORBELL: original XiaoSeek guard retained under -tags doorbell")
    print("PINNED_SOURCE: " + PIN)

if __name__ == "__main__":
    main()
