// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegisterMakesADeviceWithItsOwnKey(t *testing.T) {
	_, cf := newFakeCloudflare(t)
	a, err := cf.Register(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := cf.Register(context.Background(), "x")
	if a.ID == "" || a.Token == "" || a.V4 != "172.16.0.2" || !strings.HasPrefix(a.V6, "2606:") || a.PeerKey != testPeerKey || a.Host != "engage.cloudflareclient.com:2408" {
		t.Errorf("device = %+v", a)
	}
	if a.PrivateKey == b.PrivateKey || !validKey(a.PrivateKey) {
		t.Error("each device has a key of its own, 32 bytes")
	}
	raw, _ := base64.StdEncoding.DecodeString(a.PrivateKey)
	if raw[0]&7 != 0 || raw[31]&0x80 != 0 || raw[31]&0x40 == 0 {
		t.Error("the private key must be clamped as wg genkey does")
	}
}

func TestRegisterFallsBackToTheNextApiGeneration(t *testing.T) {
	f, cf := newFakeCloudflare(t)
	f.failNext = 1
	d, err := cf.Register(context.Background(), "x")
	if err != nil || d.ID == "" {
		t.Fatalf("the second profile should have answered: %v", err)
	}
}

func TestRegisterSaysWhenCloudflareLimitsIt(t *testing.T) {
	f, cf := newFakeCloudflare(t)
	f.limited = true
	if _, err := cf.Register(context.Background(), "x"); !errors.Is(err, errRateLimited) {
		t.Errorf("err = %v", err)
	}
}

func TestRegisterRefusesAnswersThatCannotBeUsed(t *testing.T) {
	for name, body := range map[string]string{
		"no token": `{"id":"a","config":{"peers":[{"public_key":"` + testPeerKey + `"}],"interface":{"addresses":{"v4":"172.16.0.2","v6":"2606:4700::2"}}}}`,
		"no peer":  `{"id":"a","token":"t","config":{"interface":{"addresses":{"v4":"172.16.0.2","v6":"2606:4700::2"}}}}`,
		"bad peer": `{"id":"a","token":"t","config":{"peers":[{"public_key":"abc"}],"interface":{"addresses":{"v4":"172.16.0.2","v6":"2606:4700::2"}}}}`,
		"bad v4":   `{"id":"a","token":"t","config":{"peers":[{"public_key":"` + testPeerKey + `"}],"interface":{"addresses":{"v4":"nope","v6":"2606:4700::2"}}}}`,
		"v6 as v4": `{"id":"a","token":"t","config":{"peers":[{"public_key":"` + testPeerKey + `"}],"interface":{"addresses":{"v4":"2606:4700::2","v6":"2606:4700::2"}}}}`,
		"not json": `<html>`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
		cf := &cfClient{http: srv.Client(), profiles: []cfProfile{{Base: srv.URL, UserAgent: "ua", DeviceType: "PC"}}, now: newFakeNow}
		if _, err := cf.Register(context.Background(), "x"); err == nil {
			t.Errorf("%s: accepted", name)
		}
		srv.Close()
	}
}

func TestAccountLicenseAndDelete(t *testing.T) {
	f, cf := newFakeCloudflare(t)
	d, _ := cf.Register(context.Background(), "x")
	info, err := cf.Account(context.Background(), d)
	if err != nil || info.Type != "free" || info.Plus {
		t.Fatalf("info = %+v %v", info, err)
	}
	if err := cf.SetLicense(context.Background(), d, "aaaaaaaa-bbbbbbbb-cccccccc"); err != nil {
		t.Fatal(err)
	}
	info, _ = cf.Account(context.Background(), d)
	if !info.Plus || info.Type != "unlimited" || info.Quota == 0 {
		t.Errorf("after the license: %+v", info)
	}
	if err := cf.Delete(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if len(f.deleted) != 1 || f.deleted[0] != d.ID {
		t.Errorf("deleted = %v", f.deleted)
	}
}

func TestACloudflareMessageIsShortAndReadable(t *testing.T) {
	if got := cfMessage([]byte(`{"errors":[{"message":"nope"}]}`)); got != "nope" {
		t.Errorf("got %q", got)
	}
	if got := cfMessage([]byte(strings.Repeat("x", 500))); len([]rune(got)) > 170 {
		t.Errorf("a long body must be cut: %d", len(got))
	}
}
