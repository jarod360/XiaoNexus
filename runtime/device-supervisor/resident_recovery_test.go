// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

//go:embed testdata/resident.sh
var residentRecoveryScript string

func TestResidentInterruptedTrialRecoveryOnLX04(t *testing.T) {
	if _, err := os.Stat("/system/bin/sh"); os.IsNotExist(err) {
		t.Skip("requires Android shell")
	} else if err != nil {
		t.Fatal(err)
	}

	const oldVersion, trialVersion, laterVersion = "V51.7.0", "V51.8.0", "V51.9.0"
	for _, tc := range []struct {
		name, active, pending, wantPending string
		wantRollback                       bool
	}{
		{"interrupted", oldVersion, trialVersion, "", true},
		{"newer-request-preserved", oldVersion, laterVersion, laterVersion, true},
		{"committed", trialVersion, laterVersion, laterVersion, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			write := func(name, data string, mode os.FileMode) {
				path := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(data), mode); err != nil {
					t.Fatal(err)
				}
			}
			for _, version := range []string{oldVersion, trialVersion} {
				for _, name := range []string{"bin/xiaonexus-agent", "bin/xiaonexus-supervisor", "frida/lx04_nexus_gate.js"} {
					write(filepath.Join("updates", version, name), "unused fixture\n", 0700)
				}
			}
			write("resident/efs_once.sh", "unused fixture\n", 0600)
			write("updates/active", tc.active+"\n", 0600)
			write("updates/trial", trialVersion+"\n", 0600)
			write("updates/pending", tc.pending+"\n", 0600)
			source := strings.ReplaceAll(residentRecoveryScript, "\r\n", "\n")
			baseLine := "base=/data/adb/xiaonexus"
			endRecovery := "# Avoid attaching while the OEM startup SDKs are still initializing/forking."
			if strings.Count(source, baseLine) != 1 || strings.Count(source, endRecovery) != 1 {
				t.Fatal("resident recovery harness boundaries changed")
			}
			source = strings.Replace(source, baseLine, "base=\"$1\"\npidof() { return 1; }", 1)
			source = strings.SplitN(source, endRecovery, 2)[0] + "printf '%s\\n' \"$runtime\"\nexit 0\n"
			write("resident-recovery-test.sh", source, 0600)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			output, err := exec.CommandContext(ctx, "/system/bin/sh", filepath.Join(root, "resident-recovery-test.sh"), root).CombinedOutput()
			if want := filepath.Join(root, "updates", tc.active); err != nil || strings.TrimSpace(string(output)) != want {
				t.Fatalf("selected runtime %q, want %q: %v", output, want, err)
			}
			read := func(name string) (string, error) {
				data, err := os.ReadFile(filepath.Join(root, name))
				return strings.TrimSpace(string(data)), err
			}
			if active, err := read("updates/active"); err != nil || active != tc.active {
				t.Fatalf("active=%q: %v", active, err)
			}
			if _, err := read("updates/trial"); !os.IsNotExist(err) {
				t.Fatalf("trial marker remains: %v", err)
			}
			pending, err := read("updates/pending")
			if tc.wantPending == "" {
				if !os.IsNotExist(err) {
					t.Fatalf("abandoned pending marker remains: %v", err)
				}
			} else if err != nil || pending != tc.wantPending {
				t.Fatalf("pending=%q: %v", pending, err)
			}
			log, err := read("diag/resident-status.log")
			if err != nil || strings.Contains(log, "UPDATE_INTERRUPTED_ROLLED_BACK") != tc.wantRollback {
				t.Fatal(fmt.Sprintf("unexpected recovery log: %q (%v)", log, err))
			}
		})
	}
}
