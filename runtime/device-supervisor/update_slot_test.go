// SPDX-License-Identifier: Apache-2.0
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSlotTrialCommitsOnlyAfterContinuousHealth(t *testing.T) {
	root := t.TempDir()
	active := filepath.Join(root, "active")
	if err := os.WriteFile(active, []byte("V51.7.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	trial := slotControl{root: root, trial: "V51.8.0", deadline: now.Add(45 * time.Second)}
	healthy := receiverStatus{Healthy: true, NoGlobalMute: true}
	read := func() string {
		b, err := os.ReadFile(active)
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(b))
	}
	for _, sample := range []struct {
		elapsed time.Duration
		ready   bool
		status  receiverStatus
	}{
		{time.Second, true, healthy},
		{12 * time.Second, false, healthy},
		{13 * time.Second, true, healthy},
		{27 * time.Second, true, healthy},
	} {
		if err := trial.verify(sample.ready, sample.status, now.Add(sample.elapsed)); err != nil {
			t.Fatal(err)
		}
		if read() != "V51.7.0" {
			t.Fatal("committed before continuous health window")
		}
	}
	if err := trial.verify(true, healthy, now.Add(28*time.Second)); err != nil {
		t.Fatal(err)
	}
	if trial.trial != "" || read() != "V51.8.0" {
		t.Fatal("verified candidate not committed")
	}
	failed := slotControl{root: root, trial: "V51.9.0", deadline: now}
	if failed.verify(false, receiverStatus{}, now) == nil || read() != "V51.8.0" {
		t.Fatal("failed candidate changed active slot")
	}
}

func TestAudioIdleUsesCurrentPlayersAndRecording(t *testing.T) {
	const fixture = "players:\nID:3 -- type:AudioTrack -- state:stopped -- attr:x\n\nducked players piids:\nRecordActivityMonitor dump time: 12:00\n\nplayer piid:3 state:started\n"
	if audioDumpState(fixture) != "idle" {
		t.Fatal("history counted as current playback")
	}
	for _, dump := range []string{
		strings.Replace(fixture, "state:stopped", "state:started", 1),
		strings.Replace(fixture, "state:stopped", "state:unknown", 1),
		strings.Replace(fixture, "12:00\n\n", "12:00\nactive recording\n\n", 1),
		strings.Replace(fixture, "players:", "unrecognized heading:", 1),
	} {
		if audioDumpState(dump) == "idle" {
			t.Fatal("active or unknown audio state marked idle")
		}
	}
}

func TestAudioDumpStateIdentifiesLX04Recording(t *testing.T) {
	const dump = "players:\nID:47 -- type:android.media.AudioTrack -- u/pid:10028/1052 -- state:paused -- attr:AudioAttributes\n\nducked players piids:\n\nmuted player piids:\nRecordActivityMonitor dump time: 下午6:36:41\n  session:153 -- source:VOICE_COMMUNICATION -- uid:10024 -- patch:4 -- pack:com.xiaomi.smarthome -- format client=1ch 8000Hz\n\n"
	if got := audioDumpState(dump); got != "audio-recording" {
		t.Fatalf("audio state = %s", got)
	}
}
