// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

type adminRules struct {
	Refusals   []string `json:"refusals"`
	Expansions []string `json:"expansions"`
	Direct     []string `json:"direct"`
}

func normalizeAdminReply(text string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ',', '，':
			return '，'
		case '!', '！':
			return '！'
		}
		// ECMAScript whitespace and line terminators, matching the Frida hook.
		if r == '\uFEFF' || (unicode.IsSpace(r) && r != '\u0085') {
			return -1
		}
		return r
	}, text)
}

func (r adminRules) validate() error {
	for _, list := range [][]string{r.Refusals, r.Expansions, r.Direct} {
		if len(list) > 12 {
			return errors.New("too many rules")
		}
		seen := make(map[string]bool)
		for _, phrase := range list {
			if strings.TrimSpace(phrase) != phrase || phrase == "" || utf8.RuneCountInString(phrase) > 80 ||
				strings.IndexFunc(phrase, unicode.IsControl) >= 0 || seen[phrase] {
				return errors.New("invalid rule")
			}
			seen[phrase] = true
		}
	}
	for _, prefix := range r.Direct {
		if n := utf8.RuneCountInString(prefix); n < 2 || n > 24 ||
			strings.IndexFunc(prefix, func(r rune) bool {
				return !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r >= '\u4e00' && r <= '\u9fff')
			}) >= 0 {
			return errors.New("invalid direct prefix")
		}
		for _, other := range r.Direct {
			if prefix != other && strings.HasPrefix(strings.ToLower(other), strings.ToLower(prefix)) {
				return errors.New("overlapping direct prefixes")
			}
		}
	}
	return nil
}

func (a *admin) loadRules() error {
	data, err := os.ReadFile(filepath.Join(a.dir, "admin-rules.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var rules adminRules
	if err := json.Unmarshal(data, &rules); err != nil {
		return err
	}
	if err := rules.validate(); err != nil {
		return err
	}
	a.rules.Store(&rules)
	return nil
}
