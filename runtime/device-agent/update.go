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
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

type adminUpdateStatus struct {
	Busy    bool   `json:"busy"`
	State   string `json:"state"`
	Version string `json:"version,omitempty"`
}

type voiceUpdateOffer struct {
	dialog, version string
	expires time.Time
}

func (a *admin) updateOfferDialog() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if time.Now().Before(a.voiceOffer.expires) {
		return a.voiceOffer.dialog
	}
	return ""
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

func (a *admin) updateReadyToInstall() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return !a.update.Busy && a.updatePending()
}

type oldUpdateSlot struct {
	Version string `json:"version"`
	Bytes   int64  `json:"bytes"`
}

// Caller holds a.mu, which also serializes staging and scheduling installs.
func (a *admin) oldUpdateSlots() ([]oldUpdateSlot, error) {
	root := a.slotRoot()
	if root == "" || a.credential.Initial || a.rebooting || a.update.Busy {
		return nil, errors.New("当前无法清理旧版本。")
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() {
		return nil, errors.New("升级目录不可用。")
	}
	active, err := os.ReadFile(filepath.Join(root, "active"))
	current, versionErr := versionParts(installedVersion)
	if err != nil || versionErr != nil || strings.TrimSpace(string(active)) != installedVersion {
		return nil, errors.New("当前版本尚未确认，无法清理。")
	}
	keep := make(map[string]bool)
	for _, name := range []string{"pending", "trial", "manual-previous"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, errors.New("无法确认升级状态。")
		}
		version := strings.TrimSpace(string(data))
		if !releaseVersion.MatchString(version) {
			return nil, errors.New("无法确认升级状态。")
		}
		if name == "pending" || (name == "trial" && version != installedVersion) {
			return nil, errors.New("请等升级和试运行完成后再清理。")
		}
		keep[version] = true
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, errors.New("无法读取旧版本。")
	}
	slots := make([]oldUpdateSlot, 0)
	for _, entry := range entries {
		name := entry.Name()
		version, err := versionParts(name)
		if !entry.IsDir() || err != nil || keep[name] || slices.Compare(version[:], current[:]) >= 0 {
			continue
		}
		slot := oldUpdateSlot{Version: name}
		if err := filepath.WalkDir(filepath.Join(root, name), func(_ string, item fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if item.Type().IsRegular() {
				info, err := item.Info()
				if err != nil {
					return err
				}
				slot.Bytes += info.Size()
			}
			return nil
		}); err != nil {
			return nil, errors.New("无法统计旧版本占用。")
		}
		slots = append(slots, slot)
	}
	return slots, nil
}

func (a *admin) serveOldUpdateSlots(w http.ResponseWriter, r *http.Request) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(30 * time.Second))
	slots, err := a.oldUpdateSlots()
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(slots)
		return
	}
	var versions []string
	if !readAdminJSON(w, r, &versions) || len(versions) == 0 {
		http.Error(w, "请先读取并确认要清理的旧版本。", http.StatusBadRequest)
		return
	}
	eligible := make(map[string]bool, len(slots))
	for _, slot := range slots {
		eligible[slot.Version] = true
	}
	for _, version := range versions {
		if !eligible[version] {
			http.Error(w, "旧版本列表已变化，请重新读取。", http.StatusConflict)
			return
		}
	}
	for _, version := range versions {
		if err := os.RemoveAll(filepath.Join(a.slotRoot(), version)); err != nil {
			http.Error(w, "清理未全部完成，请重新读取剩余旧版本。", http.StatusInternalServerError)
			return
		}
	}
	a.event("old_updates_removed")
	w.WriteHeader(http.StatusNoContent)
}

// Caller holds a.mu.
func (a *admin) startUpdate(mode string, announce func(string), dialog, expectedVersion string) bool {
	if a.credential.Initial || a.rebooting || a.update.Busy || a.updatePending() || (mode == "apply" && a.slotRoot() == "") {
		return false
	}
	a.update = adminUpdateStatus{Busy: true, State: "checking"}
	a.voiceOffer = voiceUpdateOffer{}
	go a.runUpdate(mode, announce, dialog, expectedVersion)
	return true
}

func (a *admin) runUpdate(mode string, announce func(string), dialog, expectedVersion string) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	client := updateHTTPClient()
	defer client.CloseIdleConnections()
	m, err := findUpdate(ctx, client)
	if err == nil && expectedVersion != "" && m.Version != expectedVersion {
		err = errors.New("available update changed after confirmation")
	}
	state := "available"
	message := "发现新版本。要安装，请说立即升级。"
	if mode != "check" && err == nil {
		if mode == "apply" && announce != nil {
			announce("正在更新，请勿断电。")
		}
		err = stageUpdate(ctx, client, a.dir, m)
		state = "staged"
		if mode == "apply" && err == nil {
			err = saveAdminFile(a.slotRoot(), "pending", []byte(m.Version+"\n"))
			state = "pending"
			message = "更新已准备好。音箱空闲后会自动安装，请勿断电。"
		}
	}
	if err != nil {
		switch {
		case errors.Is(err, errNoNewerUpdate):
			message = "当前没有可用的新版本。"
		case state != "available":
			message = "更新未能准备完成，请稍后再试。"
		default:
			message = "检查更新失败，请稍后再试。"
		}
		state = "unavailable"
	}
	offer := mode == "check" && dialog != "" && err == nil && a.slotRoot() != ""
	if announce != nil && !offer {
		announce(message)
	}
	a.mu.Lock()
	a.update = adminUpdateStatus{State: state, Version: m.Version}
	if offer {
		a.voiceOffer = voiceUpdateOffer{dialog:dialog, version:m.Version, expires:time.Now().Add(15*time.Second)}
	}
	a.event("update_" + state)
	a.mu.Unlock()
}

// scheduleReboot is called with a.mu held after session and Origin checks.
func (a *admin) scheduleReboot(w http.ResponseWriter) {
	if a.credential.Initial {
		http.Error(w, "请先更改初始管理密码。", http.StatusForbidden)
		return
	}
	if a.rebooting || a.update.Busy || a.updatePending() || a.updateTrialUnconfirmed() {
		http.Error(w, "有更新或重启正在处理，请稍后再试。", http.StatusConflict)
		return
	}
	a.rebooting = true
	a.event("device_reboot_requested")
	w.WriteHeader(http.StatusNoContent)
	_ = http.NewResponseController(w).Flush()
	time.AfterFunc(time.Second, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := exec.CommandContext(ctx, "/system/bin/reboot").Run(); err != nil {
			log.Print("device reboot command failed")
			a.event("device_reboot_failed")
		} else {
			// Android may acknowledge the request before stopping the agent.
			time.Sleep(15 * time.Second)
		}
		a.mu.Lock()
		a.rebooting = false
		a.mu.Unlock()
	})
}

func (a *admin) updateTrialUnconfirmed() bool {
	root := a.slotRoot()
	if root == "" {
		return false
	}
	trial, err := os.ReadFile(filepath.Join(root, "trial"))
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	if err != nil {
		return true
	}
	active, err := os.ReadFile(filepath.Join(root, "active"))
	return err != nil || strings.TrimSpace(string(trial)) != installedVersion ||
		strings.TrimSpace(string(active)) != installedVersion
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
