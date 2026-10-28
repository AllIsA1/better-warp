// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// inboundRig: a panel with one WARP inbound (id 5) that alice's subscription is on, bob's is not, carol's is switched off.
func inboundRig(t *testing.T, settings string) (*testRig, *warpLines) {
	t.Helper()
	r := newRig(t)
	r.panel.set(clientOn("alice", "sub1", true, 5, 9), clientOn("bob", "sub2", true, 9), clientOn("carol", "sub3", false, 5))
	r.panel.setInbounds(warpIn(5, "Poland WARP", true, settings))
	if _, err := r.app.prov.refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	_ = r.st.putDevice("sub1", sampleDevice(1))
	_ = r.st.putDevice("sub2", sampleDevice(2))
	return r, r.app.lines
}

func TestAnInboundDecidesWhoGetsWarpAndWithWhatSettings(t *testing.T) {
	r, w := inboundRig(t, `{"endpoints":["162.159.192.1:2408","188.114.97.5:500"],"mtu":1380,"ipv6":false,"keepalive":10,"preset":"strong","dns":"9.9.9.9"}`)
	if r.st.settings().Enabled {
		t.Fatal("the plugin's own switch is off: the inbound must be enough")
	}
	got := w.apply("sub1", []string{"vless://x#one"})
	if len(got) != 1+6 {
		t.Fatalf("%d lines, want the panel's line and two nodes of three lines", len(got))
	}
	conf := lucxVpnConf(got[2])
	for _, want := range []string{"# Poland WARP\n", "MTU = 1380\n", "Jc = 8\nJmin = 50\nJmax = 150\n", "DNS = 9.9.9.9\n", "Endpoint = 162.159.192.1:2408\n", "PersistentKeepalive = 10\n"} {
		if !strings.Contains(conf, want) {
			t.Errorf("config lacks %q:\n%s", want, conf)
		}
	}
	if strings.Contains(conf, "2606:") || strings.Contains(conf, "::/0") {
		t.Errorf("the inbound turned IPv6 off:\n%s", conf)
	}
	if !strings.Contains(lucxVpnConf(got[5]), "# Poland WARP 2\n") {
		t.Error("the second endpoint is the second node")
	}
	if len(w.apply("sub2", nil)) != 0 {
		t.Error("bob has a client on another inbound only")
	}
	if len(w.apply("sub3", nil)) != 0 {
		t.Error("carol is switched off")
	}
}

func TestEachWarpInboundGivesItsOwnNodes(t *testing.T) {
	r, w := inboundRig(t, `{"endpoints":["162.159.192.1:2408"]}`)
	r.panel.set(clientOn("alice", "sub1", true, 5, 6))
	r.panel.setInbounds(warpIn(5, "Poland", true, `{"endpoints":["162.159.192.1:2408"]}`), warpIn(6, "Germany", true, `{"name":"DE WARP","endpoints":["188.114.97.5:500"],"mtu":1300}`))
	if _, err := r.app.prov.refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	got := w.apply("sub1", nil)
	if len(got) != 6 {
		t.Fatalf("%d lines", len(got))
	}
	if !strings.Contains(lucxVpnConf(got[1]), "# Poland\n") || !strings.Contains(lucxVpnConf(got[4]), "# DE WARP\n") || !strings.Contains(lucxVpnConf(got[4]), "MTU = 1300\n") {
		t.Error("each inbound has its own name and settings")
	}
	// A switched-off inbound gives nothing.
	r.panel.setInbounds(warpIn(5, "Poland", false, `{}`), warpIn(6, "Germany", true, `{"endpoints":["188.114.97.5:500"]}`))
	_, _ = r.app.prov.refresh(t.Context())
	if got := w.apply("sub1", nil); len(got) != 3 || !strings.Contains(lucxVpnConf(got[1]), "# Germany\n") {
		t.Errorf("a disabled inbound still gives lines: %d", len(got))
	}
}

func TestAnInboundWithoutEndpointsUsesTheOnesThePluginKeeps(t *testing.T) {
	r, w := inboundRig(t, `{"endpoints":[]}`)
	_, _ = r.st.update(func(s *settings) error { s.Endpoints = []string{"162.159.195.2:1701"}; return nil })
	got := w.apply("sub1", nil)
	if len(got) != 3 || !strings.Contains(lucxVpnConf(got[1]), "Endpoint = 162.159.195.2:1701\n") {
		t.Errorf("lines = %d", len(got))
	}
}

func TestInboundModeNeedsADeviceLikeAnyOther(t *testing.T) {
	r, w := inboundRig(t, `{}`)
	_ = r.st.dropDevice("sub1")
	if len(w.apply("sub1", nil)) != 0 {
		t.Error("a subscription without its device yet gets nothing")
	}
	r.enable(t, func(s *settings) { s.Mode = "shared" })
	if len(w.apply("sub1", nil)) != 0 {
		t.Error("shared mode without a main account")
	}
	_ = r.st.setMain(sampleDevice(7))
	if len(w.apply("sub1", nil)) != 3 {
		t.Error("shared mode gives the main device")
	}
}

