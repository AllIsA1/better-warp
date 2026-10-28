// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

// The WARP engine lives in Better Cores. This plugin asks it, through the plugin system's peer gateway, to check an
// account at an endpoint and to scan Cloudflare's endpoints. Without Better Cores (or with one too old to have the
// engine) the plugin still serves its subscription lines; what needs the engine says so.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	enginePlugin      = "better-cores"
	wantEngineAtLeast = 1
)

// engineInfo is what the engine says about itself.
type engineInfo struct {
	Engine int    `json:"engine"`
	Host   string `json:"host"`
	Ports  []int  `json:"ports"`
}

// engineAPI is Better Cores' WARP engine as this plugin uses it (tests put a fake in its place).
type engineAPI interface {
	Info(ctx context.Context) (engineInfo, error)
	Start(ctx context.Context, kind string, req any) (string, error)
	Job(ctx context.Context, id string) (json.RawMessage, error)
	Stop(ctx context.Context, id string) error
}

type coresEngine struct{ peer *Peer }

var errNoEngine = errors.New("Better Cores is not reachable: it must be installed, on, and version 0.2.0 or newer")

func engineErr(err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "not found"), strings.Contains(msg, "HTTP 404"):
		return errors.New("Better Cores has no WARP engine: update it to 0.2.0 or newer")
	case strings.Contains(msg, "not running"), strings.Contains(msg, "HTTP 503"):
		return errors.New("Better Cores is not running: turn it on on the Plugins page")
	case strings.Contains(msg, "does not hold the permission"), strings.Contains(msg, "HTTP 403"):
		return errors.New("this plugin has no permission to call Better Cores: reinstall Better WARP and confirm all its permissions")
	}
	return err
}

func (c coresEngine) Info(ctx context.Context) (engineInfo, error) {
	var info engineInfo
	if err := c.peer.Do(ctx, http.MethodGet, "/warp/info", nil, &info); err != nil {
		return engineInfo{}, engineErr(err)
	}
	if info.Engine < wantEngineAtLeast {
		return info, errors.New("Better Cores has an older WARP engine: update it")
	}
	return info, nil
}

func (c coresEngine) Start(ctx context.Context, kind string, req any) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	if err := c.peer.Do(ctx, http.MethodPost, "/warp/"+kind, req, &out); err != nil {
		return "", engineErr(err)
	}
	return out.ID, nil
}

func (c coresEngine) Job(ctx context.Context, id string) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := c.peer.Do(ctx, http.MethodGet, "/warp/jobs/"+id, nil, &raw); err != nil {
		return nil, engineErr(err)
	}
	return raw, nil
}

func (c coresEngine) Stop(ctx context.Context, id string) error {
	if err := c.peer.Do(ctx, http.MethodDelete, "/warp/jobs/"+id, nil, nil); err != nil {
		return engineErr(err)
	}
	return nil
}

type failingEngine struct{ err error }

func (f failingEngine) Info(context.Context) (engineInfo, error) { return engineInfo{}, f.err }
func (f failingEngine) Start(context.Context, string, any) (string, error) {
	return "", f.err
}
func (f failingEngine) Job(context.Context, string) (json.RawMessage, error) { return nil, f.err }
func (f failingEngine) Stop(context.Context, string) error                   { return f.err }

// engAccount, engCheck and engScan are the bodies the engine takes.
type engAccount struct {
	PrivateKey    string   `json:"privateKey"`
	Address       []string `json:"address"`
	PeerPublicKey string   `json:"peerPublicKey"`
}

func (s settings) engAccount(d device) engAccount {
	return engAccount{PrivateKey: d.PrivateKey, Address: []string{d.V4 + "/32", d.V6 + "/128"}, PeerPublicKey: d.PeerKey}
}

type engCheck struct {
	Account  engAccount `json:"account"`
	Endpoint string     `json:"endpoint"`
	MTU      int        `json:"mtu"`
	AWG      awgSet     `json:"awg"`
	Speed    bool       `json:"speed"`
}

type engScan struct {
	Account  engAccount `json:"account"`
	AWG      awgSet     `json:"awg"`
	MTU      int        `json:"mtu"`
	Ports    []int      `json:"ports,omitempty"`
	PerRange int        `json:"perRange,omitempty"`
	IPv6     bool       `json:"ipv6,omitempty"`
	Top      int        `json:"top,omitempty"`
	Speed    bool       `json:"speed,omitempty"`
	SpeedTop int        `json:"speedTop,omitempty"`
}

// scanOptions are what the page may choose for a scan; the rest is the settings'.
type scanOptions struct {
	Ports    []int `json:"ports"`
	IPv6     bool  `json:"ipv6"`
	Speed    bool  `json:"speed"`
	PerRange int   `json:"perRange"`
}

// jobState reads the fields of an engine job this plugin needs.
type jobState struct {
	State  string          `json:"state"`
	Error  string          `json:"error,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
}

type scanHit struct {
	Endpoint string  `json:"endpoint"`
	RttMs    float64 `json:"rttMs"`
	LossPct  int     `json:"lossPct"`
	DownMbps float64 `json:"downMbps"`
	Tested   bool    `json:"tested"`
}

type scanOutcome struct {
	Tried int       `json:"tried"`
	Alive int       `json:"alive"`
	Hits  []scanHit `json:"hits"`
}

// waitJob polls a job until it ends, the context ends, or the limit passes.
func waitJob(ctx context.Context, eng engineAPI, id string, every, limit time.Duration) (jobState, error) {
	deadline := time.Now().Add(limit)
	for {
		raw, err := eng.Job(ctx, id)
		if err != nil {
			return jobState{}, err
		}
		var st jobState
		if err := json.Unmarshal(raw, &st); err != nil {
			return jobState{}, fmt.Errorf("the engine's answer is not a job: %w", err)
		}
		if st.State != "running" {
			return st, nil
		}
		if time.Now().After(deadline) {
			_ = eng.Stop(context.WithoutCancel(ctx), id)
			return jobState{}, errors.New("the scan took too long")
		}
		select {
		case <-ctx.Done():
			_ = eng.Stop(context.WithoutCancel(ctx), id)
			return jobState{}, ctx.Err()
		case <-time.After(every):
		}
	}
}
