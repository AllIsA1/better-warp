// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakePanel answers the one panel call the plugin makes.
type fakePanel struct {
	mu       sync.Mutex
	rows     []clientRow
	err      error
	inbounds []map[string]any // the panel's inbound list; nil makes the call fail as an old permission set would
}

func (f *fakePanel) Do(_ context.Context, method, path string, _, out any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	if method == http.MethodGet && path == "/inbounds/list" {
		if f.inbounds == nil {
			return errors.New("panel answered HTTP 403 for GET /inbounds/list")
		}
		raw, _ := json.Marshal(f.inbounds)
		return json.Unmarshal(raw, out)
	}
	if method != http.MethodGet || path != "/clients/list" {
		return fmt.Errorf("unexpected panel call %s %s", method, path)
	}
	raw, _ := json.Marshal(f.rows)
	return json.Unmarshal(raw, out)
}

func (f *fakePanel) set(rows ...clientRow) {
	f.mu.Lock()
	f.rows = rows
	f.mu.Unlock()
}

func client(email, sub string, enable bool) clientRow {
	return clientRow{Email: email, SubID: sub, Enable: enable}
}

// fakeCloudflare plays the registration API. Every registration gets its own addresses.
type fakeCloudflare struct {
	mu        sync.Mutex
	srv       *httptest.Server
	registers int
	deleted   []string
	failNext  int // answer this many registrations with HTTP 500
	limited   bool
	licenses  map[string]string
}

func newFakeCloudflare(t *testing.T) (*fakeCloudflare, *cfClient) {
	t.Helper()
	f := &fakeCloudflare{licenses: map[string]string{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	cf := &cfClient{
		http:     f.srv.Client(),
		profiles: []cfProfile{{Base: f.srv.URL + "/v0a4005", ClientVersion: "a-6.30-3596", UserAgent: "ua", DeviceType: "PC"}, {Base: f.srv.URL + "/v0a737", UserAgent: "ua", DeviceType: "Android"}},
		now:      time.Now,
	}
	return f, cf
}

const testPeerKey = "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo="

func (f *fakeCloudflare) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reg"):
		if f.limited {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		if f.failNext > 0 && strings.Contains(r.URL.Path, "v0a4005") {
			f.failNext--
			http.Error(w, `{"errors":[{"message":"boom"}]}`, http.StatusInternalServerError)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if k, _ := body["key"].(string); !validKey(k) {
			http.Error(w, `{"errors":[{"message":"bad key"}]}`, http.StatusBadRequest)
			return
		}
		f.registers++
		n := f.registers
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": fmt.Sprintf("dev-%d-aaaaaaaa", n), "token": fmt.Sprintf("tok-%d-secret", n),
			"account": map[string]any{"license": fmt.Sprintf("lic-%d", n)},
			"config": map[string]any{
				"peers":     []any{map[string]any{"public_key": testPeerKey, "endpoint": map[string]any{"host": "engage.cloudflareclient.com:2408"}}},
				"interface": map[string]any{"addresses": map[string]any{"v4": fmt.Sprintf("172.16.0.%d", n+1), "v6": fmt.Sprintf("2606:4700:110:8a36:df92:102a:9602:%x", n)}},
			},
		})
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/account"):
		id := strings.Split(r.URL.Path, "/")[len(strings.Split(r.URL.Path, "/"))-2]
		typ := "free"
		if f.licenses[id] != "" {
			typ = "unlimited"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"account_type": typ, "warp_plus": typ != "free", "premium_data": 1 << 30})
	case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/account"):
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		id := strings.Split(r.URL.Path, "/")[len(strings.Split(r.URL.Path, "/"))-2]
		if body["license"] == "" || r.Header.Get("Authorization") == "" {
			http.Error(w, `{"errors":[{"message":"no"}]}`, http.StatusBadRequest)
			return
		}
		f.licenses[id] = body["license"]
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id})
	case r.Method == http.MethodDelete:
		parts := strings.Split(r.URL.Path, "/")
		f.deleted = append(f.deleted, parts[len(parts)-1])
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

