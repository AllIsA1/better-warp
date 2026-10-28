// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// panelAPI is the panel's API as the plugin sees it (tests put a fake in its place).
type panelAPI interface {
	Do(ctx context.Context, method, path string, in, out any) error
}

// subInfo is one subscription: the clients that share its id and whether any of them may be served.
type subInfo struct {
	ID     string   `json:"subId"`
	Emails []string `json:"emails"`
	Active bool     `json:"active"`
}

// roster is what the plugin knows of the panel's subscriptions. It is read in the background; a subscription request
// only looks at memory.
type roster struct {
	mu     sync.RWMutex
	loaded bool
	at     time.Time
	err    string
	subs   map[string]*subInfo

	// The WARP inbounds of the panel (protocol "warp") and which subscriptions have a usable client attached to which.
	// While there is any, they decide who gets WARP and with what settings; the plugin's own settings serve only
	// the defaults (the endpoints a scan keeps) and the account.
	inbounds    []warpInbound
	inboundsErr string
	attached    map[string][]int // subscription id -> ids of the WARP inbounds a usable client of it is attached to
}

// warpInbound is a row of the panel's inbound list whose protocol is warp.
type warpInbound struct {
	ID     int
	Remark string
	Enable bool
	S      inboundWarp
}

// inboundWarp is the settings JSON of a WARP inbound (checked by the panel when it is saved).
type inboundWarp struct {
	Name      string   `json:"name"`
	Endpoints []string `json:"endpoints"`
	MTU       int      `json:"mtu"`
	DNS       string   `json:"dns"`
	IPv6      *bool    `json:"ipv6"`
	Keepalive *int     `json:"keepalive"`
	Preset    string   `json:"preset"`
	AWG       awgSet   `json:"awg"`
}

// hasInbounds says whether the panel has any WARP inbound: the plugin is then driven by them.
func (r *roster) hasInbounds() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.inbounds) > 0
}

// nodesFor are the enabled WARP inbounds a usable client of the subscription is attached to.
func (r *roster) nodesFor(subID string) []warpInbound {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []warpInbound
	for _, in := range r.inbounds {
		if !in.Enable {
			continue
		}
		for _, id := range r.attached[subID] {
			if id == in.ID {
				out = append(out, in)
				break
			}
		}
	}
	return out
}

func (r *roster) warpInbounds() []warpInbound {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]warpInbound(nil), r.inbounds...)
}

func (r *roster) active(subID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s := r.subs[subID]
	return s != nil && s.Active
}

func (r *roster) snapshot() (subs []subInfo, loaded bool, at time.Time, errText string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, s := range r.subs {
		subs = append(subs, *s)
	}
	sort.Slice(subs, func(i, j int) bool { return strings.ToLower(subs[i].Emails[0]) < strings.ToLower(subs[j].Emails[0]) })
	return subs, r.loaded, r.at, r.err
}

type clientRow struct {
	Email      string `json:"email"`
	SubID      string `json:"subId"`
	Enable     bool   `json:"enable"`
	TotalGB    int64  `json:"totalGB"`
	ExpiryTime int64  `json:"expiryTime"`
	InboundIDs []int  `json:"inboundIds"`
	Traffic    *struct {
		Up   int64 `json:"up"`
		Down int64 `json:"down"`
	} `json:"traffic"`
}

// usable: the panel gives a client that is switched off, expired or out of traffic no working link, so neither does this plugin.
func (c clientRow) usable(nowMs int64) bool {
	if !c.Enable {
		return false
	}
	if c.ExpiryTime > 0 && c.ExpiryTime <= nowMs {
		return false
	}
	if c.TotalGB > 0 && c.Traffic != nil && c.Traffic.Up+c.Traffic.Down >= c.TotalGB {
		return false
	}
	return true
}

func buildRoster(rows []clientRow, nowMs int64) map[string]*subInfo {
	out := map[string]*subInfo{}
	for _, c := range rows {
		if c.SubID == "" {
			continue
		}
		s := out[c.SubID]
		if s == nil {
			s = &subInfo{ID: c.SubID}
			out[c.SubID] = s
		}
		s.Emails = append(s.Emails, c.Email)
		s.Active = s.Active || c.usable(nowMs)
	}
	return out
}

