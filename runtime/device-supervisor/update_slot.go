// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var errSwitchSlot = errors.New("requested update is ready for trial")
var slotVersion = regexp.MustCompile(`^V(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})$`)

type slotControl struct {
	root, trial          string
	deadline, readySince time.Time
}

func (s *slotControl) verify(ready bool, status receiverStatus, now time.Time) error {
	if s.trial == "" {
		return nil
	}
	if !now.Before(s.deadline) {
		return errors.New("update trial health timeout")
	}
	if !ready || !status.alive() {
		s.readySince = time.Time{}
		return nil
	}
	if s.readySince.IsZero() {
		s.readySince = now
	}
	if now.Sub(s.readySince) < 15*time.Second {
		return nil
	}
	if err := commitSlot(s.root, s.trial); err != nil {
		return err
	}
	s.trial = ""
	return nil
}

func commitSlot(root, version string) error {
	if root == "" || !slotVersion.MatchString(version) {
		return errors.New("invalid trial slot")
	}
	file, err := os.CreateTemp(root, ".active-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, err = file.WriteString(version + "\n")
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(file.Name(), filepath.Join(root, "active")); err != nil {
		return err
	}
	directory, err := os.Open(root)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func androidAudioState(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, 1200*time.Millisecond)
	defer cancel()
	data, err := exec.CommandContext(ctx, "/system/bin/dumpsys", "audio").Output()
	if err != nil {
		return "audio-state-unavailable"
	}
	return audioDumpState(string(data))
}

func audioDumpState(dump string) string {
	players, playersDone, records, recordsDone := false, false, false, false
	for _, line := range strings.Split(dump, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "players:" && !playersDone:
			players = true
		case players && line == "ducked players piids:":
			players, playersDone = false, true
		case players && line != "":
			_, state, ok := strings.Cut(line, " -- state:")
			if !strings.HasPrefix(line, "ID:") || !ok {
				return "audio-format-unknown"
			}
			state, _, ok = strings.Cut(state, " -- attr:")
			if !ok {
				return "audio-format-unknown"
			}
			switch state {
			case "idle", "paused", "stopped", "released":
			case "started":
				return "audio-playing"
			default:
				return "audio-format-unknown"
			}
		case strings.HasPrefix(line, "RecordActivityMonitor dump time:") && !recordsDone:
			records = true
		case records:
			if line != "" {
				if strings.HasPrefix(line, "session:") {
					return "audio-recording"
				}
				return "audio-format-unknown"
			}
			records, recordsDone = false, true
		}
	}
	if playersDone && recordsDone {
		return "idle"
	}
	return "audio-format-unknown"
}