func newTestStore(t *testing.T) *store {
	t.Helper()
	st, err := openStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func sampleDevice(n int) *device {
	return &device{ID: fmt.Sprintf("dev-%d-aaaaaaaa", n), Token: "tok-secret", PrivateKey: "cHJpdmF0ZXByaXZhdGVwcml2YXRlcHJpdmF0ZTEyMzQ=", PeerKey: testPeerKey,
		V4: fmt.Sprintf("172.16.0.%d", n+1), V6: "2606:4700:110:8a36:df92:102a:9602:fba2", Created: time.Now()}
}

// fakeEngine plays Better Cores' WARP engine.
type fakeEngine struct {
	mu      sync.Mutex
	info    engineInfo
	err     error
	started []fakeStart
	jobs    map[string]string // id -> job JSON
	stopped []string
}

type fakeStart struct {
	Kind string
	Req  json.RawMessage
}

func newFakeEngine() *fakeEngine {
	return &fakeEngine{info: engineInfo{Engine: 1, Host: "engage.cloudflareclient.com", Ports: []int{2408, 500}}, jobs: map[string]string{}}
}

func (f *fakeEngine) Info(context.Context) (engineInfo, error) { return f.info, f.err }

func (f *fakeEngine) Start(_ context.Context, kind string, req any) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	raw, _ := json.Marshal(req)
	f.started = append(f.started, fakeStart{kind, raw})
	id := fmt.Sprintf("job%d", len(f.started))
	f.jobs[id] = `{"id":"` + id + `","state":"running","stage":"probe","done":1,"total":4}`
	return id, nil
}

func (f *fakeEngine) finish(id, json string) {
	f.mu.Lock()
	f.jobs[id] = json
	f.mu.Unlock()
}

func (f *fakeEngine) Job(_ context.Context, id string) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if j, ok := f.jobs[id]; ok {
		return json.RawMessage(j), nil
	}
	return nil, fmt.Errorf("no such job")
}

func (f *fakeEngine) Stop(_ context.Context, id string) error {
	f.mu.Lock()
	f.stopped = append(f.stopped, id)
	f.mu.Unlock()
	return nil
}

type testRig struct {
	app   *app
	panel *fakePanel
	cf    *fakeCloudflare
	eng   *fakeEngine
	st    *store
}

func newRig(t *testing.T) *testRig {
	t.Helper()
	panel := &fakePanel{}
	fcf, cf := newFakeCloudflare(t)
	eng := newFakeEngine()
	st := newTestStore(t)
	a := newApp(panel, st, cf, eng)
	a.prov.spacing = time.Millisecond
	return &testRig{app: a, panel: panel, cf: fcf, eng: eng, st: st}
}

func (r *testRig) enable(t *testing.T, mut func(*settings)) {
	t.Helper()
	if _, err := r.st.update(func(s *settings) error {
		s.Enabled = true
		if mut != nil {
			mut(s)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func newFakeNow() time.Time { return time.Now() }

// warpIn is a row of the panel's inbound list: a WARP inbound with the given settings JSON.
func warpIn(id int, remark string, enable bool, settings string) map[string]any {
	// The panel's inbound list carries the settings as an object.
	return map[string]any{"id": id, "remark": remark, "enable": enable, "protocol": "warp", "settings": json.RawMessage(settings)}
}

func (f *fakePanel) setInbounds(list ...map[string]any) {
	f.mu.Lock()
	if list == nil {
		list = []map[string]any{}
	}
	f.inbounds = list
	f.mu.Unlock()
}

func clientOn(email, sub string, enable bool, inbounds ...int) clientRow {
	c := client(email, sub, enable)
	c.InboundIDs = inbounds
	return c
}
