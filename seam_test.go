// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// rig with one active subscription "sub1" that has a device, WARP on.
func seamRig(t *testing.T) (*testRig, *warpLines) {
	t.Helper()
	r := newRig(t)
	r.enable(t, nil)
	r.panel.set(client("alice", "sub1", true), client("bob", "sub2", false))
	if _, err := r.app.prov.refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := r.st.putDevice("sub1", sampleDevice(1)); err != nil {
		t.Fatal(err)
	}
	return r, r.app.lines
}

func rawCall(body string, wrap bool) SeamCall {
	var c SeamCall
	c.Seam.Engine, c.Seam.Mode, c.Seam.Route = "sub", "after", seamRoute
	c.Request.Params = map[string]string{"subid": "sub1"}
	if wrap {
		body = base64.StdEncoding.EncodeToString([]byte(body))
	}
	ct := "text/plain; charset=utf-8"
	c.Response = &SeamResponse{Status: 200, Headers: map[string][]string{"Content-Type": {ct}}, Body: []byte(body)}
	return c
}

func modified(t *testing.T, rep SeamReply) []byte {
	t.Helper()
	if rep.Action != "modify" || rep.Response == nil || rep.Response.Body == nil {
		t.Fatalf("reply = %+v, want a modification", rep)
	}
	return *rep.Response.Body
}

func TestRawSubscriptionGetsTheThreeLinesOfEveryEndpoint(t *testing.T) {
	r, w := seamRig(t)
	r.enable(t, func(s *settings) { s.Endpoints = []string{"162.159.192.1:2408", "188.114.97.5:500"} })
	panelLines := "vless://x@h:443?type=tcp#one\nvless://y@h:443?type=tcp#two"
	rep, err := subscriptionSeam(rawCall(panelLines, true), w)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := base64.StdEncoding.DecodeString(string(modified(t, rep)))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(string(dec), "\n")
	if len(got) != 2+6 {
		t.Fatalf("%d lines: %q", len(got), got)
	}
	if got[0] != "vless://x@h:443?type=tcp#one" || got[1] != "vless://y@h:443?type=tcp#two" {
		t.Error("the panel's own lines must stay first and unchanged")
	}
	for i, prefix := range []string{"amneziawg://", "vpn://", "amneziawg://", "amneziawg://", "vpn://", "amneziawg://"} {
		if !strings.HasPrefix(got[2+i], prefix) {
			t.Errorf("line %d = %.30s, want %s…", 2+i, got[2+i], prefix)
		}
	}
	if !strings.Contains(got[2], "162.159.192.1:2408") || !strings.Contains(got[5], "188.114.97.5:500") {
		t.Error("one node per endpoint, in order")
	}
	if _, name, _ := strings.Cut(got[7], "#"); name != "Cloudflare%20WARP%202" {
		t.Errorf("the second node's name = %q", name)
	}
}

func TestPlainSubscriptionIsAnsweredPlain(t *testing.T) {
	_, w := seamRig(t)
	rep, _ := subscriptionSeam(rawCall("vless://x@h:443#one", false), w)
	body := string(modified(t, rep))
	if strings.HasPrefix(body, "dmxlc3M") || !strings.HasPrefix(body, "vless://x@h:443#one\namneziawg://") {
		t.Errorf("a plain subscription must stay plain: %.60s", body)
	}
}

func TestSecondPassAddsNothing(t *testing.T) {
	_, w := seamRig(t)
	lines := w.apply("sub1", []string{"vless://x@h:443#one"})
	again := w.apply("sub1", append([]string(nil), lines...))
	if strings.Join(lines, "\n") != strings.Join(again, "\n") {
		t.Error("a line that is there is not added again")
	}
	rep, _ := subscriptionSeam(rawCall(strings.Join(lines, "\n"), true), w)
	if rep.Action != "keep" {
		t.Errorf("a subscription that has the lines is kept as it is, got %s", rep.Action)
	}
}

func TestNothingIsAddedUnlessWarpIsMeantForThem(t *testing.T) {
	r, w := seamRig(t)
	base := []string{"vless://x@h:443#one"}
	same := func(why, sub string) {
		t.Helper()
		if got := w.apply(sub, append([]string(nil), base...)); len(got) != 1 {
			t.Errorf("%s: %d lines", why, len(got))
		}
	}
	same("a switched-off client", "sub2")
	same("an unknown subscription", "nobody")

	r.enable(t, func(s *settings) { s.Scope, s.Selected = "selected", []string{"other"} })
	same("a subscription out of scope", "sub1")
	r.enable(t, func(s *settings) { s.Scope = "all" })
	if got := w.apply("sub1", append([]string(nil), base...)); len(got) != 4 {
		t.Errorf("back in scope: %d lines", len(got))
	}

	r.enable(t, func(s *settings) { s.Enabled = false })
	if _, err := r.st.update(func(s *settings) error { s.Enabled = false; return nil }); err != nil {
		t.Fatal(err)
	}
	same("WARP off", "sub1")

	r.enable(t, nil)
	if err := r.st.dropDevice("sub1"); err != nil {
		t.Fatal(err)
	}
	same("a subscription without a device yet", "sub1")
}

