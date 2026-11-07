// SPDX-License-Identifier: Apache-2.0
package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

const maxUpdateBytes = 48 << 20
const maxUpdateUnpacked = 96 << 20

type updateManifest struct {
	Version  string `json:"version"`
	Hardware string `json:"hardware"`
	Arch     string `json:"arch"`
	Flavor   string `json:"flavor"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
}

var releaseVersion = regexp.MustCompile(`^V(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})$`)

func versionParts(s string) ([3]int, error) {
	var v [3]int
	m := releaseVersion.FindStringSubmatch(s)
	if m == nil {
		return v, errors.New("invalid update version")
	}
	for i := range v {
		v[i], _ = strconv.Atoi(m[i+1])
	}
	return v, nil
}

func verifyUpdateManifest(raw, sig []byte, key ed25519.PublicKey, current, flavor string) (updateManifest, error) {
	var m updateManifest
	if len(key) != ed25519.PublicKeySize || len(raw) > 4096 || !ed25519.Verify(key, raw, sig) {
		return m, errors.New("invalid update signature")
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, err
	}
	digest, err := hex.DecodeString(m.SHA256)
	if err != nil || len(digest) != sha256.Size || m.Size < 1 || m.Size > maxUpdateBytes ||
		m.Hardware != "LX04" || m.Arch != "armv7" || (flavor != "standard" && flavor != "doorbell") || m.Flavor != flavor {
		return m, errors.New("incompatible update")
	}
	newer, err := versionParts(m.Version)
	if err != nil {
		return m, err
	}
	older, err := versionParts(current)
	if err != nil {
		return m, err
	}
	for i, n := range newer {
		if n > older[i] {
			return m, nil
		}
		if n < older[i] {
			break
		}
	}
	return m, errors.New("update must be newer than installed version")
}

func saveVerifiedArchive(dst *os.File, src io.Reader, m updateManifest) error {
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(dst, hash), io.LimitReader(src, m.Size+1))
	if err != nil {
		return err
	}
	if n != m.Size || hex.EncodeToString(hash.Sum(nil)) != m.SHA256 {
		return errors.New("update size or digest mismatch")
	}
	if _, err := dst.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return nil
}

// Extract only the files managed by the launcher into a new, private directory.
// The outer archive must already have passed signature and digest verification.
func extractUpdate(dst string, src io.Reader) error {
	gzipReader, err := gzip.NewReader(src)
	if err != nil {
		return err
	}
	defer gzipReader.Close()
	bounded := &io.LimitedReader{R: gzipReader, N: maxUpdateUnpacked + 1}
	archive := tar.NewReader(bounded)
	required := map[string]os.FileMode{
		"bin/xiaonexus-agent":      0700,
		"bin/xiaonexus-supervisor": 0700,
		"frida/lx04_nexus_gate.js": 0600,
	}
	seen := make(map[string]bool, len(required))
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		name := header.Name
		if len(name) > 2 && name[:2] == "./" {
			name = name[2:]
		}
		mode, managed := required[name]
		if !managed {
			continue
		}
		if seen[name] || header.Typeflag != tar.TypeReg || header.Size < 1 || header.Size > maxUpdateUnpacked {
			return errors.New("invalid archive entry")
		}
		target := filepath.Join(dst, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if err != nil {
			return err
		}
		_, err = io.Copy(f, archive)
		if err == nil {
			err = f.Sync()
		}
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
		seen[name] = true
	}
	if _, err := io.Copy(io.Discard, bounded); err != nil {
		return err
	}
	if bounded.N == 0 || len(seen) != len(required) {
		return fmt.Errorf("incomplete or oversized runtime archive")
	}
	for _, name := range []string{"xiaonexus-agent", "xiaonexus-supervisor"} {
		binary, err := elf.Open(filepath.Join(dst, "bin", name))
		if err != nil {
			return errors.New("invalid update executable")
		}
		arm := binary.Machine == elf.EM_ARM && binary.Class == elf.ELFCLASS32 && binary.Data == elf.ELFDATA2LSB
		binary.Close()
		if !arm {
			return errors.New("update executable is not ARM")
		}
	}
	return nil
}
