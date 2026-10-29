// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"embed"
	"encoding/json"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

// The page is part of the binary, so the plugin needs no file access beyond its own state directory.
//
//go:embed ui
var pages embed.FS

const maxBody = 1 << 20

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// admin lets a call through only when the panel says an administrator made it.
func admin(w http.ResponseWriter, r *http.Request) bool {
	if RequestUser(r) == "" {
		fail(w, http.StatusForbidden, errNoAdmin.Error())
		return false
	}
	return true
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(v); err != nil {
		fail(w, http.StatusBadRequest, "the request is not valid JSON of the expected shape")
		return false
	}
	return true
}

// newMux serves everything the plugin answers: the subscription seam, the page and the calls the page makes. The
// panel's page bridge only reaches /api/..., which is where the page's calls go.
func newMux(a *app) http.Handler {
	inner := http.NewServeMux()
	inner.HandleFunc("GET /state", a.handleState)
	inner.HandleFunc("POST /settings", a.handleSettings)
	inner.HandleFunc("GET /preview", a.handlePreview)
	inner.HandleFunc("GET /subs", a.handleSubs)
	inner.HandleFunc("POST /scope", a.handleScope)
	inner.HandleFunc("POST /account/register", a.handleRegister)
	inner.HandleFunc("POST /account/license", a.handleLicense)
	inner.HandleFunc("POST /account/refresh", a.handleRefresh)
	inner.HandleFunc("POST /account/delete", a.handleDeleteAccount)
	inner.HandleFunc("POST /devices/sync", a.handleSync)
	inner.HandleFunc("POST /devices/reset", a.handleResetDevices)
	inner.HandleFunc("POST /check", a.handleCheck)
	inner.HandleFunc("POST /scan", a.handleScan)
	inner.HandleFunc("POST /endpoints", a.handleEndpoints)
	inner.HandleFunc("POST /xray", a.handleXray)
	inner.HandleFunc("POST /xray/server/delete", a.handleXrayDeleteServer)
	inner.HandleFunc("POST /xray/restart", a.handleXrayRestart)
	inner.HandleFunc("GET /jobs/{id}", a.handleJob)
	inner.HandleFunc("DELETE /jobs/{id}", a.handleStopJob)

	mux := http.NewServeMux()
	mux.Handle("POST /_lucx/seam/{mode}", SeamHandler(func(call SeamCall) (SeamReply, error) {
		return subscriptionSeam(call, a.lines)
	}))
	mux.Handle("POST /_lucx/pipeline/xray.config", PipelineHandler(a.xrayPatch))
	mux.Handle("/api/", http.StripPrefix("/api", inner))
	mux.Handle("/ui/", pageHandler())
	return mux
}

type accountView struct {
	Registered bool       `json:"registered"`
	ID         string     `json:"id,omitempty"`
	Created    *time.Time `json:"created,omitempty"`
	Type       string     `json:"type,omitempty"`
	Plus       bool       `json:"plus"`
	Quota      int64      `json:"quota,omitempty"`
	License    bool       `json:"license"` // a WARP+ key was applied
	CheckedAt  *time.Time `json:"checkedAt,omitempty"`
	Error      string     `json:"error,omitempty"`
}

type devicesView struct {
	Have      int        `json:"have"`
	Wanted    int        `json:"wanted"` // active subscriptions in scope that have no device yet
	LastError string     `json:"lastError,omitempty"`
	RetryAt   *time.Time `json:"retryAt,omitempty"`
}

type rosterView struct {
	Loaded        bool       `json:"loaded"`
	Subscriptions int        `json:"subscriptions"`
	Active        int        `json:"active"`
	At            *time.Time `json:"at,omitempty"`
	Error         string     `json:"error,omitempty"`
}

// inboundView is a WARP inbound of the panel as the page lists it.
type inboundView struct {
	ID        int      `json:"id"`
	Remark    string   `json:"remark"`
	Enable    bool     `json:"enable"`
	Endpoints []string `json:"endpoints"`
	Subs      int      `json:"subs"` // subscriptions with a usable client on it
}

