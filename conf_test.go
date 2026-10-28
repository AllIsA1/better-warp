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

func testNode(s settings) node {
	return node{Dev: *sampleDevice(1), Endpoint: "162.159.192.1:2408", Name: s.Name}
}

func TestConfIsWhatAKnownGoodWarpConfigLooksLike(t *testing.T) {
	s := defaultSettings()
	conf := s.conf(testNode(s))
	want := []string{
		"# Cloudflare WARP\n[Interface]\n",
		"PrivateKey = cHJpdmF0ZXByaXZhdGVwcml2YXRlcHJpdmF0ZTEyMzQ=\n",
		"Address = 172.16.0.2/32, 2606:4700:110:8a36:df92:102a:9602:fba2/128\n",
		"DNS = 1.1.1.1, 1.0.0.1, 2606:4700:4700::1111, 2606:4700:4700::1001\n",
		"MTU = 1280\nJc = 4\nJmin = 40\nJmax = 70\n",
		"\n[Peer]\nPublicKey = " + testPeerKey + "\n",
		"AllowedIPs = 0.0.0.0/0, ::/0\nEndpoint = 162.159.192.1:2408\nPersistentKeepalive = 25\n",
	}
	for _, w := range want {
		if !strings.Contains(conf, w) {
			t.Errorf("config lacks %q:\n%s", w, conf)
		}
	}
	for _, bad := range []string{"S1 =", "S2 =", "H1 =", "I1 ="} {
		if strings.Contains(conf, bad) {
			t.Errorf("a plain WARP config must not carry %q:\n%s", bad, conf)
		}
	}
	if !strings.HasSuffix(conf, "\n") || strings.HasSuffix(conf, "\n\n") {
		t.Error("a config ends in exactly one newline")
	}
}

func TestConfFollowsTheSettings(t *testing.T) {
	s := defaultSettings()
	s.IPv6, s.MTU, s.Keepalive, s.Name = false, 1380, 0, "Name\nwith a break"
	s.AWG = awgSet{Jc: 2, Jmin: 10, Jmax: 20, I1: "<b 0xdeadbeef>"}
	n := testNode(s)
	n.Name = s.Name
	conf := s.conf(n)
	if strings.Contains(conf, "2606:") || strings.Contains(conf, "::/0") || strings.Contains(conf, "PersistentKeepalive") {
		t.Errorf("IPv6 and keepalive are off:\n%s", conf)
	}
	if !strings.Contains(conf, "Address = 172.16.0.2/32\n") || !strings.Contains(conf, "MTU = 1380\n") || !strings.Contains(conf, "I1 = <b 0xdeadbeef>\n") {
		t.Errorf("settings missing:\n%s", conf)
	}
	if strings.Contains(strings.Split(conf, "\n")[0], "\r") || strings.Count(conf, "# ") != 1 {
		t.Errorf("a name stays on one line:\n%s", conf)
	}
}

// decodeVpn reads the config out of a vpn:// link the way AmneziaVPN and the panel's page do.
func decodeVpn(t *testing.T, link string) (env map[string]any, conf string) {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(link, "vpn://"))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 6 || raw[4] != 0x78 || raw[5] != 0x01 {
		t.Fatalf("not qCompress of stored zlib: % x", raw[:min(8, len(raw))])
	}
	zr, err := zlib.NewReader(bytes.NewReader(raw[4:]))
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := io.ReadAll(zr)
	if uint32(len(payload)) != uint32(raw[0])<<24|uint32(raw[1])<<16|uint32(raw[2])<<8|uint32(raw[3]) {
		t.Error("the length in front of the zlib data is wrong")
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		t.Fatal(err)
	}
	cs := env["containers"].([]any)
	awg := cs[len(cs)-1].(map[string]any)["awg"].(map[string]any)
	var inner struct {
		Config string `json:"config"`
	}
	if err := json.Unmarshal([]byte(awg["last_config"].(string)), &inner); err != nil {
		t.Fatal(err)
	}
	return env, inner.Config
}

// onlyStoredBlocks checks that the zlib stream carries no compressed block: the subscription page reads the label of a
// link out of stored blocks only.
func onlyStoredBlocks(t *testing.T, link string) {
	t.Helper()
	raw, _ := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(link, "vpn://"))
	z := raw[4+2:] // after the length and the two zlib header bytes
	for {
		if len(z) < 5 {
			t.Fatal("truncated stored block")
		}
		final, btype := z[0]&1, (z[0]>>1)&3
		if btype != 0 {
			t.Fatalf("block type %d, want stored (0)", btype)
		}
		n := int(z[1]) | int(z[2])<<8
		z = z[5+n:]
		if final == 1 {
			return
		}
	}
}

