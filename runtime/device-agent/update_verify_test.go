// SPDX-License-Identifier: Apache-2.0
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func updateArchiveFixture(t *testing.T, machine uint16) *bytes.Buffer {
	t.Helper()
	elfHeader := make([]byte, 52)
	copy(elfHeader, []byte{0x7f, 'E', 'L', 'F', 1, 1, 1})
	binary.LittleEndian.PutUint16(elfHeader[16:], 2)  // ET_EXEC
	binary.LittleEndian.PutUint16(elfHeader[18:], 40) // EM_ARM
	binary.LittleEndian.PutUint32(elfHeader[20:], 1)
	binary.LittleEndian.PutUint16(elfHeader[40:], 52)
	executable := append([]byte(nil), elfHeader...)
	binary.LittleEndian.PutUint16(executable[18:], machine)
	data := new(bytes.Buffer)
	gzipWriter := gzip.NewWriter(data)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, body := range map[string][]byte{
		"./bin/xiaonexus-agent":    executable,
		"bin/xiaonexus-supervisor": elfHeader,
		"frida/lx04_nexus_gate.js": []byte("gate"),
		"../escape":                []byte("not extracted"),
	} {
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0600, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestExtractUpdateAllowsOnlyManagedARMFiles(t *testing.T) {
	root := t.TempDir()
	slot := filepath.Join(root, "slot")
	if err := os.Mkdir(slot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := extractUpdate(slot, updateArchiveFixture(t, 40)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "escape")); !os.IsNotExist(err) {
		t.Fatal("archive escaped its slot")
	}
	if err := extractUpdate(t.TempDir(), updateArchiveFixture(t, 3)); err == nil {
		t.Fatal("non-ARM binary accepted")
	}
}

func TestUpdateRejectsWrongSignatureTargetVersionAndDigest(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	content := "signed LX04 package"
	digest := sha256.Sum256([]byte(content))
	manifest := updateManifest{"V51.8.0", "LX04", "armv7", "doorbell", int64(len(content)), hex.EncodeToString(digest[:])}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(private, raw)
	if _, err := verifyUpdateManifest(raw, sig, public, "V51.7.0", "doorbell"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		version, flavor string
		tamper          bool
	}{
		{"V51.8.0", "doorbell", false},
		{"V51.9.0", "doorbell", false},
		{"V51.7.0", "standard", false},
		{"V51.7.0", "doorbell", true},
	} {
		candidate := raw
		if tc.tamper {
			candidate = append(append([]byte(nil), raw...), ' ')
		}
		if _, err := verifyUpdateManifest(candidate, sig, public, tc.version, tc.flavor); err == nil {
			t.Fatalf("accepted %+v", tc)
		}
	}
	f, err := os.CreateTemp(t.TempDir(), "archive")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := saveVerifiedArchive(f, strings.NewReader(content), manifest); err != nil {
		t.Fatal(err)
	}
	if err := saveVerifiedArchive(f, strings.NewReader(content+"!"), manifest); err == nil {
		t.Fatal("oversized archive accepted")
	}
	if err := saveVerifiedArchive(f, strings.NewReader(strings.Repeat("z", len(content))), manifest); err == nil {
		t.Fatal("invalid digest accepted")
	}
}

type releaseFixtureTransport func(*http.Request) (*http.Response, error)

func (f releaseFixtureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestSignedReleaseDownloadAndStageOnLX04(t *testing.T) {
	if _, err := os.Stat("/system/bin/df"); os.IsNotExist(err) {
		t.Skip("requires Android disk space probe")
	} else if err != nil {
		t.Fatal(err)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	oldKey, oldVersion, oldFlavor := updatePublicKey, installedVersion, installedFlavor
	updatePublicKey, installedVersion, installedFlavor = hex.EncodeToString(public), "V51.7.0", "doorbell"
	t.Cleanup(func() { updatePublicKey, installedVersion, installedFlavor = oldKey, oldVersion, oldFlavor })
	payload := updateArchiveFixture(t, 40).Bytes()
	digest := sha256.Sum256(payload)
	manifest := updateManifest{"V51.8.0", "LX04", "armv7", "doorbell", int64(len(payload)), hex.EncodeToString(digest[:])}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(private, raw)
	for _, variant := range []string{"valid", "bad-signature", "bad-archive"} {
		t.Run(variant, func(t *testing.T) {
			dir := t.TempDir()
			root := filepath.Join(dir, "updates")
			slot := filepath.Join(root, manifest.Version)
			requests := 0
			client := &http.Client{Transport: releaseFixtureTransport(func(req *http.Request) (*http.Response, error) {
				requests++
				var data []byte
				switch req.URL.String() {
				case releaseRoot + "latest/download/lx04-doorbell.json":
					data = raw
				case releaseRoot + "latest/download/lx04-doorbell.json.sig":
					data = sig
					if variant == "bad-signature" {
						data = append([]byte(nil), sig...)
						data[0] ^= 1
					}
				case releaseRoot + "download/V51.8.0/XiaoNexus-V51.8.0-LX04-doorbell-armv7.tar.gz":
					data = payload
					if variant == "bad-archive" {
						data = append([]byte(nil), payload...)
						data[0] ^= 1
					}
				default:
					t.Fatalf("unexpected release asset %s", req.URL)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data))}, nil
			})}
			found, err := findUpdate(context.Background(), client)
			if err == nil {
				err = stageUpdate(context.Background(), client, dir, found)
			}
			if variant != "valid" {
				if err == nil {
					t.Fatal("accepted damaged release")
				}
				wantRequests := 3
				if variant == "bad-signature" {
					wantRequests = 2
				}
				if requests != wantRequests {
					t.Fatalf("downloaded unexpected assets: %d", requests)
				}
				entries, listErr := os.ReadDir(root)
				if (listErr != nil && !os.IsNotExist(listErr)) || len(entries) != 0 {
					t.Fatalf("left update artifacts: %v, %v", entries, listErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{"VERSION", "manifest.json", "bin/xiaonexus-agent", "bin/xiaonexus-supervisor", "frida/lx04_nexus_gate.js"} {
				if _, err := os.Stat(filepath.Join(slot, path)); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := os.Stat(filepath.Join(root, "active")); !os.IsNotExist(err) {
				t.Fatal("staging changed active slot")
			}
			if requests != 3 {
				t.Fatalf("unexpected asset count %d", requests)
			}
		})
	}
}
