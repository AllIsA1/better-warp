// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type apiRig struct {
	*testRig
	srv *httptest.Server
}

func newAPIRig(t *testing.T) *apiRig {
	t.Helper()
	r := newRig(t)
	srv := httptest.NewServer(newMux(r.app))
	t.Cleanup(srv.Close)
	return &apiRig{r, srv}
}

func (a *apiRig) call(t *testing.T, method, path, body string, admin bool) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, a.srv.URL+"/api"+path, strings.NewReader(body))
	if admin {
		req.Header.Set("X-LucX-User", "admin")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func TestOnlyTheAdministratorChangesAnything(t *testing.T) {
	a := newAPIRig(t)
	for _, c := range []struct{ method, path, body string }{
		{"POST", "/settings", `{"enabled":true}`},
		{"POST", "/scope", `{"scope":"selected"}`},
		{"POST", "/account/register", ``},
		{"POST", "/account/license", `{"license":"aaaaaaaa-bbbbbbbb-cccccccc"}`},
		{"POST", "/account/refresh", ``},
		{"POST", "/account/delete", ``},
		{"POST", "/devices/sync", ``},
		{"POST", "/devices/reset", ``},
		{"POST", "/check", `{}`},
		{"POST", "/scan", `{}`},
		{"POST", "/endpoints", `{"endpoints":["162.159.192.1:2408"]}`},
		{"DELETE", "/jobs/x", ``},
		{"POST", "/xray", `{"enabled":true}`},
		{"POST", "/xray/server/delete", ``},
		{"POST", "/xray/restart", ``},
	} {
		if code, _ := a.call(t, c.method, c.path, c.body, false); code != http.StatusForbidden {
			t.Errorf("%s %s without an administrator: %d, want 403", c.method, c.path, code)
		}
	}
	if a.cf.registers != 0 || a.st.main() != nil || a.st.settings().Enabled {
		t.Error("something changed")
	}
}

func TestTheStateShowsNoKeysAndNoTokens(t *testing.T) {
	a := newAPIRig(t)
	a.enable(t, nil)
	main := sampleDevice(1)
	_ = a.st.setMain(main)
	_ = a.st.putDevice("s1", sampleDevice(2))
	for _, path := range []string{"/state", "/subs", "/preview"} {
		_, body := a.call(t, "GET", path, "", true)
		for _, secret := range []string{main.PrivateKey, main.Token, "tok-secret", sampleDevice(2).PrivateKey} {
			if strings.Contains(body, secret) {
				t.Errorf("%s shows a secret: %s", path, secret)
			}
		}
	}
	_, body := a.call(t, "GET", "/state", "", true)
	var st stateView
	if err := json.Unmarshal([]byte(body), &st); err != nil {
		t.Fatal(err)
	}
	if !st.Account.Registered || st.Account.ID != "dev-1-aa…" || st.Devices.Have != 1 || !st.Engine.OK || st.Engine.Version != 1 || !st.Settings.Enabled {
		t.Errorf("state = %+v", st)
	}
}

func TestSettingsAreSavedValidatedAndKeepTheSelection(t *testing.T) {
	a := newAPIRig(t)
	_, _ = a.st.update(func(s *settings) error { s.Scope, s.Selected = "selected", []string{"s1", "s2"}; return nil })
	in := defaultSettings()
	in.Enabled, in.Name, in.Preset, in.Endpoints = true, "WARP", "light", []string{"188.114.97.5:500"}
	in.Scope = "selected"
	raw, _ := json.Marshal(in)
	if code, body := a.call(t, "POST", "/settings", string(raw), true); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	s := a.st.settings()
	if !s.Enabled || s.Name != "WARP" || s.AWG.Jc != 2 || s.Endpoints[0] != "188.114.97.5:500" {
		t.Errorf("settings = %+v", s)
	}
	if len(s.Selected) != 2 {
		t.Errorf("the form must not wipe the selection: %v", s.Selected)
	}
	in.MTU = 10
	raw, _ = json.Marshal(in)
	if code, body := a.call(t, "POST", "/settings", string(raw), true); code != 400 || !strings.Contains(body, "MTU") {
		t.Errorf("bad settings: %d %s", code, body)
	}
	if a.st.settings().MTU != 1280 {
		t.Error("refused settings were kept")
	}
}

func TestScopeCanBeSelectedOneByOne(t *testing.T) {
	a := newAPIRig(t)
	a.call(t, "POST", "/scope", `{"scope":"selected","add":["s1","s2"]}`, true)
	a.call(t, "POST", "/scope", `{"remove":["s1"],"add":["s3"]}`, true)
	s := a.st.settings()
	if s.Scope != "selected" || strings.Join(s.Selected, ",") != "s2,s3" {
		t.Errorf("scope = %s %v", s.Scope, s.Selected)
	}
}

func TestSubsListSearchesByEmailAndSaysWhoHasADevice(t *testing.T) {
	a := newAPIRig(t)
	a.panel.set(client("alice@example.com", "s1", true), client("bob@example.com", "s2", false))
	if _, err := a.app.prov.refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	_ = a.st.putDevice("s1", sampleDevice(1))
	_, body := a.call(t, "GET", "/subs?search=ALICE", "", true)
	var out struct {
		Loaded bool     `json:"loaded"`
		Subs   []subRow `json:"subs"`
	}
	_ = json.Unmarshal([]byte(body), &out)
	if !out.Loaded || len(out.Subs) != 1 || out.Subs[0].ID != "s1" || !out.Subs[0].Active || !out.Subs[0].Device {
		t.Errorf("subs = %+v", out)
	}
	_, body = a.call(t, "GET", "/subs", "", true)
	_ = json.Unmarshal([]byte(body), &out)
	if len(out.Subs) != 2 {
		t.Errorf("all = %d", len(out.Subs))
	}
}

func TestRegisteringTheAccountAndALicense(t *testing.T) {
	a := newAPIRig(t)
	if code, body := a.call(t, "POST", "/account/license", `{"license":"aaaaaaaa-bbbbbbbb-cccccccc"}`, true); code != 400 || !strings.Contains(body, "register") {
		t.Errorf("a license without an account: %d %s", code, body)
	}
	if code, body := a.call(t, "POST", "/account/register", ``, true); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	if a.st.main() == nil || a.cf.registers != 1 {
		t.Fatal("not registered")
	}
	if code, _ := a.call(t, "POST", "/account/register", ``, true); code != 400 {
		t.Error("registering twice")
	}
	if code, _ := a.call(t, "POST", "/account/license", `{"license":"not a key"}`, true); code != 400 {
		t.Error("a malformed key")
	}
	if code, body := a.call(t, "POST", "/account/license", `{"license":"aaaaaaaa-bbbbbbbb-cccccccc"}`, true); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	_, body := a.call(t, "GET", "/state", "", true)
	var st stateView
	_ = json.Unmarshal([]byte(body), &st)
	if !st.Account.Plus || !st.Account.License || st.Account.Type != "unlimited" {
		t.Errorf("account = %+v", st.Account)
	}
	if code, _ := a.call(t, "POST", "/account/delete", ``, true); code != 200 || a.st.main() != nil || len(a.cf.deleted) != 1 {
		t.Error("deleting the account")
	}
}

func TestResetDevicesDeletesThemAtCloudflare(t *testing.T) {
	a := newAPIRig(t)
	_ = a.st.putDevice("s1", sampleDevice(1))
	_ = a.st.putDevice("s2", sampleDevice(2))
	code, body := a.call(t, "POST", "/devices/reset", `{"subIds":["s1"]}`, true)
	if code != 200 || !strings.Contains(body, `"removed":1`) || a.st.device("s1") != nil || a.st.device("s2") == nil {
		t.Errorf("one: %d %s", code, body)
	}
	a.call(t, "POST", "/devices/reset", ``, true)
	if len(a.st.deviceIDs()) != 0 || len(a.cf.deleted) != 2 {
		t.Errorf("all: %v deleted %v", a.st.deviceIDs(), a.cf.deleted)
	}
}

func TestCheckAndScanNeedTheAccountAndGoToTheEngine(t *testing.T) {
	a := newAPIRig(t)
	if code, body := a.call(t, "POST", "/check", `{}`, true); code != 400 || !strings.Contains(body, "register") {
		t.Errorf("check without an account: %d %s", code, body)
	}
	_ = a.st.setMain(sampleDevice(1))
	if code, body := a.call(t, "POST", "/check", `{"endpoint":"162.159.193.5:500","speed":true}`, true); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	if code, _ := a.call(t, "POST", "/check", `{"endpoint":"no port"}`, true); code != 400 {
		t.Error("a bad endpoint")
	}
	if len(a.eng.started) != 1 || a.eng.started[0].Kind != "check" {
		t.Fatalf("started = %v", a.eng.started)
	}
	var req engCheck
	_ = json.Unmarshal(a.eng.started[0].Req, &req)
	if req.Endpoint != "162.159.193.5:500" || !req.Speed || req.MTU != 1280 || req.AWG.Jc != 4 || req.Account.PeerPublicKey != testPeerKey ||
		len(req.Account.Address) != 2 || req.Account.Address[0] != "172.16.0.2/32" {
		t.Errorf("the engine was asked: %+v", req)
	}
	// The job is read through to the page.
	a.eng.finish("job1", `{"id":"job1","state":"done","result":{"ok":true,"warp":"on"}}`)
	if code, body := a.call(t, "GET", "/jobs/job1", "", true); code != 200 || !strings.Contains(body, `"warp":"on"`) {
		t.Errorf("job: %d %s", code, body)
	}
	if code, _ := a.call(t, "GET", "/jobs/nope", "", true); code != http.StatusBadGateway {
		t.Errorf("an unknown job: %d", code)
	}
}

func TestScanKeepsItsResultAndTheOperatorUsesIt(t *testing.T) {
	a := newAPIRig(t)
	_ = a.st.setMain(sampleDevice(1))
	code, body := a.call(t, "POST", "/scan", `{"speed":true,"ipv6":true}`, true)
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	var started struct{ ID string }
	_ = json.Unmarshal([]byte(body), &started)
	if code, _ := a.call(t, "POST", "/scan", `{}`, true); code != 400 {
		t.Error("only one scan at a time")
	}
	a.eng.finish(started.ID, `{"id":"`+started.ID+`","state":"done","result":{"tried":300,"alive":40,"hits":[{"endpoint":"188.114.97.5:500","rttMs":12.5,"lossPct":0,"downMbps":88.1,"tested":true},{"endpoint":"162.159.192.7:2408","rttMs":20,"lossPct":20}]}}`)
	deadline := time.Now().Add(5 * time.Second)
	for a.st.lastScan() == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	last := a.st.lastScan()
	if last == nil || last.Error != "" || last.Auto || !strings.Contains(string(last.Result), "188.114.97.5:500") {
		t.Fatalf("last scan = %+v", last)
	}
	var req engScan
	_ = json.Unmarshal(a.eng.started[0].Req, &req)
	if !req.Speed || !req.IPv6 || req.Account.PrivateKey == "" {
		t.Errorf("scan request = %+v", req)
	}
	if code, _ := a.call(t, "POST", "/endpoints", `{"endpoints":["188.114.97.5:500","162.159.192.7:2408"]}`, true); code != 200 {
		t.Error("use endpoints")
	}
	if eps := a.st.settings().Endpoints; len(eps) != 2 || eps[0] != "188.114.97.5:500" {
		t.Errorf("endpoints = %v", eps)
	}
	if code, _ := a.call(t, "POST", "/endpoints", `{"endpoints":[]}`, true); code != 400 {
		t.Error("no endpoints")
	}
	if code, _ := a.call(t, "POST", "/endpoints", `{"endpoints":["a:1","a:2","a:3","a:4"]}`, true); code != 400 {
		t.Error("too many endpoints")
	}
}

func TestAScheduledScanAppliesTheBestEndpoints(t *testing.T) {
	a := newAPIRig(t)
	a.enable(t, func(s *settings) { s.AutoScan = autoScan{Enabled: true, Hours: 6, Count: 2} })
	_ = a.st.setMain(sampleDevice(1))
	id, err := a.app.startScan(t.Context(), scanOptions{}, true)
	if err != nil {
		t.Fatal(err)
	}
	a.eng.finish(id, `{"state":"done","result":{"hits":[{"endpoint":"188.114.97.5:500","rttMs":12,"lossPct":0},{"endpoint":"162.159.192.7:2408","rttMs":20,"lossPct":80},{"endpoint":"162.159.195.2:1701","rttMs":30,"lossPct":0}]}}`)
	deadline := time.Now().Add(5 * time.Second)
	for a.st.lastScan() == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	eps := a.st.settings().Endpoints
	if len(eps) != 2 || eps[0] != "188.114.97.5:500" || eps[1] != "162.159.195.2:1701" {
		t.Errorf("a lossy endpoint must be skipped and two kept: %v", eps)
	}
	if last := a.st.lastScan(); !last.Auto || len(last.Applied) != 2 {
		t.Errorf("last = %+v", last)
	}
}

func TestAScanThatFindsNothingChangesNothing(t *testing.T) {
	a := newAPIRig(t)
	_ = a.st.setMain(sampleDevice(1))
	before := a.st.settings().Endpoints[0]
	id, _ := a.app.startScan(t.Context(), scanOptions{}, true)
	a.eng.finish(id, `{"state":"done","result":{"tried":300,"alive":0,"hits":[]}}`)
	deadline := time.Now().Add(5 * time.Second)
	for a.st.lastScan() == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if a.st.settings().Endpoints[0] != before {
		t.Error("the endpoints changed without a result")
	}
}

func TestWithoutTheEngineTheStateSaysSoAndTheSubscriptionStillWorks(t *testing.T) {
	a := newAPIRig(t)
	a.eng.err = errNoEngine
	_ = a.st.setMain(sampleDevice(1))
	_, body := a.call(t, "GET", "/state", "", true)
	var st stateView
	_ = json.Unmarshal([]byte(body), &st)
	if st.Engine.OK || !strings.Contains(st.Engine.Error, "Better Cores") {
		t.Errorf("engine = %+v", st.Engine)
	}
	if code, body := a.call(t, "POST", "/check", `{}`, true); code != 400 || !strings.Contains(body, "Better Cores") {
		t.Errorf("check: %d %s", code, body)
	}
	if code, _ := a.call(t, "GET", "/preview", "", true); code != 200 {
		t.Error("the preview does not need the engine")
	}
}

func TestThePageIsServedAndNothingElseOfTheFilesystem(t *testing.T) {
	a := newAPIRig(t)
	for path, want := range map[string]int{"/ui/index.html": 200, "/ui/": 200, "/ui/app.js": 200, "/ui/style.css": 200, "/ui/../go.mod": 404, "/ui/nope": 404} {
		resp, err := http.Get(a.srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("%s: %d, want %d", path, resp.StatusCode, want)
		}
	}
}

func TestTheScheduleStartsAScanOnlyWhenOneIsDue(t *testing.T) {
	a := newAPIRig(t)
	a.app.scheduleOnce(t.Context()) // WARP off
	a.enable(t, func(s *settings) { s.AutoScan = autoScan{Enabled: true, Hours: 6, Count: 1, Speed: true} })
	a.app.scheduleOnce(t.Context()) // no account
	if len(a.eng.started) != 0 {
		t.Fatal("started without an account or while off")
	}
	_ = a.st.setMain(sampleDevice(1))
	now := time.Now()
	a.app.now = func() time.Time { return now }
	_ = a.st.setLastScan(scanRecord{At: now.Add(-2 * time.Hour)})
	a.app.scheduleOnce(t.Context()) // ran two hours ago, every six
	if len(a.eng.started) != 0 {
		t.Fatal("started too early")
	}
	_ = a.st.setLastScan(scanRecord{At: now.Add(-7 * time.Hour)})
	a.app.scheduleOnce(t.Context())
	if len(a.eng.started) != 1 || a.eng.started[0].Kind != "scan" {
		t.Fatalf("started = %v", a.eng.started)
	}
	var req engScan
	_ = json.Unmarshal(a.eng.started[0].Req, &req)
	if !req.Speed {
		t.Error("the schedule's download option")
	}
}

func TestAFailedScheduledScanIsNotRetriedEveryFiveMinutes(t *testing.T) {
	a := newAPIRig(t)
	a.enable(t, func(s *settings) { s.AutoScan = autoScan{Enabled: true, Hours: 6, Count: 1} })
	_ = a.st.setMain(sampleDevice(1))
	a.eng.err = errNoEngine
	a.app.scheduleOnce(t.Context())
	last := a.st.lastScan()
	if last == nil || last.Error == "" || !last.Auto {
		t.Fatalf("last = %+v", last)
	}
	a.eng.err = nil
	a.app.scheduleOnce(t.Context())
	if len(a.eng.started) != 0 {
		t.Error("it tried again at once")
	}
}

// newAPIRigFrom serves the API of an existing rig.
func newAPIRigFrom(t *testing.T, r *testRig) *apiRig {
	t.Helper()
	srv := httptest.NewServer(newMux(r.app))
	t.Cleanup(srv.Close)
	return &apiRig{r, srv}
}
