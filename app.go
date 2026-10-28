// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync"
	"time"
)

// app is the plugin: its state, the Cloudflare client, the panel, the engine, and the work that goes on in the
// background (reading the clients, giving them devices, scanning on a schedule).
type app struct {
	store *store
	cf    *cfClient
	prov  *provisioner
	eng   engineAPI
	lines *warpLines
	now   func() time.Time

	mu       sync.Mutex
	info     *accountInfo // what Cloudflare said of the main account, and when and how it failed
	infoAt   time.Time
	infoErr  string
	engInfo  engineInfo
	engErr   string
	engAt    time.Time
	scanning bool
}

func newApp(api panelAPI, st *store, cf *cfClient, eng engineAPI) *app {
	prov := newProvisioner(api, st, cf)
	return &app{store: st, cf: cf, prov: prov, eng: eng, lines: &warpLines{store: st, roster: prov.roster}, now: time.Now}
}

var (
	errNoMain  = errors.New("register the WARP account first (Account → Register)")
	errNoAdmin = errors.New("only the panel's administrator may change this")
)

// engineStatus is the engine's state, looked at at most every twenty seconds.
func (a *app) engineStatus(ctx context.Context) (engineInfo, string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.now().Sub(a.engAt) < 20*time.Second && !a.engAt.IsZero() {
		return a.engInfo, a.engErr
	}
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	info, err := a.eng.Info(cctx)
	a.engInfo, a.engAt, a.engErr = info, a.now(), ""
	if err != nil {
		a.engErr = err.Error()
	}
	return a.engInfo, a.engErr
}

// registerMain makes the main account: the device the checks and scans use, and (in shared mode) the one every
// subscription gets.
func (a *app) registerMain(ctx context.Context) error {
	if a.store.main() != nil {
		return errors.New("the account is registered already")
	}
	rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	d, err := a.cf.Register(rctx, "lucx-warp")
	if err != nil {
		return err
	}
	if err := a.store.setMain(&d); err != nil {
		_ = a.cf.Delete(rctx, d)
		return err
	}
	a.mu.Lock()
	a.info, a.infoAt, a.infoErr = nil, time.Time{}, ""
	a.mu.Unlock()
	return nil
}

// deleteMain takes the main account back at Cloudflare and forgets it.
func (a *app) deleteMain(ctx context.Context) error {
	d := a.store.main()
	if d == nil {
		return errNoMain
	}
	dctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := a.cf.Delete(dctx, *d); err != nil && !strings.Contains(err.Error(), "refused") {
		return err
	}
	a.mu.Lock()
	a.info, a.infoAt = nil, time.Time{}
	a.mu.Unlock()
	return a.store.setMain(nil)
}