func TestVpnLinkIsAnAmneziaContainerThatHoldsTheConfig(t *testing.T) {
	s := defaultSettings()
	n := testNode(s)
	conf := s.conf(n)
	link, err := s.vpnLink(n, conf)
	if err != nil {
		t.Fatal(err)
	}
	env, got := decodeVpn(t, link)
	if got != conf {
		t.Errorf("the config in the container differs:\n%s\nwant\n%s", got, conf)
	}
	onlyStoredBlocks(t, link)
	if env["description"] != "Cloudflare WARP" || env["defaultContainer"] != "amnezia-awg" || env["hostName"] != "162.159.192.1" || env["dns1"] != "1.1.1.1" {
		t.Errorf("envelope = %v", env)
	}
	awg := env["containers"].([]any)[0].(map[string]any)["awg"].(map[string]any)
	if awg["port"] != "2408" || awg["isThirdPartyConfig"] != true || awg["transport_proto"] != "udp" {
		t.Errorf("awg = %v", awg)
	}
	var inner map[string]any
	_ = json.Unmarshal([]byte(awg["last_config"].(string)), &inner)
	for k, want := range map[string]any{"client_priv_key": n.Dev.PrivateKey, "server_pub_key": testPeerKey, "Jc": "4", "Jmin": "40", "Jmax": "70", "mtu": "1280", "hostName": "162.159.192.1"} {
		if inner[k] != want {
			t.Errorf("last_config[%s] = %v, want %v", k, inner[k], want)
		}
	}
	if inner["port"] != float64(2408) {
		t.Errorf("port = %v", inner["port"])
	}
}

func TestConfLinkCarriesTheConfigAndTheName(t *testing.T) {
	s := defaultSettings()
	n := testNode(s)
	n.Name = "Cloudflare WARP 2 & more"
	conf := s.conf(n)
	link := confLink(conf, oneLine(n.Name))
	body, name, _ := strings.Cut(strings.TrimPrefix(link, "amneziawg://"), "#")
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil || string(raw) != conf {
		t.Fatalf("the link does not hold the config: %v", err)
	}
	if got, _ := url.PathUnescape(name); got != n.Name {
		t.Errorf("name = %q", got)
	}
	if !regexp.MustCompile(`^amneziawg://[A-Za-z0-9_-]+(#.*)?$`).MatchString(link) {
		t.Error("the page would not take this for a config link")
	}
}

func TestLegacyLinkIsTheOlderAmneziawgForm(t *testing.T) {
	s := defaultSettings()
	n := testNode(s)
	link := s.legacyLink(n)
	u, err := url.Parse(link)
	if err != nil {
		t.Fatalf("%v: %s", err, link)
	}
	if u.Scheme != "amneziawg" || u.Host != "162.159.192.1:2408" || u.Fragment != "Cloudflare WARP" {
		t.Errorf("link = %s", link)
	}
	key := u.User.Username()
	if key != n.Dev.PrivateKey {
		t.Errorf("the key in the userinfo = %q", key)
	}
	q := u.Query()
	for k, want := range map[string]string{
		"publickey": testPeerKey, "mtu": "1280", "keepalive": "25", "jc": "4", "jmin": "40", "jmax": "70",
		"address": "172.16.0.2/32,2606:4700:110:8a36:df92:102a:9602:fba2/128",
	} {
		if q.Get(k) != want {
			t.Errorf("%s = %q, want %q", k, q.Get(k), want)
		}
	}
	// A key with + / = must survive being a userinfo.
	n.Dev.PrivateKey = "a+b/c0123456789012345678901234567890123456="
	if u2, err := url.Parse(s.legacyLink(n)); err != nil {
		t.Fatal(err)
	} else if k := u2.User.Username(); k != n.Dev.PrivateKey {
		t.Errorf("key = %q", k)
	}
	// An IPv6 endpoint keeps its brackets.
	n.Endpoint = "[2606:4700:d0::a29f:c001]:2408"
	if !strings.Contains(s.legacyLink(n), "@[2606:4700:d0::a29f:c001]:2408?") {
		t.Errorf("v6 endpoint: %s", s.legacyLink(n))
	}
}

func TestLinesAreInTheOrderThePageGroupsThem(t *testing.T) {
	s := defaultSettings()
	lines := s.lines(testNode(s))
	if len(lines) != 3 {
		t.Fatalf("%d lines", len(lines))
	}
	confRe := regexp.MustCompile(`^amneziawg://[A-Za-z0-9_-]+(#.*)?$`)
	if !strings.HasPrefix(lines[0], "amneziawg://") || confRe.MatchString(lines[0]) {
		t.Errorf("the first line must be the legacy link: %s", lines[0])
	}
	if !strings.HasPrefix(lines[1], "vpn://") {
		t.Errorf("the second line must be vpn://: %s", lines[1])
	}
	if !confRe.MatchString(lines[2]) {
		t.Errorf("the third line must be the config link: %s", lines[2])
	}
	for _, l := range lines {
		if strings.ContainsAny(l, " \r\n\t") {
			t.Errorf("a line with whitespace: %q", l)
		}
	}
}
