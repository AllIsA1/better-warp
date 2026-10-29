// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestXrayRulesAreChecked(t *testing.T) {
	good := xraySet{Enabled: true, Tag: "warp", Rules: []xrayRule{{
		Name: "AI", Enabled: true, Domains: []string{" geosite:openai ", "domain:example.com", "geosite:openai"},
		IPs: []string{"geoip:us", "1.1.1.0/24", "2606:4700::1111"}, Inbounds: []string{"inbound-443"},
	}}}
	if err := good.normalize(); err != nil {
		t.Fatalf("a good set was refused: %v", err)
	}
	if got := good.Rules[0].Domains; len(got) != 2 || got[0] != "geosite:openai" {
		t.Errorf("domains not cleaned: %v", got)
	}
	for name, mut := range map[string]func(*xraySet){
		"reserved tag":    func(x *xraySet) { x.Tag = "direct" },
		"tag with space":  func(x *xraySet) { x.Tag = "my warp" },
		"bad endpoint":    func(x *xraySet) { x.Endpoint = "nonsense" },
		"no matcher":      func(x *xraySet) { x.Rules = []xrayRule{{Name: "empty", Enabled: true}} },
		"bad ip":          func(x *xraySet) { x.Rules = []xrayRule{{Enabled: true, IPs: []string{"not-an-ip"}}} },
		"blank in domain": func(x *xraySet) { x.Rules = []xrayRule{{Enabled: true, Domains: []string{"a b"}}} },
		"bad inbound tag": func(x *xraySet) { x.Rules = []xrayRule{{Enabled: true, Inbounds: []string{"a/b"}}} },
		"too many rules": func(x *xraySet) {
			x.Rules = make([]xrayRule, maxXrayRules+1)
			for i := range x.Rules {
				x.Rules[i] = xrayRule{Enabled: true, Domains: []string{"domain:a.com"}}
			}
		},
	} {
		x := xraySet{Tag: "warp", Rules: []xrayRule{{Enabled: true, Domains: []string{"domain:a.com"}}}}
		mut(&x)
		if err := x.normalize(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	empty := xraySet{}
	if err := empty.normalize(); err != nil || empty.Tag != "warp" {
		t.Errorf("an empty set should become the default: %v %+v", err, empty)
	}
}

func TestReservedBytesComeFromTheClientID(t *testing.T) {
	if got := (device{ClientID: "AQID"}).reserved(); len(got) != 3 || got[0] != 1 || got[2] != 3 {
		t.Errorf("reserved = %v", got)
	}
	for _, id := range []string{"", "!!!", "AQ=="} {
		if got := (device{ClientID: id}).reserved(); got != nil {
			t.Errorf("%q gave %v", id, got)
		}
	}
}

func xrayRig(t *testing.T) *testRig {
	t.Helper()
	r := newRig(t)
	r.app.res.lookup = func(_ context.Context, host string) ([]string, error) {
		if host == "engage.cloudflareclient.com" {
			return []string{"2606:4700:d0::a29f:c001", "162.159.192.1"}, nil
		}
		return nil, errors.New("no such host")
	}
	return r
}

func enableXray(t *testing.T, r *testRig, rules ...xrayRule) {
	t.Helper()
	if _, err := r.app.setXray(context.Background(), xraySet{Enabled: true, Tag: "warp", Rules: rules}); err != nil {
		t.Fatal(err)
	}
}

func patchDoc(t *testing.T, r *testRig, doc string) []PatchOp {
	t.Helper()
	ops, err := r.app.xrayPatch(json.RawMessage(doc))
	if err != nil {
		t.Fatal(err)
	}
	return ops
}

const baseConfig = `{"api":{"tag":"api"},"outbounds":[{"tag":"direct","protocol":"freedom"}],
"routing":{"rules":[{"type":"field","inboundTag":["api"],"outboundTag":"api"},{"type":"field","outboundTag":"direct","network":"tcp,udp"}]}}`

func TestNothingChangesWhileTheOutboundIsOff(t *testing.T) {
	r := xrayRig(t)
	if ops := patchDoc(t, r, baseConfig); ops != nil {
		t.Errorf("off: %v", ops)
	}
	// On, but without a device (the state file was edited, or a registration was undone).
	if _, err := r.st.update(func(s *settings) error { s.Xray.Enabled = true; return nil }); err != nil {
		t.Fatal(err)
	}
	if ops := patchDoc(t, r, baseConfig); ops != nil {
		t.Errorf("no device: %v", ops)
	}
}

func TestTheOutboundAndTheRulesGoIntoTheConfig(t *testing.T) {
	r := xrayRig(t)
	enableXray(t, r,
		xrayRule{Name: "AI", Enabled: true, Domains: []string{"geosite:openai"}},
		xrayRule{Name: "off", Enabled: false, Domains: []string{"domain:off.com"}},
		xrayRule{Name: "bridge", Enabled: true, Inbounds: []string{"olcrtc-1"}, IPs: []string{"geoip:us"}},
	)
	if r.cf.registers != 1 || r.st.server() == nil || r.st.server().ClientID != "AQID" {
		t.Fatalf("the server device: registers=%d %+v", r.cf.registers, r.st.server())
	}
	ops := patchDoc(t, r, baseConfig)
	if len(ops) != 3 {
		t.Fatalf("ops = %+v", ops)
	}
	if ops[0].Op != "add" || ops[0].Path != "/outbounds/-" {
		t.Errorf("outbound op: %+v", ops[0])
	}
	ob := ops[0].Value.(map[string]any)
	set := ob["settings"].(map[string]any)
	peer := set["peers"].([]any)[0].(map[string]any)
	if ob["tag"] != "warp" || ob["protocol"] != "wireguard" || peer["endpoint"] != "162.159.192.1:2408" {
		t.Errorf("outbound: %+v", ob)
	}
	if res := set["reserved"].([]int); len(res) != 3 || res[1] != 2 {
		t.Errorf("reserved: %v", set["reserved"])
	}
	if set["secretKey"] != r.st.server().PrivateKey || set["mtu"] != 1280 {
		t.Errorf("settings: %+v", set)
	}
	// After the API rule (index 0), in the order given; the disabled rule is left out.
	if ops[1].Path != "/routing/rules/1" || ops[2].Path != "/routing/rules/2" {
		t.Errorf("rule positions: %s %s", ops[1].Path, ops[2].Path)
	}
	first := ops[1].Value.(map[string]any)
	if first["outboundTag"] != "warp" || first["ruleTag"] != "lucx-warp-1" || first["type"] != "field" {
		t.Errorf("first rule: %+v", first)
	}
	if rr := ops[2].Value.(map[string]any); rr["ruleTag"] != "lucx-warp-3" || len(rr["inboundTag"].([]string)) != 1 || len(rr["ip"].([]string)) != 1 {
		t.Errorf("last rule: %+v", rr)
	}
	if r.app.xrayView(r.st.settings()).Problem != "" || r.app.xrayView(r.st.settings()).BuiltAt == nil {
		t.Errorf("the page should say the config was built: %+v", r.app.xrayView(r.st.settings()))
	}
}

func TestADocumentThatHasTheChangeGetsNoOperations(t *testing.T) {
	r := xrayRig(t)
	enableXray(t, r, xrayRule{Enabled: true, Domains: []string{"geosite:openai"}})
	ops := patchDoc(t, r, baseConfig)
	ob, rule := ops[0].Value, ops[1].Value
	doc := map[string]any{
		"api": map[string]any{"tag": "api"}, "outbounds": []any{map[string]any{"tag": "direct"}, ob},
		"routing": map[string]any{"rules": []any{
			map[string]any{"type": "field", "inboundTag": []any{"api"}, "outboundTag": "api"}, rule,
			map[string]any{"type": "field", "outboundTag": "direct"},
		}},
	}
	raw, _ := json.Marshal(doc)
	if again := patchDoc(t, r, string(raw)); again != nil {
		t.Errorf("a second pass should change nothing: %+v", again)
	}
}

func TestChangedRulesReplaceTheOldOnes(t *testing.T) {
	r := xrayRig(t)
	enableXray(t, r, xrayRule{Enabled: true, Domains: []string{"geosite:netflix"}})
	// The document carries a rule from an earlier setting.
	old := `{"api":{"tag":"api"},"outbounds":[{"tag":"warp"}],"routing":{"rules":[
	 {"type":"field","inboundTag":["api"],"outboundTag":"api"},
	 {"type":"field","ruleTag":"lucx-warp-1","domain":["geosite:openai"],"outboundTag":"warp"},
	 {"type":"field","outboundTag":"direct"}]}}`
	ops := patchDoc(t, r, old)
	if len(ops) != 2 || ops[0].Op != "remove" || ops[0].Path != "/routing/rules/1" || ops[1].Op != "add" || ops[1].Path != "/routing/rules/1" {
		t.Errorf("ops = %+v", ops)
	}
	// Rules switched off entirely: only the removal is left.
	if _, err := r.app.setXray(context.Background(), xraySet{Enabled: true, Tag: "warp"}); err != nil {
		t.Fatal(err)
	}
	if ops := patchDoc(t, r, old); len(ops) != 1 || ops[0].Op != "remove" {
		t.Errorf("ops = %+v", ops)
	}
}

func TestMissingSectionsAreCreated(t *testing.T) {
	r := xrayRig(t)
	enableXray(t, r, xrayRule{Enabled: true, Domains: []string{"domain:a.com"}})
	for doc, paths := range map[string][]string{
		`{}`:                                {"/outbounds", "/routing"},
		`{"outbounds":null,"routing":null}`: {"/outbounds", "/routing"},
		`{"outbounds":[],"routing":{}}`:     {"/outbounds", "/routing/rules"},
		`{"outbounds":[],"routing":{"rules":[]}}`: {"/outbounds", "/routing/rules/0"},
	} {
		ops := patchDoc(t, r, doc)
		if len(ops) != len(paths) {
			t.Errorf("%s: ops = %+v", doc, ops)
			continue
		}
		for i, p := range paths {
			if ops[i].Path != p {
				t.Errorf("%s: op %d at %s, want %s", doc, i, ops[i].Path, p)
			}
		}
	}
}

func TestAnOutboundOfTheSameTagIsLeftAlone(t *testing.T) {
	r := xrayRig(t)
	enableXray(t, r, xrayRule{Enabled: true, Domains: []string{"domain:a.com"}})
	ops := patchDoc(t, r, `{"outbounds":[{"tag":"warp","protocol":"freedom"}],"routing":{"rules":[]}}`)
	if len(ops) != 1 || ops[0].Path != "/routing/rules/0" {
		t.Errorf("ops = %+v", ops)
	}
}

func TestWithoutAnAddressTheOutboundIsLeftOut(t *testing.T) {
	r := xrayRig(t)
	enableXray(t, r, xrayRule{Enabled: true, Domains: []string{"domain:a.com"}})
	r.app.res.lookup = func(context.Context, string) ([]string, error) { return nil, errors.New("timeout") }
	// A fresh resolver has nothing to fall back on.
	r.app.res.cache = map[string]resolved{}
	if ops := patchDoc(t, r, baseConfig); ops != nil {
		t.Errorf("an outbound Xray could not start with must not be written: %+v", ops)
	}
	if p := r.app.xrayView(r.st.settings()).Problem; !strings.Contains(p, "cannot look up") {
		t.Errorf("the page should say why: %q", p)
	}
}

func TestTheLastGoodAddressIsKept(t *testing.T) {
	r := xrayRig(t)
	now := time.Now()
	r.app.res.now = func() time.Time { return now }
	enableXray(t, r, xrayRule{Enabled: true, Domains: []string{"domain:a.com"}})
	patchDoc(t, r, baseConfig)
	now = now.Add(endpointTTL + time.Minute)
	r.app.res.lookup = func(context.Context, string) ([]string, error) { return nil, errors.New("down") }
	ops := patchDoc(t, r, baseConfig)
	if len(ops) == 0 {
		t.Fatal("the stale address should still be used")
	}
	peer := ops[0].Value.(map[string]any)["settings"].(map[string]any)["peers"].([]any)[0].(map[string]any)
	if peer["endpoint"] != "162.159.192.1:2408" {
		t.Errorf("endpoint %v", peer["endpoint"])
	}
}

func TestAnEndpointOfTheOperatorWins(t *testing.T) {
	r := xrayRig(t)
	if _, err := r.app.setXray(context.Background(), xraySet{Enabled: true, Endpoint: "188.114.97.1:500"}); err != nil {
		t.Fatal(err)
	}
	ops := patchDoc(t, r, `{}`)
	peer := ops[0].Value.([]any)[0].(map[string]any)["settings"].(map[string]any)["peers"].([]any)[0].(map[string]any)
	if peer["endpoint"] != "188.114.97.1:500" {
		t.Errorf("endpoint %v", peer["endpoint"])
	}
}

func TestIPv6OffLeavesItOutOfTheTunnel(t *testing.T) {
	r := xrayRig(t)
	if _, err := r.st.update(func(s *settings) error { s.IPv6 = false; return nil }); err != nil {
		t.Fatal(err)
	}
	enableXray(t, r)
	ob := patchDoc(t, r, `{}`)[0].Value.([]any)[0].(map[string]any)["settings"].(map[string]any)
	if a := ob["address"].([]string); len(a) != 1 || !strings.HasSuffix(a[0], "/32") {
		t.Errorf("address %v", a)
	}
	if allowed := ob["peers"].([]any)[0].(map[string]any)["allowedIPs"].([]string); len(allowed) != 1 {
		t.Errorf("allowedIPs %v", allowed)
	}
}

func TestTheServerHasADeviceOfItsOwn(t *testing.T) {
	r := xrayRig(t)
	if err := r.app.registerMain(context.Background()); err != nil {
		t.Fatal(err)
	}
	enableXray(t, r)
	if r.cf.registers != 2 || r.st.server().ID == r.st.main().ID || r.st.server().PrivateKey == r.st.main().PrivateKey {
		t.Errorf("main and server should be different devices: %d", r.cf.registers)
	}
	// Saving again does not register again.
	enableXray(t, r)
	if r.cf.registers != 2 {
		t.Errorf("registered again: %d", r.cf.registers)
	}
}

func TestAFailedRegistrationLeavesTheSwitchOff(t *testing.T) {
	r := xrayRig(t)
	r.cf.limited = true
	if _, err := r.app.setXray(context.Background(), xraySet{Enabled: true}); err == nil {
		t.Fatal("expected the rate limit")
	}
	if r.st.settings().Xray.Enabled || r.st.server() != nil {
		t.Error("nothing should have been saved")
	}
}

func TestDeletingTheServerDeviceSwitchesTheOutboundOff(t *testing.T) {
	r := xrayRig(t)
	enableXray(t, r, xrayRule{Enabled: true, Domains: []string{"domain:a.com"}})
	id := r.st.server().ID
	if err := r.app.deleteServerDevice(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.st.server() != nil || r.st.settings().Xray.Enabled {
		t.Error("device or switch left")
	}
	if len(r.cf.deleted) != 1 || r.cf.deleted[0] != id {
		t.Errorf("deleted at Cloudflare: %v", r.cf.deleted)
	}
	if len(r.st.settings().Xray.Rules) != 1 {
		t.Error("the rules should stay")
	}
	if ops := patchDoc(t, r, baseConfig); ops != nil {
		t.Errorf("ops = %+v", ops)
	}
}

func TestTheXrayCallsOverHTTP(t *testing.T) {
	a := newAPIRig(t)
	a.app.res.lookup = func(context.Context, string) ([]string, error) { return []string{"162.159.192.1"}, nil }
	if code, body := a.call(t, "POST", "/xray", `{"enabled":true,"tag":"direct"}`, true); code != http.StatusBadRequest {
		t.Errorf("a reserved tag: %d %s", code, body)
	}
	code, body := a.call(t, "POST", "/xray", `{"enabled":true,"tag":"warp","rules":[{"name":"AI","enabled":true,"domains":["geosite:openai"]}]}`, true)
	if code != http.StatusOK {
		t.Fatalf("%d %s", code, body)
	}
	_, state := a.call(t, "GET", "/state", "", false)
	var v stateView
	if err := json.Unmarshal([]byte(state), &v); err != nil {
		t.Fatal(err)
	}
	if !v.Xray.Enabled || !v.Xray.Registered || v.Xray.Endpoint != defaultEndpoint || len(v.Settings.Xray.Rules) != 1 {
		t.Errorf("state: %+v", v.Xray)
	}
	if strings.Contains(state, a.st.server().PrivateKey) || strings.Contains(state, a.st.server().Token) {
		t.Error("the state shows a secret of the server device")
	}
	// The connection form must not overwrite the outbound's settings.
	if code, body := a.call(t, "POST", "/settings", `{"mtu":1280}`, true); code != http.StatusOK {
		t.Fatalf("%d %s", code, body)
	}
	if !a.st.settings().Xray.Enabled || len(a.st.settings().Xray.Rules) != 1 {
		t.Error("saving the connection settings changed the Xray outbound")
	}
	if code, _ := a.call(t, "POST", "/xray/restart", "", true); code != http.StatusOK || a.panel.restarts != 1 {
		t.Errorf("restart: %d, restarts=%d", code, a.panel.restarts)
	}
	if code, _ := a.call(t, "POST", "/xray/server/delete", "", true); code != http.StatusOK || a.st.server() != nil {
		t.Errorf("delete: %d", code)
	}
}

func TestThePipelineIsServed(t *testing.T) {
	a := newAPIRig(t)
	a.app.res.lookup = func(context.Context, string) ([]string, error) { return []string{"162.159.192.1"}, nil }
	if _, err := a.app.setXray(context.Background(), xraySet{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(a.srv.URL+"/_lucx/pipeline/xray.config", "application/json", strings.NewReader(`{"outbounds":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Patch []PatchOp `json:"patch"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&out) != nil || len(out.Patch) != 1 {
		t.Errorf("status %d, patch %+v", resp.StatusCode, out.Patch)
	}
}