type stateView struct {
	Source   string        `json:"source"` // "inbounds": the panel's WARP inbounds decide; "own": this plugin's settings
	Inbounds []inboundView `json:"inbounds"`
	// InboundsError is why the panel's inbounds could not be read (the plugin then uses its own settings).
	InboundsError string            `json:"inboundsError,omitempty"`
	Settings      settings          `json:"settings"`
	Presets       map[string]awgSet `json:"presets"`
	Account       accountView       `json:"account"`
	Devices       devicesView       `json:"devices"`
	Roster        rosterView        `json:"roster"`
	Engine        struct {
		OK      bool   `json:"ok"`
		Error   string `json:"error,omitempty"`
		Version int    `json:"version,omitempty"`
		Ports   []int  `json:"ports,omitempty"`
	} `json:"engine"`
	LastScan *scanRecord `json:"lastScan,omitempty"`
	Scanning bool        `json:"scanning"`
	Xray     xrayView    `json:"xray"`
}

func tp(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func (a *app) state(ctx context.Context) stateView {
	var v stateView
	v.Settings, v.Presets = a.store.settings(), presets
	v.Source, v.Inbounds = a.inboundViews(v.Settings)
	a.prov.roster.mu.RLock()
	v.InboundsError = a.prov.roster.inboundsErr
	a.prov.roster.mu.RUnlock()
	if m := a.store.main(); m != nil {
		v.Account = accountView{Registered: true, ID: shortID(m.ID), Created: tp(m.Created), License: m.PlusKey != ""}
		if info, errText, at := a.accountInfo(); info != nil {
			v.Account.Type, v.Account.Plus, v.Account.Quota, v.Account.CheckedAt = info.Type, info.Plus, info.Quota, tp(at)
		} else if errText != "" {
			v.Account.Error = errText
		}
	}
	subs, loaded, at, rerr := a.prov.roster.snapshot()
	v.Roster = rosterView{Loaded: loaded, Subscriptions: len(subs), At: tp(at), Error: rerr}
	for _, s := range subs {
		if s.Active {
			v.Roster.Active++
		}
	}
	lastErr, retry := a.prov.status()
	v.Devices = devicesView{Have: len(a.store.deviceIDs()), LastError: lastErr}
	if retry.After(a.now()) {
		v.Devices.RetryAt = tp(retry)
	}
	if a.prov.meant(v.Settings) && v.Settings.Mode == "perClient" {
		v.Devices.Wanted = len(a.prov.wanted(v.Settings))
	}
	info, engErr := a.engineStatus(ctx)
	v.Engine.OK, v.Engine.Error, v.Engine.Version, v.Engine.Ports = engErr == "", engErr, info.Engine, info.Ports
	v.LastScan = a.store.lastScan()
	a.mu.Lock()
	v.Scanning = a.scanning
	a.mu.Unlock()
	v.Xray = a.xrayView(v.Settings)
	return v
}

// inboundViews says where the settings come from and lists the WARP inbounds.
func (a *app) inboundViews(s settings) (string, []inboundView) {
	ins := a.prov.roster.warpInbounds()
	out := []inboundView{}
	if len(ins) == 0 {
		return "own", out
	}
	subs, _, _, _ := a.prov.roster.snapshot()
	for _, in := range ins {
		v := inboundView{ID: in.ID, Remark: in.Remark, Enable: in.Enable, Endpoints: s.withInbound(in).Endpoints}
		for _, sub := range subs {
			for _, n := range a.prov.roster.nodesFor(sub.ID) {
				if n.ID == in.ID && sub.Active {
					v.Subs++
					break
				}
			}
		}
		out = append(out, v)
	}
	return "inbounds", out
}

// shortID shows enough of a device id to tell devices apart and not enough to use it.
func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8] + "…"
}

func (a *app) handleState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.state(r.Context()))
}

func (a *app) handleSettings(w http.ResponseWriter, r *http.Request) {
	if !admin(w, r) {
		return
	}
	var in settings
	if !decode(w, r, &in) {
		return
	}
	// The scope list and the Xray outbound (it has its own call) are not the form's to change here.
	cur, err := a.store.update(func(s *settings) error {
		in.Selected = s.Selected
		in.Xray = s.Xray
		*s = in
		return nil
	})
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	a.prov.kick()
	writeJSON(w, http.StatusOK, cur)
}

