// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
)

func TestDefaultsAreValidAndKnownToWork(t *testing.T) {
	s := defaultSettings()
	if err := s.normalize(); err != nil {
		t.Fatal(err)
	}
	if s.Enabled {
		t.Error("installing the plugin must not change anyone's subscription")
	}
	if s.MTU != 1280 || s.Endpoints[0] != "engage.cloudflareclient.com:2408" || s.AWG.Jc != 4 || s.AWG.Jmin != 40 || s.AWG.Jmax != 70 || s.AWG.S1 != 0 || !s.AWG.plain() {
		t.Errorf("defaults = %+v", s)
	}
}

func TestPresetsSetTheJunkAndCustomKeepsWhatIsTyped(t *testing.T) {
	s := defaultSettings()
	s.Preset = "strong"
	s.AWG = awgSet{Jc: 1, Jmin: 1, Jmax: 1}
	if err := s.normalize(); err != nil || s.AWG.Jc != 8 {
		t.Errorf("a preset wins over the fields: %+v %v", s.AWG, err)
	}
	s.Preset = "custom"
	s.AWG = awgSet{Jc: 3, Jmin: 5, Jmax: 9}
	if err := s.normalize(); err != nil || s.AWG.Jc != 3 {
		t.Errorf("custom: %+v %v", s.AWG, err)
	}
	s.Preset = "nonsense"
	if s.normalize() == nil {
		t.Error("unknown preset")
	}
}

func TestSettingsRefuseWhatWouldBreakALineOrAConnection(t *testing.T) {
	cases := map[string]func(*settings){
		"name with a break":        func(s *settings) { s.Name = "a\nb" },
		"long name":                func(s *settings) { s.Name = strings.Repeat("я", 61) },
		"bad mode":                 func(s *settings) { s.Mode = "x" },
		"bad scope":                func(s *settings) { s.Scope = "x" },
		"four endpoints":           func(s *settings) { s.Endpoints = []string{"a.b:1", "a.b:2", "a.b:3", "a.b:4"} },
		"endpoint without port":    func(s *settings) { s.Endpoints = []string{"162.159.192.1"} },
		"endpoint port zero":       func(s *settings) { s.Endpoints = []string{"162.159.192.1:0"} },
		"endpoint with a space":    func(s *settings) { s.Endpoints = []string{"a b:2408"} },
		"endpoint with a slash":    func(s *settings) { s.Endpoints = []string{"a/b:2408"} },
		"mtu too small":            func(s *settings) { s.MTU = 100 },
		"mtu too large":            func(s *settings) { s.MTU = 9000 },
		"dns that is a name":       func(s *settings) { s.DNS = "one.one.one.one" },
		"dns with a line break":    func(s *settings) { s.DNS = "1.1.1.1\n[Peer]" },
		"keepalive negative":       func(s *settings) { s.Keepalive = -1 },
		"junk count too high":      func(s *settings) { s.Preset, s.AWG.Jc = "custom", 99 },
		"jmax under jmin":          func(s *settings) { s.Preset, s.AWG.Jmin, s.AWG.Jmax = "custom", 50, 10 },
		"i1 with a line break":     func(s *settings) { s.Preset, s.AWG.I1 = "custom", "<b 0x00>\n[Peer]" },
		"h1 with non-ascii":        func(s *settings) { s.Preset, s.AWG.H1 = "custom", "н" },
		"auto scan too often":      func(s *settings) { s.AutoScan.Hours = 9999 },
		"auto scan keeps too many": func(s *settings) { s.AutoScan.Count = 9 },
	}
	for name, mutate := range cases {
		s := defaultSettings()
		mutate(&s)
		if s.normalize() == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestEndpointsAndSelectionAreCleaned(t *testing.T) {
	s := defaultSettings()
	s.Endpoints = []string{" 162.159.192.1:2408 ", "[2606:4700:d0::1]:500", "engage.cloudflareclient.com:854"}
	s.Selected = []string{"a", " a ", "", "b c", "b", strings.Repeat("x", 200)}
	if err := s.normalize(); err != nil {
		t.Fatal(err)
	}
	if s.Endpoints[0] != "162.159.192.1:2408" {
		t.Errorf("endpoints = %v", s.Endpoints)
	}
	if len(s.Selected) != 2 || s.Selected[0] != "a" || s.Selected[1] != "b" {
		t.Errorf("selected = %v", s.Selected)
	}
	if !s.inScope("anything") {
		t.Error("scope all")
	}
	s.Scope = "selected"
	if !s.inScope("a") || s.inScope("c") {
		t.Error("scope selected")
	}
}

func TestNodeNamesNumberTheEndpoints(t *testing.T) {
	s := defaultSettings()
	if s.nodeName(0) != "Cloudflare WARP" || s.nodeName(1) != "Cloudflare WARP 2" || s.nodeName(2) != "Cloudflare WARP 3" {
		t.Error("names")
	}
}