func TestSharedModeGivesEveryoneTheMainDevice(t *testing.T) {
	r, w := seamRig(t)
	r.panel.set(client("alice", "sub1", true), client("bob", "sub2", true))
	if _, err := r.app.prov.refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	_ = r.st.dropDevice("sub1")
	r.enable(t, func(s *settings) { s.Mode = "shared" })
	if got := w.apply("sub2", nil); len(got) != 0 {
		t.Errorf("shared mode without a main account: %d lines", len(got))
	}
	main := sampleDevice(9)
	main.PrivateKey = "bWFpbm1haW5tYWluam1haW5tYWlubWFpbm1haW40NTY="
	_ = r.st.setMain(main)
	a, b := w.apply("sub1", nil), w.apply("sub2", nil)
	if len(a) != 3 || len(b) != 3 || a[0] != b[0] {
		t.Errorf("both subscriptions get the same device: %d %d", len(a), len(b))
	}
	if !strings.Contains(a[0], "172.16.0.10") {
		t.Error("the main device's address")
	}
}

func TestOnlyASuccessfulPanelAnswerIsTouched(t *testing.T) {
	_, w := seamRig(t)
	c := rawCall("x", true)
	c.Response.Status = 404
	if rep, _ := subscriptionSeam(c, w); rep.Action != "keep" {
		t.Errorf("404: %s", rep.Action)
	}
	c = rawCall("{\"json\":true}", false)
	c.Response.Headers["Content-Type"] = []string{"application/json"}
	if rep, _ := subscriptionSeam(c, w); rep.Action != "keep" {
		t.Errorf("json: %s", rep.Action)
	}
	c = rawCall("x", true)
	c.Request.Params = nil
	if rep, _ := subscriptionSeam(c, w); rep.Action != "keep" {
		t.Errorf("no subid: %s", rep.Action)
	}
	c = rawCall("x", true)
	c.Seam.Mode = "before"
	if _, err := subscriptionSeam(c, w); err == nil {
		t.Error("a seam that was not declared must be an error")
	}
}

const samplePage = `<!doctype html><html><body><script>window.__SUB_PAGE_DATA__={"subId":"sub1","links":["vless://a#1"],"linkNames":["one"]};</script></body></html>`

func TestThePageListGetsTheLinesToo(t *testing.T) {
	_, w := seamRig(t)
	c := rawCall(samplePage, false)
	c.Response.Headers["Content-Type"] = []string{"text/html; charset=utf-8"}
	rep, _ := subscriptionSeam(c, w)
	body := modified(t, rep)
	at := bytes.Index(body, []byte(pageDataMarker))
	var data struct {
		Links     []string `json:"links"`
		LinkNames []string `json:"linkNames"`
		SubID     string   `json:"subId"`
	}
	if err := json.NewDecoder(bytes.NewReader(body[at+len(pageDataMarker):])).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if len(data.Links) != 4 || data.Links[0] != "vless://a#1" || !strings.HasPrefix(data.Links[2], "vpn://") {
		t.Errorf("links = %v", data.Links)
	}
	if data.SubID != "sub1" || len(data.LinkNames) != 1 {
		t.Errorf("the rest of the page data must stay: %+v", data)
	}
	if !strings.HasSuffix(string(body), "</script></body></html>") {
		t.Error("the page around the data must stay")
	}
	// A page without the data is left alone.
	c = rawCall("<html>custom theme</html>", false)
	c.Response.Headers["Content-Type"] = []string{"text/html"}
	if rep, _ := subscriptionSeam(c, w); rep.Action != "keep" {
		t.Errorf("custom page: %s", rep.Action)
	}
}

func TestAnEmptyAnswerIsAWarpOnlySubscriberAndGetsTheLines(t *testing.T) {
	_, w := seamRig(t)
	for _, enc := range []string{"base64", "plain"} {
		c := rawCall("", false)
		c.Response.Body = nil
		c.Response.Headers["X-Lucx-Sub-Encoding"] = []string{enc}
		rep, err := subscriptionSeam(c, w)
		if err != nil {
			t.Fatal(err)
		}
		body := string(modified(t, rep))
		if enc == "base64" {
			dec, derr := base64.StdEncoding.DecodeString(body)
			if derr != nil {
				t.Fatalf("an encrypted subscription must stay base64: %v", derr)
			}
			body = string(dec)
		}
		if lines := strings.Split(body, "\n"); len(lines) != 3 || !strings.HasPrefix(lines[0], "amneziawg://") || !strings.HasPrefix(lines[1], "vpn://") {
			t.Errorf("%s: %q", enc, lines)
		}
	}
	// Without the header the plain form is used; a subscriber that gets nothing is left as it is.
	c := rawCall("", false)
	c.Response.Body = nil
	if rep, _ := subscriptionSeam(c, w); strings.HasPrefix(string(modified(t, rep)), "dmxlc3M") {
		t.Error("no header: plain")
	}
	c = rawCall("", false)
	c.Response.Body = nil
	c.Request.Params["subid"] = "sub2"
	if rep, _ := subscriptionSeam(c, w); rep.Action != "keep" {
		t.Errorf("nothing to add: %s", rep.Action)
	}
}