// setLicense attaches a WARP+ license to the main account and reads the account again.
func (a *app) setLicense(ctx context.Context, license string) error {
	d := a.store.main()
	if d == nil {
		return errNoMain
	}
	license = strings.TrimSpace(license)
	if license != "" && !licensePattern.MatchString(license) {
		return errors.New("a WARP+ license key looks like xxxxxxxx-xxxxxxxx-xxxxxxxx")
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if license != "" {
		if err := a.cf.SetLicense(cctx, *d, license); err != nil {
			return err
		}
		next := *d
		next.PlusKey = license
		if err := a.store.setMain(&next); err != nil {
			return err
		}
	}
	return a.refreshInfo(cctx)
}

// refreshInfo asks Cloudflare about the main account.
func (a *app) refreshInfo(ctx context.Context) error {
	d := a.store.main()
	if d == nil {
		return errNoMain
	}
	info, err := a.cf.Account(ctx, *d)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.infoAt = a.now()
	if err != nil {
		a.infoErr = err.Error()
		return err
	}
	a.info, a.infoErr = &info, ""
	return nil
}

func (a *app) accountInfo() (*accountInfo, string, time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.info, a.infoErr, a.infoAt
}

// startCheck asks the engine to check the main account at an endpoint.
func (a *app) startCheck(ctx context.Context, endpoint string, speed bool) (string, error) {
	d := a.store.main()
	if d == nil {
		return "", errNoMain
	}
	s := a.store.settings()
	if endpoint == "" {
		endpoint = s.Endpoints[0]
	}
	return a.eng.Start(ctx, "check", engCheck{Account: s.engAccount(*d), Endpoint: endpoint, MTU: s.MTU, AWG: s.AWG, Speed: speed})
}

// startScan starts a scan and a goroutine that waits for it and keeps the result (and, for the schedule, applies it).
func (a *app) startScan(ctx context.Context, opt scanOptions, auto bool) (string, error) {
	d := a.store.main()
	if d == nil {
		return "", errNoMain
	}
	a.mu.Lock()
	if a.scanning {
		a.mu.Unlock()
		return "", errors.New("a scan is running already")
	}
	a.scanning = true
	a.mu.Unlock()
	s := a.store.settings()
	req := engScan{Account: s.engAccount(*d), AWG: s.AWG, MTU: s.MTU, Ports: opt.Ports, IPv6: opt.IPv6, Speed: opt.Speed, PerRange: opt.PerRange, Top: 10}
	id, err := a.eng.Start(ctx, "scan", req)
	if err != nil {
		a.mu.Lock()
		a.scanning = false
		a.mu.Unlock()
		return "", err
	}
	go a.finishScan(id, auto, s.AutoScan.Count)
	return id, nil
}

// finishScan waits for the scan and keeps what it found.
func (a *app) finishScan(id string, auto bool, keep int) {
	defer func() {
		a.mu.Lock()
		a.scanning = false
		a.mu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	st, err := waitJob(ctx, a.eng, id, 2*time.Second, 9*time.Minute)
	rec := scanRecord{At: a.now(), Auto: auto}
	switch {
	case err != nil:
		rec.Error = err.Error()
	case st.State != "done":
		rec.Error = st.Error
		if rec.Error == "" {
			rec.Error = "the scan " + st.State
		}
	default:
		rec.Result = st.Result
		var out scanOutcome
		if json.Unmarshal(st.Result, &out) == nil && auto {
			rec.Applied = a.applyBest(out, keep)
		}
	}
	if err := a.store.setLastScan(rec); err != nil {
		log.Printf("better-warp: keeping the scan: %v", err)
	}
}

// applyBest makes the quickest endpoints of a scan the subscription's endpoints.
func (a *app) applyBest(out scanOutcome, keep int) []string {
	var eps []string
	for _, h := range out.Hits {
		if h.LossPct >= 60 {
			continue
		}
		eps = append(eps, h.Endpoint)
		if len(eps) == keep {
			break
		}
	}
	if len(eps) == 0 {
		return nil
	}
	if _, err := a.store.update(func(s *settings) error { s.Endpoints = eps; return nil }); err != nil {
		log.Printf("better-warp: applying the scan: %v", err)
		return nil
	}
	return eps
}

// scheduleLoop looks every five minutes whether a scheduled scan is due.
func (a *app) scheduleLoop(ctx context.Context) {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		a.scheduleOnce(ctx)
	}
}

// scheduleOnce starts the scan when the settings ask for one and the last one is old enough.
func (a *app) scheduleOnce(ctx context.Context) {
	s := a.store.settings()
	if !s.Enabled || !s.AutoScan.Enabled || a.store.main() == nil {
		return
	}
	if last := a.store.lastScan(); last != nil && a.now().Sub(last.At) < time.Duration(s.AutoScan.Hours)*time.Hour {
		return
	}
	sctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err := a.startScan(sctx, scanOptions{Speed: s.AutoScan.Speed}, true); err != nil {
		log.Printf("better-warp: the scheduled scan: %v", err)
		// Do not try again every five minutes: count the failure as a scan.
		_ = a.store.setLastScan(scanRecord{At: a.now(), Auto: true, Error: err.Error()})
	}
}
