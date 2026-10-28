// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"testing"
	"time"
)

func TestRosterKnowsWhoMayBeServed(t *testing.T) {
	now := time.Now().UnixMilli()
	rows := []clientRow{
		{Email: "on", SubID: "s1", Enable: true},
		{Email: "off", SubID: "s2", Enable: false},
		{Email: "expired", SubID: "s3", Enable: true, ExpiryTime: now - 1000},
		{Email: "later", SubID: "s4", Enable: true, ExpiryTime: now + 100000},
		{Email: "full", SubID: "s5", Enable: true, TotalGB: 100, Traffic: &struct {
			Up   int64 `json:"up"`
			Down int64 `json:"down"`
		}{60, 40}},
		{Email: "second", SubID: "s2", Enable: true}, // a subscription with one usable client is usable
		{Email: "nosub", SubID: "", Enable: true},
	}
	subs := buildRoster(rows, now)
	for id, want := range map[string]bool{"s1": true, "s2": true, "s3": false, "s4": true, "s5": false} {
		if subs[id] == nil || subs[id].Active != want {
			t.Errorf("%s active = %v, want %v", id, subs[id] != nil && subs[id].Active, want)
		}
	}
	if len(subs) != 5 || len(subs["s2"].Emails) != 2 {
		t.Errorf("subs = %v", subs)
	}
}

func TestEveryActiveSubscriptionGetsADeviceOfItsOwn(t *testing.T) {
	r := newRig(t)
	r.enable(t, nil)
	r.panel.set(client("a", "s1", true), client("b", "s2", true), client("c", "s3", false))
	r.app.prov.tick(t.Context())
	if r.st.device("s1") == nil || r.st.device("s2") == nil {
		t.Fatal("active subscriptions should have devices")
	}
	if r.st.device("s3") != nil {
		t.Error("a switched-off subscription must not get one")
	}
	if r.st.device("s1").PrivateKey == r.st.device("s2").PrivateKey {
		t.Error("devices are separate")
	}
	n := r.cf.registers
	r.app.prov.tick(t.Context())
	if r.cf.registers != n {
		t.Error("a subscription that has a device is not registered again")
	}
}

func TestNoDevicesAreMadeWhileWarpIsOffInSharedModeOrOutOfScope(t *testing.T) {
	r := newRig(t)
	r.panel.set(client("a", "s1", true))
	r.app.prov.tick(t.Context()) // disabled
	r.enable(t, func(s *settings) { s.Mode = "shared" })
	r.app.prov.tick(t.Context())
	r.enable(t, func(s *settings) { s.Mode, s.Scope, s.Selected = "perClient", "selected", []string{"other"} })
	r.app.prov.tick(t.Context())
	if r.cf.registers != 0 {
		t.Errorf("%d registrations", r.cf.registers)
	}
}

func TestAFailedRegistrationBacksOffAndDoesNotHammerCloudflare(t *testing.T) {
	r := newRig(t)
	r.enable(t, nil)
	r.cf.limited = true
	r.panel.set(client("a", "s1", true), client("b", "s2", true))
	now := time.Now()
	r.app.prov.now = func() time.Time { return now }
	r.app.prov.tick(t.Context())
	msg, until := r.app.prov.status()
	if msg == "" || !until.After(now.Add(5*time.Minute)) {
		t.Errorf("a rate limit means a long wait: %q until %v", msg, until)
	}
	r.cf.limited = false
	r.app.prov.tick(t.Context())
	if r.cf.registers != 0 {
		t.Error("registered during the wait")
	}
	now = now.Add(11 * time.Minute)
	r.app.prov.tick(t.Context())
	if r.cf.registers != 2 {
		t.Errorf("after the wait: %d registrations, want 2", r.cf.registers)
	}
	if msg, _ := r.app.prov.status(); msg != "" {
		t.Errorf("the error stays after success: %q", msg)
	}
}

func TestAtMostAFewDevicesAreMadePerPass(t *testing.T) {
	r := newRig(t)
	r.enable(t, nil)
	var rows []clientRow
	for i := 0; i < 25; i++ {
		rows = append(rows, client("u", string(rune('a'+i)), true))
	}
	r.panel.set(rows...)
	r.app.prov.tick(t.Context())
	if r.cf.registers != maxPerTick {
		t.Errorf("%d registrations in one pass, want %d", r.cf.registers, maxPerTick)
	}
}

func TestADeviceOfARemovedSubscriptionIsDeletedAfterAFewPasses(t *testing.T) {
	r := newRig(t)
	r.enable(t, nil)
	r.panel.set(client("a", "s1", true), client("b", "s2", true))
	r.app.prov.tick(t.Context())
	d2 := r.st.device("s2")
	r.panel.set(client("a", "s1", true)) // s2 is gone
	for i := 0; i < missesToDrop-1; i++ {
		r.app.prov.tick(t.Context())
		if r.st.device("s2") == nil {
			t.Fatalf("dropped after %d passes", i+1)
		}
	}
	r.app.prov.tick(t.Context())
	if r.st.device("s2") != nil || len(r.cf.deleted) != 1 || r.cf.deleted[0] != d2.ID {
		t.Errorf("device kept: %v, deleted at Cloudflare: %v", r.st.device("s2") != nil, r.cf.deleted)
	}
	if r.st.device("s1") == nil {
		t.Error("the other device must stay")
	}
}

func TestAnEmptyOrFailedAnswerFromThePanelDeletesNothing(t *testing.T) {
	r := newRig(t)
	r.enable(t, nil)
	r.panel.set(client("a", "s1", true))
	r.app.prov.tick(t.Context())
	r.panel.set() // no clients at all
	for i := 0; i < 5; i++ {
		r.app.prov.tick(t.Context())
	}
	r.panel.err = errors.New("panel down")
	for i := 0; i < 5; i++ {
		r.app.prov.tick(t.Context())
	}
	if r.st.device("s1") == nil || len(r.cf.deleted) != 0 {
		t.Error("devices were lost because the panel did not answer properly")
	}
	subs, loaded, _, errText := r.app.prov.roster.snapshot()
	if !loaded || errText == "" || len(subs) != 0 {
		t.Errorf("roster: loaded=%v err=%q subs=%d", loaded, errText, len(subs))
	}
}