func TestDevicesAreMadeOnlyForSubscriptionsOnAnEnabledWarpInbound(t *testing.T) {
	r := newRig(t)
	r.panel.set(clientOn("alice", "sub1", true, 5), clientOn("bob", "sub2", true, 9), clientOn("carol", "sub3", false, 5))
	r.panel.setInbounds(warpIn(5, "Poland", true, `{}`))
	r.app.prov.tick(t.Context())
	if r.cf.registers != 1 || r.st.device("sub1") == nil {
		t.Fatalf("%d registrations; only alice should have a device", r.cf.registers)
	}
	// With the inbound off nothing is registered, whatever the plugin's own switch says.
	r2 := newRig(t)
	r2.enable(t, nil)
	r2.panel.set(clientOn("alice", "sub1", true, 5))
	r2.panel.setInbounds(warpIn(5, "Poland", false, `{}`))
	r2.app.prov.tick(t.Context())
	if r2.cf.registers != 0 {
		t.Error("a switched-off WARP inbound registers nothing")
	}
}

func TestWithoutWarpInboundsThePluginKeepsItsOwnSettings(t *testing.T) {
	r := newRig(t)
	r.enable(t, nil)
	r.panel.set(clientOn("alice", "sub1", true))
	r.panel.setInbounds() // the panel has inbounds, none of them WARP
	if _, err := r.app.prov.refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	_ = r.st.putDevice("sub1", sampleDevice(1))
	if got := r.app.lines.apply("sub1", nil); len(got) != 3 {
		t.Errorf("own settings: %d lines", len(got))
	}
	// An old permission set cannot read inbounds: the same.
	r.panel.inbounds = nil
	if _, err := r.app.prov.refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := r.app.lines.apply("sub1", nil); len(got) != 3 {
		t.Errorf("no inbound permission: %d lines", len(got))
	}
}

func TestAFailedInboundReadKeepsWhatWasKnown(t *testing.T) {
	r, w := inboundRig(t, `{}`)
	r.panel.inbounds = nil
	if _, err := r.app.prov.refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(w.apply("sub1", nil)) != 3 {
		t.Error("a moment of failure must not take WARP away")
	}
	var st stateView
	a := newAPIRigFrom(t, r)
	_, body := a.call(t, "GET", "/state", "", true)
	_ = json.Unmarshal([]byte(body), &st)
	if st.InboundsError == "" || st.Source != "inbounds" {
		t.Errorf("state = %q %q", st.Source, st.InboundsError)
	}
}

func TestTheStateListsTheInboundsAndThePreviewFollowsThem(t *testing.T) {
	r, _ := inboundRig(t, `{"endpoints":["162.159.192.1:2408"],"mtu":1333}`)
	a := newAPIRigFrom(t, r)
	_, body := a.call(t, "GET", "/state", "", true)
	var st stateView
	_ = json.Unmarshal([]byte(body), &st)
	if st.Source != "inbounds" || len(st.Inbounds) != 1 || st.Inbounds[0].Remark != "Poland WARP" || st.Inbounds[0].Subs != 1 || st.Inbounds[0].Endpoints[0] != "162.159.192.1:2408" {
		t.Errorf("state = %+v", st)
	}
	_, body = a.call(t, "GET", "/preview", "", true)
	if !strings.Contains(body, "MTU = 1333") || !strings.Contains(body, "Poland WARP") {
		t.Errorf("preview = %s", body)
	}
	_, body = a.call(t, "GET", "/subs", "", true)
	var subs struct {
		Source string   `json:"source"`
		Subs   []subRow `json:"subs"`
	}
	_ = json.Unmarshal([]byte(body), &subs)
	sel := map[string]bool{}
	for _, s := range subs.Subs {
		sel[s.ID] = s.Selected
	}
	if subs.Source != "inbounds" || !sel["sub1"] || sel["sub2"] {
		t.Errorf("subs = %+v", subs)
	}
}

func TestInboundSettingsLeaveOutOfRangeValuesToTheDefaults(t *testing.T) {
	s := defaultSettings()
	in := warpInbound{ID: 1, Remark: "x\ny", S: inboundWarp{MTU: 50, Endpoints: nil, Preset: "custom", AWG: awgSet{Jc: 99}}}
	got := s.withInbound(in)
	if got.MTU != 1280 || got.AWG.Jc != 4 || got.Endpoints[0] != defaultEndpoint {
		t.Errorf("settings = %+v", got)
	}
	if strings.ContainsAny(got.Name, "\r\n") {
		t.Errorf("name = %q", got.Name)
	}
}

func TestInboundSettingsAreReadAsAnObjectOrAsAString(t *testing.T) {
	obj := decodeInboundWarp(json.RawMessage(`{"endpoints":["a.b:1"],"mtu":1300}`))
	str := decodeInboundWarp(json.RawMessage(`"{\"endpoints\":[\"a.b:1\"],\"mtu\":1300}"`))
	if obj.MTU != 1300 || str.MTU != 1300 || obj.Endpoints[0] != "a.b:1" || str.Endpoints[0] != "a.b:1" {
		t.Errorf("object %+v, string %+v", obj, str)
	}
	if got := decodeInboundWarp(json.RawMessage(`nonsense`)); got.MTU != 0 || len(got.Endpoints) != 0 {
		t.Errorf("unreadable settings must read as empty: %+v", got)
	}
}