func (a *app) handleXray(w http.ResponseWriter, r *http.Request) {
	if !admin(w, r) {
		return
	}
	var in xraySet
	if !decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	out, err := a.setXray(ctx, in)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *app) handleXrayDeleteServer(w http.ResponseWriter, r *http.Request) {
	if !admin(w, r) {
		return
	}
	a.answer(w, a.deleteServerDevice(r.Context()))
}

func (a *app) handleXrayRestart(w http.ResponseWriter, r *http.Request) {
	if !admin(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	a.answer(w, a.restartXray(ctx))
}

// handlePreview shows what a subscriber would get under the current settings, with the key hidden.
func (a *app) handlePreview(w http.ResponseWriter, r *http.Request) {
	s := a.store.settings()
	d := device{PrivateKey: "(the subscriber's own key)", PeerKey: cloudflareKey, V4: "172.16.0.2", V6: "2606:4700:110:8000::2"}
	if m := a.store.main(); m != nil {
		d.PeerKey, d.V4, d.V6 = m.PeerKey, m.V4, m.V6
	}
	out := []map[string]string{}
	preview := func(s settings) {
		for i, ep := range s.Endpoints {
			n := node{Dev: d, Endpoint: ep, Name: s.nodeName(i)}
			out = append(out, map[string]string{"name": n.Name, "endpoint": ep, "conf": s.conf(n)})
		}
	}
	if ins := a.prov.roster.warpInbounds(); len(ins) > 0 {
		for _, in := range ins {
			if in.Enable {
				preview(s.withInbound(in))
			}
		}
	} else {
		preview(s)
	}
	writeJSON(w, http.StatusOK, out)
}

// cloudflareKey is Cloudflare's public WARP key, shown in the preview before an account exists.
const cloudflareKey = "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo="

type subRow struct {
	ID       string   `json:"subId"`
	Emails   []string `json:"emails"`
	Active   bool     `json:"active"`
	Device   bool     `json:"device"`
	Selected bool     `json:"selected"`
}

func (a *app) handleSubs(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("search")))
	s := a.store.settings()
	subs, loaded, _, rerr := a.prov.roster.snapshot()
	rows := []subRow{}
	for _, sub := range subs {
		if q != "" && !strings.Contains(strings.ToLower(sub.ID+" "+strings.Join(sub.Emails, " ")), q) {
			continue
		}
		rows = append(rows, subRow{ID: sub.ID, Emails: sub.Emails, Active: sub.Active, Device: a.store.device(sub.ID) != nil, Selected: a.prov.wants(s, sub.ID)})
		if len(rows) == 200 {
			break
		}
	}
	source, _ := a.inboundViews(s)
	writeJSON(w, http.StatusOK, map[string]any{"loaded": loaded, "error": rerr, "subs": rows, "scope": s.Scope, "source": source})
}

func (a *app) handleScope(w http.ResponseWriter, r *http.Request) {
	if !admin(w, r) {
		return
	}
	var in struct {
		Scope    string   `json:"scope"`
		Add      []string `json:"add"`
		Remove   []string `json:"remove"`
		Selected []string `json:"selected"`
	}
	if !decode(w, r, &in) {
		return
	}
	cur, err := a.store.update(func(s *settings) error {
		if in.Scope != "" {
			s.Scope = in.Scope
		}
		if in.Selected != nil {
			s.Selected = in.Selected
		}
		drop := map[string]bool{}
		for _, id := range in.Remove {
			drop[id] = true
		}
		keep := s.Selected[:0:0]
		for _, id := range s.Selected {
			if !drop[id] {
				keep = append(keep, id)
			}
		}
		s.Selected = append(keep, in.Add...)
		return nil
	})
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	a.prov.kick()
	writeJSON(w, http.StatusOK, map[string]any{"scope": cur.Scope, "selected": len(cur.Selected)})
}

