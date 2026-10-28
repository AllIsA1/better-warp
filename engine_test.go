// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type gatewayCall struct{ Method, Path, Auth, Body string }

// fakeGateway plays the plugin system's gateway in front of Better Cores.
func fakeGateway(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) (coresEngine, *[]gatewayCall) {
	t.Helper()
	var calls []gatewayCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		calls = append(calls, gatewayCall{r.Method, r.URL.Path, r.Header.Get("Authorization"), string(raw)})
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	p := &Panel{base: srv.URL, token: "plugin-token", client: srv.Client()}
	return coresEngine{peer: p.Peer("better-cores")}, &calls
}

func reply(w http.ResponseWriter, status int, ok bool, msg string, obj any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": ok, "msg": msg, "obj": obj})
}

func TestPeerCallsGoToTheGatewayWithThePluginsToken(t *testing.T) {
	eng, calls := fakeGateway(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/peer/better-cores/warp/info":
			reply(w, 200, true, "", engineInfo{Engine: 1, Ports: []int{2408}})
		case r.URL.Path == "/peer/better-cores/warp/check":
			reply(w, 200, true, "", map[string]string{"id": "j1"})
		case r.URL.Path == "/peer/better-cores/warp/jobs/j1" && r.Method == "GET":
			reply(w, 200, true, "", map[string]any{"state": "running"})
		case r.URL.Path == "/peer/better-cores/warp/jobs/j1" && r.Method == "DELETE":
			reply(w, 200, true, "", nil)
		}
	})
	ctx := context.Background()
	info, err := eng.Info(ctx)
	if err != nil || info.Engine != 1 {
		t.Fatalf("info = %+v %v", info, err)
	}
	id, err := eng.Start(ctx, "check", map[string]string{"endpoint": "x"})
	if err != nil || id != "j1" {
		t.Fatalf("start = %q %v", id, err)
	}
	raw, err := eng.Job(ctx, "j1")
	if err != nil || !strings.Contains(string(raw), "running") {
		t.Fatalf("job = %s %v", raw, err)
	}
	if err := eng.Stop(ctx, "j1"); err != nil {
		t.Fatal(err)
	}
	for _, c := range *calls {
		if c.Auth != "Bearer plugin-token" {
			t.Errorf("%s %s without the token", c.Method, c.Path)
		}
	}
	if (*calls)[1].Method != "POST" || !strings.Contains((*calls)[1].Body, `"endpoint":"x"`) {
		t.Errorf("start call = %+v", (*calls)[1])
	}
}

func TestEngineProblemsAreExplained(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		msg    string
		want   string
	}{
		"too old":       {404, "plugin endpoint not found", "update it"},
		"not running":   {503, "the plugin is not running", "turn it on"},
		"no permission": {403, "this plugin does not hold the permission to call better-cores", "reinstall"},
		"older engine":  {200, "", "older WARP engine"},
	} {
		eng, _ := fakeGateway(t, func(w http.ResponseWriter, _ *http.Request) {
			if tc.status == 200 {
				reply(w, 200, true, "", engineInfo{Engine: 0})
				return
			}
			reply(w, tc.status, false, tc.msg, nil)
		})
		_, err := eng.Info(context.Background())
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want it to say %q", name, err, tc.want)
		}
	}
}

func TestWaitJobPollsUntilTheJobEnds(t *testing.T) {
	f := newFakeEngine()
	f.jobs["j"] = `{"state":"running"}`
	go func() {
		time.Sleep(30 * time.Millisecond)
		f.finish("j", `{"state":"done","result":{"hits":[]}}`)
	}()
	st, err := waitJob(context.Background(), f, "j", 5*time.Millisecond, time.Second)
	if err != nil || st.State != "done" {
		t.Errorf("st = %+v %v", st, err)
	}
}

func TestWaitJobStopsAJobThatTakesTooLong(t *testing.T) {
	f := newFakeEngine()
	f.jobs["j"] = `{"state":"running"}`
	_, err := waitJob(context.Background(), f, "j", 5*time.Millisecond, 30*time.Millisecond)
	if err == nil || len(f.stopped) != 1 {
		t.Errorf("err = %v, stopped = %v", err, f.stopped)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f.stopped = nil
	if _, err := waitJob(ctx, f, "j", 5*time.Millisecond, time.Second); !errors.Is(err, context.Canceled) || len(f.stopped) != 1 {
		t.Errorf("a cancelled wait stops the job: %v %v", err, f.stopped)
	}
}
