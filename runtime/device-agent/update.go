// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type adminUpdateStatus struct {
	Busy    bool   `json:"busy"`
	State   string `json:"state"`
	Version string `json:"version,omitempty"`
}

func (a *admin) slotRoot() string {
	root := filepath.Join(a.dir, "updates")
	if os.Getenv("XIAONEXUS_SLOT_ROOT") != root {
		return ""
	}
	return root
}

func (a *admin) updatePending() bool {
	root := a.slotRoot()
	if root == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(root, "pending"))
	return err == nil
}

// Caller holds a.mu.
func (a *admin) startUpdate(mode string) bool {
	if a.credential.Initial || a.update.Busy || a.updatePending() || (mode == "apply" && a.slotRoot() == "") {
		return false
	}
	a.update = adminUpdateStatus{Busy: true, State: "checking"}
	go a.runUpdate(mode)
	return true
}

func (a *admin) runUpdate(mode string) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	client := updateHTTPClient()
	defer client.CloseIdleConnections()
	m, err := findUpdate(ctx, client)
	state := "available"
	if mode != "check" && err == nil {
		err = stageUpdate(ctx, client, a.dir, m)
		state = "staged"
		if mode == "apply" && err == nil {
			err = saveAdminFile(a.slotRoot(), "pending", []byte(m.Version+"\n"))
			state = "pending"
		}
	}
	if err != nil {
		state = "unavailable"
	}
	a.mu.Lock()
	a.update = adminUpdateStatus{State: state, Version: m.Version}
	a.event("update_" + state)
	a.mu.Unlock()
}

const releaseRoot = "https://github.com/jarod360/XiaoNexus/releases/"

// Set by the release build. Empty key means update requests are unavailable.
var installedVersion, installedFlavor, updatePublicKey string

func updateHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 3 * time.Minute,
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) > 4 || r.URL.Scheme != "https" || r.URL.User != nil || r.URL.Port() != "" {
				return errors.New("invalid update redirect")
			}
			switch r.URL.Hostname() {
			case "github.com":
				if !strings.HasPrefix(r.URL.Path, "/jarod360/XiaoNexus/releases/") {
					return errors.New("invalid release path")
				}
			case "release-assets.githubusercontent.com", "objects.githubusercontent.com":
			default:
				return errors.New("invalid release host")
			}
			return nil
		},
	}
}

func updateAsset(ctx context.Context, c *http.Client, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, errors.New("release asset unavailable")
	}
	return res.Body, nil
}

func smallUpdateAsset(ctx context.Context, c *http.Client, url string, max int64) ([]byte, error) {
	body, err := updateAsset(ctx, c, url)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	data, err := io.ReadAll(io.LimitReader(body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, errors.New("release metadata too large")
	}
	return data, nil
}

func findUpdate(ctx context.Context, c *http.Client) (updateManifest, error) {
	key, err := hex.DecodeString(updatePublicKey)
	if err != nil || len(key) != ed25519.PublicKeySize || (installedFlavor != "standard" && installedFlavor != "doorbell") {
		return updateManifest{}, errors.New("trusted update key or variant unavailable")
	}
	path := releaseRoot + "latest/download/lx04-" + installedFlavor + ".json"
	raw, err := smallUpdateAsset(ctx, c, path, 4096)
	if err != nil {
		return updateManifest{}, err
	}
	sig, err := smallUpdateAsset(ctx, c, path+".sig", ed25519.SignatureSize)
	if err != nil {
		return updateManifest{}, err
	}
	return verifyUpdateManifest(raw, sig, key, installedVersion, installedFlavor)
}

func checkUpdateSpace(ctx context.Context, dir string, archiveBytes int64) error {
	output, err := exec.CommandContext(ctx, "/system/bin/df", "-k", dir).Output()
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 6 {
		return errors.New("unrecognized disk capacity output")
	}
	freeKiB, err := strconv.ParseInt(fields[len(fields)-3], 10, 64)
	if err != nil || freeKiB < (archiveBytes+maxUpdateUnpacked+(16<<20))/1024+1 {
		return errors.New("insufficient update space")
	}
	return nil
}

func stageUpdate(ctx context.Context, c *http.Client, dir string, m updateManifest) error {
	if err := checkUpdateSpace(ctx, dir, m.Size); err != nil {
		return err
	}
	root := filepath.Join(dir, "updates")
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	destination := filepath.Join(root, m.Version)
	metadata, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if saved, err := os.ReadFile(filepath.Join(destination, "manifest.json")); err == nil {
		if bytes.Equal(saved, metadata) {
			return nil
		}
		return errors.New("staged release metadata changed")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	archive, err := os.CreateTemp(root, ".archive-")
	if err != nil {
		return err
	}
	defer os.Remove(archive.Name())
	defer archive.Close()
	name := fmt.Sprintf("XiaoNexus-%s-LX04-%s-armv7.tar.gz", m.Version, m.Flavor)
	body, err := updateAsset(ctx, c, releaseRoot+"download/"+m.Version+"/"+name)
	if err != nil {
		return err
	}
	defer body.Close()
	if err := saveVerifiedArchive(archive, body, m); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(root, ".stage-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := extractUpdate(stage, archive); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stage, "VERSION"), []byte(m.Version+"\n"), 0600); err != nil {
		return err
	}
	if err := saveAdminFile(stage, "manifest.json", metadata); err != nil {
		return err
	}
	return os.Rename(stage, destination)
}