func (a *app) answer(w http.ResponseWriter, err error) {
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *app) handleRegister(w http.ResponseWriter, r *http.Request) {
	if !admin(w, r) {
		return
	}
	a.answer(w, a.registerMain(r.Context()))
}

func (a *app) handleLicense(w http.ResponseWriter, r *http.Request) {
	if !admin(w, r) {
		return
	}
	var in struct {
		License string `json:"license"`
	}
	if !decode(w, r, &in) {
		return
	}
	a.answer(w, a.setLicense(r.Context(), in.License))
}

func (a *app) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if !admin(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	a.answer(w, a.refreshInfo(ctx))
}

func (a *app) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	if !admin(w, r) {
		return
	}
	a.answer(w, a.deleteMain(r.Context()))
}

func (a *app) handleSync(w http.ResponseWriter, r *http.Request) {
	if !admin(w, r) {
		return
	}
	a.prov.kick()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleResetDevices deletes the devices of the named subscriptions (all when none is named); in per-client mode the
// next pass gives them new ones.
func (a *app) handleResetDevices(w http.ResponseWriter, r *http.Request) {
	if !admin(w, r) {
		return
	}
	var in struct {
		SubIDs []string `json:"subIds"`
	}
	if r.ContentLength != 0 && !decode(w, r, &in) {
		return
	}
	ids := in.SubIDs
	if len(ids) == 0 {
		ids = a.store.deviceIDs()
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	removed := 0
	for _, id := range ids {
		d := a.store.device(id)
		if d == nil {
			continue
		}
		dctx, c := context.WithTimeout(ctx, 15*time.Second)
		_ = a.cf.Delete(dctx, *d) // best effort: the device may be gone at Cloudflare already
		c()
		if err := a.store.dropDevice(id); err != nil {
			fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		removed++
	}
	a.prov.kick()
	writeJSON(w, http.StatusOK, map[string]int{"removed": removed})
}

func (a *app) startOrFail(w http.ResponseWriter, id string, err error) {
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

func (a *app) handleCheck(w http.ResponseWriter, r *http.Request) {
	if !admin(w, r) {
		return
	}
	var in struct {
		Endpoint string `json:"endpoint"`
		Speed    bool   `json:"speed"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Endpoint != "" && !validHostPort(in.Endpoint) {
		fail(w, http.StatusBadRequest, "the endpoint must be host:port")
		return
	}
	id, err := a.startCheck(r.Context(), in.Endpoint, in.Speed)
	a.startOrFail(w, id, err)
}

func (a *app) handleScan(w http.ResponseWriter, r *http.Request) {
	if !admin(w, r) {
		return
	}
	var in scanOptions
	if r.ContentLength != 0 && !decode(w, r, &in) {
		return
	}
	id, err := a.startScan(r.Context(), in, false)
	a.startOrFail(w, id, err)
}

func (a *app) handleEndpoints(w http.ResponseWriter, r *http.Request) {
	if !admin(w, r) {
		return
	}
	var in struct {
		Endpoints []string `json:"endpoints"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.Endpoints) == 0 {
		fail(w, http.StatusBadRequest, "name at least one endpoint")
		return
	}
	cur, err := a.store.update(func(s *settings) error { s.Endpoints = in.Endpoints; return nil })
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"endpoints": cur.Endpoints})
}

func (a *app) handleJob(w http.ResponseWriter, r *http.Request) {
	raw, err := a.eng.Job(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
}

func (a *app) handleStopJob(w http.ResponseWriter, r *http.Request) {
	if !admin(w, r) {
		return
	}
	a.answer(w, a.eng.Stop(r.Context(), r.PathValue("id")))
}

// pageHandler answers GET /ui/... with the embedded files; a path ending in / serves its index.html.
func pageHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if strings.HasSuffix(name, "/") {
			name += "index.html"
		}
		clean := path.Clean(name)
		if !strings.HasPrefix(clean, "ui/") {
			http.NotFound(w, r)
			return
		}
		data, err := pages.ReadFile(clean)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		typ := mime.TypeByExtension(path.Ext(clean))
		if typ == "" {
			typ = "application/octet-stream"
		}
		w.Header().Set("Content-Type", typ)
		_, _ = w.Write(data)
	})
}