const (
	syncEvery       = 30 * time.Second
	maxPerTick      = 10
	registerSpacing = 3 * time.Second
	missesToDrop    = 3
)

// provisioner keeps the roster fresh and, in per-client mode, gives every active subscription a device of its own.
type provisioner struct {
	api     panelAPI
	store   *store
	cf      *cfClient
	roster  *roster
	now     func() time.Time
	pause   func(ctx context.Context, d time.Duration)
	wake    chan struct{}
	spacing time.Duration

	mu           sync.Mutex
	backoffUntil time.Time
	lastErr      string
	missed       map[string]int
}

func newProvisioner(api panelAPI, st *store, cf *cfClient) *provisioner {
	return &provisioner{
		api: api, store: st, cf: cf, roster: &roster{subs: map[string]*subInfo{}},
		now: time.Now, wake: make(chan struct{}, 1), spacing: registerSpacing, missed: map[string]int{},
		pause: func(ctx context.Context, d time.Duration) {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-t.C:
			case <-ctx.Done():
			}
		},
	}
}

// kick asks for a pass now.
func (p *provisioner) kick() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

func (p *provisioner) run(ctx context.Context) {
	p.tick(ctx)
	t := time.NewTicker(syncEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-p.wake:
		}
		p.tick(ctx)
	}
}

func (p *provisioner) status() (lastErr string, backoff time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastErr, p.backoffUntil
}

func (p *provisioner) setErr(msg string) {
	p.mu.Lock()
	p.lastErr = msg
	p.mu.Unlock()
}

// refresh reads the clients and the WARP inbounds from the panel. The panel is asked before the roster is locked:
// a subscription request only ever looks at memory.
func (p *provisioner) refresh(ctx context.Context) ([]clientRow, error) {
	var rows []clientRow
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	err := p.api.Do(cctx, http.MethodGet, "/clients/list", nil, &rows)
	var ins []warpInbound
	var inErr error
	if err == nil {
		ins, inErr = p.loadInbounds(cctx)
	}
	now := p.now()
	p.roster.mu.Lock()
	defer p.roster.mu.Unlock()
	if err != nil {
		p.roster.err = err.Error()
		return nil, err
	}
	p.roster.subs, p.roster.loaded, p.roster.at, p.roster.err = buildRoster(rows, now.UnixMilli()), true, now, ""
	if inErr != nil {
		// A plugin installed before it had the permission reads none, and stays on its own settings; what was known stays.
		p.roster.inboundsErr = inErr.Error()
		return rows, nil
	}
	p.roster.inboundsErr = ""
	p.roster.inbounds, p.roster.attached = ins, attachedTo(ins, rows, now.UnixMilli())
	return rows, nil
}

// loadInbounds reads the WARP inbounds of the panel.
func (p *provisioner) loadInbounds(ctx context.Context) ([]warpInbound, error) {
	var list []struct {
		ID       int             `json:"id"`
		Remark   string          `json:"remark"`
		Enable   bool            `json:"enable"`
		Protocol string          `json:"protocol"`
		Settings json.RawMessage `json:"settings"` // an object in the list, a string in some other answers
	}
	if err := p.api.Do(ctx, http.MethodGet, "/inbounds/list", nil, &list); err != nil {
		return nil, err
	}
	var ins []warpInbound
	for _, in := range list {
		if in.Protocol != "warp" {
			continue
		}
		w := warpInbound{ID: in.ID, Remark: in.Remark, Enable: in.Enable}
		w.S = decodeInboundWarp(in.Settings) // settings the panel checked; a row it cannot read gets the defaults
		ins = append(ins, w)
	}
	return ins, nil
}

// decodeInboundWarp reads the settings of a WARP inbound, which the panel gives as an object or as a string holding one.
func decodeInboundWarp(raw json.RawMessage) inboundWarp {
	var s inboundWarp
	var text string
	if json.Unmarshal(raw, &text) == nil {
		raw = json.RawMessage(text)
	}
	_ = json.Unmarshal(raw, &s)
	return s
}

