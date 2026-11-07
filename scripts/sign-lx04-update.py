#!/usr/bin/env python3
"""Create online release metadata in the private LX04 signing environment."""
import argparse
import getpass
import hashlib
import json
from pathlib import Path
import re
import shutil
import subprocess
import tempfile


def sign_manifest(manifest: Path, signature: Path, key: Path) -> None:
    openssl = shutil.which("openssl")
    if openssl:
        subprocess.run(
            [openssl, "pkeyutl", "-sign", "-rawin", "-inkey", str(key),
             "-in", str(manifest), "-out", str(signature)],
            check=True,
        )
        return
    try:
        from cryptography.hazmat.primitives import serialization
        from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
    except ModuleNotFoundError as exc:
        raise SystemExit("Signing requires OpenSSL or the Python cryptography package") from exc

    pem = key.read_bytes()
    password = None
    if pem.startswith(b"-----BEGIN ENCRYPTED PRIVATE KEY-----"):
        password = getpass.getpass("Ed25519 PEM passphrase: ").encode("utf-8")
    private_key = serialization.load_pem_private_key(pem, password)
    if not isinstance(private_key, Ed25519PrivateKey):
        raise SystemExit("Expected a private Ed25519 PEM key")
    signature.write_bytes(private_key.sign(manifest.read_bytes()))

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--key", required=True, type=Path, help="private Ed25519 PEM")
    args = parser.parse_args()
    match = re.fullmatch(
        r"XiaoNexus-(V(?:0|[1-9][0-9]{0,5})\.(?:0|[1-9][0-9]{0,5})\."
        r"(?:0|[1-9][0-9]{0,5}))-LX04-(standard|doorbell)-armv7\.tar\.gz",
        args.archive.name,
    )
    if not match:
        parser.error("expected an LX04 release archive with an exact three-part version")
    version, flavor = match.groups()
    size = args.archive.stat().st_size
    if not 0 < size <= 48 * 1024 * 1024:
        parser.error("archive is outside the accepted size range")
    with args.archive.open("rb") as stream:
        digest = hashlib.file_digest(stream, "sha256").hexdigest()
    metadata = {
        "version": version,
        "hardware": "LX04",
        "arch": "armv7",
        "flavor": flavor,
        "size": size,
        "sha256": digest,
    }
    manifest = args.archive.with_name(f"lx04-{flavor}.json")
    signature = manifest.with_suffix(".json.sig")
    with tempfile.TemporaryDirectory(dir=manifest.parent) as tmp:
        temp_manifest = Path(tmp) / manifest.name
        temp_signature = Path(tmp) / signature.name
        temp_manifest.write_bytes(json.dumps(metadata, separators=(",", ":")).encode("ascii"))
        sign_manifest(temp_manifest, temp_signature, args.key)
        if temp_signature.stat().st_size != 64:
            parser.error("expected an Ed25519 signature")
        temp_signature.replace(signature)
        temp_manifest.replace(manifest)


if __name__ == "__main__":
    main()
