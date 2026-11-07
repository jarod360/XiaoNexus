// SPDX-License-Identifier: Apache-2.0
package main

import (
	"strings"
	"testing"
	"time"
)

func TestCustomExpansionNeedsImmediateKnowledgeAndPreservesNativeActions(t *testing.T) {
	now := time.Now()
	first, second, third := strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 32)
	for _, test := range []struct {
		name, phrase           string
		delay                  time.Duration
		intervening, knowledge bool
		want                   string
	}{
		{"prior knowledge", "帮我加点背景", time.Second, false, true, "arm_expand"},
		{"no knowledge", "帮我加点背景", time.Second, false, false, "arm"},
		{"stale knowledge", "帮我加点背景", 61 * time.Second, false, true, "arm"},
		{"intervening request", "帮我加点背景", time.Second, true, true, "arm"},
		{"native action", "播放一首歌", time.Second, false, true, "arm"},
	} {
		t.Run(test.name, func(t *testing.T) {
			elapsed := time.Duration(0)
			r := newRouter(func() time.Time { return now.Add(elapsed) })
			r.expansions = []string{test.phrase}
			r.feed(instruction{"asr", first, "太阳系有多少颗行星"})
			if test.knowledge {
				r.feed(instruction{"knowledge", first, "八颗"})
			}
			if test.intervening {
				r.feed(instruction{"asr", second, "今天星期几"})
			}
			elapsed = test.delay
			got := r.feed(instruction{"asr", third, test.phrase})
			if got.kind != test.want {
				t.Fatalf("got %s, want %s", got.kind, test.want)
			}
			if got.kind == "arm_expand" {
				routed := r.gate(third)
				if routed.kind != "expand" || routed.answer != "八颗" {
					t.Fatalf("wrong expansion: %+v", routed)
				}
			}
		})
	}
}