// attachedTo maps each subscription to the WARP inbounds a usable client of it is attached to.
func attachedTo(ins []warpInbound, rows []clientRow, nowMs int64) map[string][]int {
	attached := map[string][]int{}
	for _, c := range rows {
		if c.SubID == "" || !c.usable(nowMs) {
			continue
		}
		for _, id := range c.InboundIDs {
			for _, in := range ins {
				if in.ID == id && !slices.Contains(attached[c.SubID], id) {
					attached[c.SubID] = append(attached[c.SubID], id)
				}
			}
		}
	}
	return attached
}

func (p *provisioner) tick(ctx context.Context) {
	rows, err := p.refresh(ctx)
	if err != nil {
		log.Printf("better-warp: reading the clients: %v", err)
		return
	}
	s := p.store.settings()
	if p.meant(s) && s.Mode == "perClient" {
		p.provision(ctx, s)
	}
	p.cleanup(ctx, rows)
}

// meant says whether WARP is to be given out at all: while the panel has WARP inbounds, when one of them is on;
// otherwise when the plugin's own switch is on.
func (p *provisioner) meant(s settings) bool {
	if p.roster.hasInbounds() {
		for _, in := range p.roster.warpInbounds() {
			if in.Enable {
				return true
			}
		}
		return false
	}
	return s.Enabled
}

// wants says whether a subscription is to get WARP.
func (p *provisioner) wants(s settings, subID string) bool {
	if p.roster.hasInbounds() {
		return len(p.roster.nodesFor(subID)) > 0
	}
	return s.inScope(subID)
}

// wanted lists the subscriptions that should have a device and have none, in a stable order.
func (p *provisioner) wanted(s settings) []string {
	subs, _, _, _ := p.roster.snapshot()
	var out []string
	for _, sub := range subs {
		if sub.Active && p.wants(s, sub.ID) && p.store.device(sub.ID) == nil {
			out = append(out, sub.ID)
		}
	}
	sort.Strings(out)
	return out
}

func (p *provisioner) provision(ctx context.Context, s settings) {
	p.mu.Lock()
	wait := p.backoffUntil
	p.mu.Unlock()
	if p.now().Before(wait) {
		return
	}
	todo := p.wanted(s)
	if len(todo) > maxPerTick {
		todo = todo[:maxPerTick]
	}
	for i, id := range todo {
		if ctx.Err() != nil {
			return
		}
		if i > 0 {
			p.pause(ctx, p.spacing)
		}
		if err := p.registerFor(ctx, id); err != nil {
			wait := 2 * time.Minute
			if errors.Is(err, errRateLimited) {
				wait = 10 * time.Minute
			}
			p.mu.Lock()
			p.backoffUntil, p.lastErr = p.now().Add(wait), err.Error()
			p.mu.Unlock()
			log.Printf("better-warp: registering a device for %s: %v (next try in %s)", id, err, wait)
			return
		}
		p.setErr("")
	}
}

// registerFor makes and keeps a device for one subscription.
func (p *provisioner) registerFor(ctx context.Context, subID string) error {
	rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	d, err := p.cf.Register(rctx, "lucx-warp")
	if err != nil {
		return err
	}
	if err := p.store.putDevice(subID, &d); err != nil {
		// The device exists at Cloudflare but cannot be kept: take it back rather than leave it behind.
		_ = p.cf.Delete(rctx, d)
		return err
	}
	log.Printf("better-warp: a device was registered for subscription %s", subID)
	return nil
}

// cleanup removes the devices of subscriptions that have been gone from the panel for a few passes in a row.
func (p *provisioner) cleanup(ctx context.Context, rows []clientRow) {
	if len(rows) == 0 {
		return // an empty answer is more likely a failure than a panel without clients
	}
	present := map[string]bool{}
	for _, r := range rows {
		present[r.SubID] = true
	}
	for _, id := range p.store.deviceIDs() {
		if present[id] {
			delete(p.missed, id)
			continue
		}
		p.missed[id]++
		if p.missed[id] < missesToDrop {
			continue
		}
		if d := p.store.device(id); d != nil {
			dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			_ = p.cf.Delete(dctx, *d) // best effort: the device may be gone already
			cancel()
		}
		if err := p.store.dropDevice(id); err == nil {
			delete(p.missed, id)
			log.Printf("better-warp: the device of the removed subscription %s was deleted", id)
		}
	}
}
