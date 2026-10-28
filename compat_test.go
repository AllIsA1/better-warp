// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// The lines have to behave next to two other pieces: Better LucX (which adds the amneziawg://<config> line after every
// vpn:// line it sees, and must find ours already there) and the panel's subscription page (which groups a node's
// forms by position). Both are written out here from their rules, so a change on either side shows up as a failure.

// lucxVpnConf is Better LucX's reading of a vpn:// line (amneziawg.go, vpnConf).
func lucxVpnConf(line string) string {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(strings.TrimPrefix(strings.TrimSpace(line), "vpn://"), "="))
	if err != nil || len(raw) <= 4 {
		return ""
	}
	zr, err := zlib.NewReader(bytes.NewReader(raw[4:]))
	if err != nil {
		return ""
	}
	payload, err := io.ReadAll(zr)
	if err != nil {
		return ""
	}
	var env struct {
		Containers []map[string]json.RawMessage `json:"containers"`
	}
	if json.Unmarshal(payload, &env) != nil || len(env.Containers) == 0 {
		return ""
	}
	last := env.Containers[len(env.Containers)-1]
	for _, key := range []string{"awg", "wireguard"} {
		var proto struct {
			LastConfig json.RawMessage `json:"last_config"`
		}
		if raw, ok := last[key]; ok && json.Unmarshal(raw, &proto) == nil {
			var asString string
			if json.Unmarshal(proto.LastConfig, &asString) == nil {
				var inner struct {
					Config string `json:"config"`
				}
				if json.Unmarshal([]byte(asString), &inner) == nil && strings.Contains(inner.Config, "[Interface]") {
					return strings.TrimRight(strings.TrimSpace(inner.Config), "\n") + "\n"
				}
			}
		}
	}
	return ""
}

// lucxConfLink is Better LucX's amneziawg://<config>#name for a config (awgConfLink).
func lucxConfLink(conf string) string {
	link := "amneziawg://" + base64.RawURLEncoding.EncodeToString([]byte(conf))
	for _, line := range strings.Split(conf, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "#"); ok && strings.TrimSpace(rest) != "" {
			return link + "#" + strings.ReplaceAll(url.QueryEscape(strings.TrimSpace(rest)), "+", "%20")
		}
	}
	return link
}

func TestBetterLucxFindsOurConfigLineAlreadyThere(t *testing.T) {
	s := defaultSettings()
	for _, name := range []string{"Cloudflare WARP", "Cloudflare WARP 2", "WARP — быстрый & надёжный"} {
		n := testNode(s)
		n.Name = name
		lines := s.lines(n)
		conf := lucxVpnConf(lines[1])
		if conf == "" {
			t.Fatalf("%s: Better LucX cannot read our vpn:// line", name)
		}
		if got := lucxConfLink(conf); got != lines[2] {
			t.Errorf("%s: it would add a different config line\n  ours   %s\n  theirs %s", name, lines[2], got)
		}
	}
}

// pageGroups is the page's grouping (awgGroups.ts): a vpn:// line takes the legacy link before it and the config link after it.
var confLinkRe = regexp.MustCompile(`^amneziawg://[A-Za-z0-9_-]+(#.*)?$`)

type pageGroup struct{ vpn, legacy, conf string }

func pageGroups(links []string) []pageGroup {
	var out []pageGroup
	for i, l := range links {
		if !strings.HasPrefix(l, "vpn://") {
			continue
		}
		g := pageGroup{vpn: l}
		if i > 0 && strings.HasPrefix(links[i-1], "amneziawg://") && !confLinkRe.MatchString(links[i-1]) {
			g.legacy = links[i-1]
		}
		if i+1 < len(links) && confLinkRe.MatchString(links[i+1]) {
			g.conf = links[i+1]
		}
		out = append(out, g)
	}
	return out
}

func TestThePageShowsEachNodeAsOneRowWithAllFourForms(t *testing.T) {
	r, w := seamRig(t)
	r.enable(t, func(s *settings) {
		s.Endpoints = []string{"162.159.192.1:2408", "188.114.97.5:500", "162.159.193.7:1701"}
	})
	links := w.apply("sub1", []string{"vless://x@h:443#one", "vpn://other", "vless://z@h:443#two"})
	groups := pageGroups(links)
	var warp []pageGroup
	for _, g := range groups {
		if g.vpn != "vpn://other" {
			warp = append(warp, g)
		}
	}
	if len(warp) != 3 {
		t.Fatalf("%d groups, want one per endpoint: %v", len(warp), links)
	}
	for i, g := range warp {
		if g.legacy == "" || g.conf == "" {
			t.Errorf("node %d: legacy=%v conf=%v: the page would not offer all the forms", i, g.legacy != "", g.conf != "")
		}
		// The fourth form, .conf, is read out of the vpn:// link by the page; it must be there and be the same config.
		if conf := lucxVpnConf(g.vpn); !strings.Contains(conf, "[Peer]") {
			t.Errorf("node %d: no .conf in the vpn:// link", i)
		}
	}
	// A foreign vpn:// line with nothing around it must not borrow our lines.
	for _, g := range groups {
		if g.vpn == "vpn://other" && (g.legacy != "" || g.conf != "") {
			t.Error("an unrelated vpn:// link took one of our lines into its group")
		}
	}
}
