// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The settings of the WARP protocol as the operator sees them. The defaults are what is known to work with
// Cloudflare's endpoints: WARP speaks plain WireGuard, so the only obfuscation it can carry is junk packets before the
// handshake (Jc, Jmin, Jmax); S1/S2 and H1-H4 stay at their defaults or the endpoint drops the handshake.
type settings struct {
	Enabled   bool     `json:"enabled"`   // put WARP into the subscriptions
	Mode      string   `json:"mode"`      // perClient: a device of its own for each subscription; shared: one device for all
	Scope     string   `json:"scope"`     // all subscriptions, or only the selected ones
	Selected  []string `json:"selected"`  // subscription ids, when Scope is "selected"
	Name      string   `json:"name"`      // what the node is called in the subscription
	Endpoints []string `json:"endpoints"` // host:port, one node each (at most 3)
	MTU       int      `json:"mtu"`
	DNS       string   `json:"dns"`
	IPv6      bool     `json:"ipv6"`
	Keepalive int      `json:"keepalive"`
	Preset    string   `json:"preset"` // standard, light, strong or custom
	AWG       awgSet   `json:"awg"`
	AutoScan  autoScan `json:"autoScan"`
}

// awgSet is the AmneziaWG part of a config.
type awgSet struct {
	Jc   int    `json:"jc"`
	Jmin int    `json:"jmin"`
	Jmax int    `json:"jmax"`
	S1   int    `json:"s1"`
	S2   int    `json:"s2"`
	H1   string `json:"h1,omitempty"`
	H2   string `json:"h2,omitempty"`
	H3   string `json:"h3,omitempty"`
	H4   string `json:"h4,omitempty"`
	I1   string `json:"i1,omitempty"`
	I2   string `json:"i2,omitempty"`
	I3   string `json:"i3,omitempty"`
	I4   string `json:"i4,omitempty"`
	I5   string `json:"i5,omitempty"`
}

// autoScan keeps the endpoints of the subscription at the quickest ones a scan finds, on a schedule.
type autoScan struct {
	Enabled bool `json:"enabled"`
	Hours   int  `json:"hours"` // how often
	Count   int  `json:"count"` // how many endpoints to keep (1-3)
	Speed   bool `json:"speed"` // rank the best by a download
}

const (
	defaultEndpoint = "engage.cloudflareclient.com:2408"
	defaultDNS      = "1.1.1.1, 1.0.0.1, 2606:4700:4700::1111, 2606:4700:4700::1001"
	defaultName     = "Cloudflare WARP"
	maxEndpoints    = 3
)

var presets = map[string]awgSet{
	"standard": {Jc: 4, Jmin: 40, Jmax: 70},
	"light":    {Jc: 2, Jmin: 20, Jmax: 40},
	"strong":   {Jc: 8, Jmin: 50, Jmax: 150},
}

func defaultSettings() settings {
	return settings{
		Mode: "perClient", Scope: "all", Name: defaultName, Endpoints: []string{defaultEndpoint},
		MTU: 1280, DNS: defaultDNS, IPv6: true, Keepalive: 25, Preset: "standard", AWG: presets["standard"],
		AutoScan: autoScan{Hours: 24, Count: 1},
	}
}

var hostPattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)

// validHostPort accepts host:port, ip:port and [v6]:port.
func validHostPort(s string) bool {
	host, port, err := net.SplitHostPort(strings.TrimSpace(s))
	if err != nil {
		return false
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return false
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip.IsValid()
	}
	return hostPattern.MatchString(host)
}

var (
	iPattern   = regexp.MustCompile(`^[ -~]{0,1000}$`) // printable ASCII: the tags <b 0x..> <r 16> <t> and the like
	dnsPattern = regexp.MustCompile(`^[0-9A-Fa-f:.]+$`)
)

// normalize fixes what can be fixed (trimming, defaults, the preset's values) and rejects the rest.
func (s *settings) normalize() error {
	d := defaultSettings()
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" {
		s.Name = d.Name
	}
	if utf8.RuneCountInString(s.Name) > 60 || strings.ContainsAny(s.Name, "\r\n\x00") {
		return errors.New("the name is at most 60 characters on one line")
	}
	switch s.Mode {
	case "perClient", "shared":
	case "":
		s.Mode = d.Mode
	default:
		return errors.New("mode must be perClient or shared")
	}
	switch s.Scope {
	case "all", "selected":
	case "":
		s.Scope = d.Scope
	default:
		return errors.New("scope must be all or selected")
	}
	if len(s.Selected) > 5000 {
		return errors.New("too many selected subscriptions")
	}
	seen := map[string]bool{}
	sel := s.Selected[:0:0]
	for _, id := range s.Selected {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] || strings.ContainsAny(id, " \r\n/?#") || len(id) > 128 {
			continue
		}
		seen[id] = true
		sel = append(sel, id)
	}
	s.Selected = sel

	if len(s.Endpoints) == 0 {
		s.Endpoints = d.Endpoints
	}
	if len(s.Endpoints) > maxEndpoints {
		return fmt.Errorf("at most %d endpoints", maxEndpoints)
	}
	eps := make([]string, 0, len(s.Endpoints))
	for _, e := range s.Endpoints {
		e = strings.TrimSpace(e)
		if !validHostPort(e) {
			return fmt.Errorf("endpoint %q must be host:port", e)
		}
		eps = append(eps, e)
	}
	s.Endpoints = eps

	if s.MTU == 0 {
		s.MTU = d.MTU
	}
	if s.MTU < 576 || s.MTU > 1420 {
		return errors.New("MTU must be between 576 and 1420")
	}
	s.DNS = strings.TrimSpace(s.DNS)
	if s.DNS == "" {
		s.DNS = d.DNS
	}
	for _, part := range strings.Split(s.DNS, ",") {
		part = strings.TrimSpace(part)
		if _, err := netip.ParseAddr(part); err != nil || !dnsPattern.MatchString(part) {
			return fmt.Errorf("DNS server %q is not an IP address", part)
		}
	}
	if s.Keepalive < 0 || s.Keepalive > 600 {
		return errors.New("keepalive is 0 to 600 seconds")
	}

	if p, ok := presets[s.Preset]; ok {
		s.AWG = p
	} else if s.Preset == "" {
		s.Preset = "custom"
	} else if s.Preset != "custom" {
		return errors.New("preset must be standard, light, strong or custom")
	}
	if err := s.AWG.validate(); err != nil {
		return err
	}
	a := s.AutoScan
	if a.Hours == 0 {
		a.Hours = d.AutoScan.Hours
	}
	if a.Count == 0 {
		a.Count = d.AutoScan.Count
	}
	if a.Hours < 1 || a.Hours > 168 || a.Count < 1 || a.Count > maxEndpoints {
		return errors.New("auto scan: every 1-168 hours, keep 1-3 endpoints")
	}
	s.AutoScan = a
	return nil
}

func (a awgSet) validate() error {
	if a.Jc < 0 || a.Jc > 16 || a.Jmin < 0 || a.Jmax < a.Jmin || a.Jmax > 1280 {
		return errors.New("junk packets: Jc 0-16 and 0 <= Jmin <= Jmax <= 1280")
	}
	if a.S1 < 0 || a.S1 > 1132 || a.S2 < 0 || a.S2 > 1188 {
		return errors.New("S1 and S2 are padding sizes (0 for WARP)")
	}
	for name, v := range map[string]string{"H1": a.H1, "H2": a.H2, "H3": a.H3, "H4": a.H4, "I1": a.I1, "I2": a.I2, "I3": a.I3, "I4": a.I4, "I5": a.I5} {
		if v != "" && !iPattern.MatchString(v) {
			return fmt.Errorf("%s may contain printable ASCII only", name)
		}
	}
	return nil
}

// plain says the set carries no obfuscation WARP could refuse: only junk packets.
func (a awgSet) plain() bool {
	return a.S1 == 0 && a.S2 == 0 && a.H1+a.H2+a.H3+a.H4 == ""
}

// names and keeps a node name per endpoint.
func (s settings) nodeName(i int) string {
	if i == 0 {
		return s.Name
	}
	return fmt.Sprintf("%s %d", s.Name, i+1)
}

// inScope says whether a subscription is meant to get WARP.
func (s settings) inScope(subID string) bool {
	if s.Scope != "selected" {
		return true
	}
	for _, id := range s.Selected {
		if id == subID {
			return true
		}
	}
	return false
}

// licensePattern is the shape of a WARP+ license key.
var licensePattern = regexp.MustCompile(`^[A-Za-z0-9]{8}-[A-Za-z0-9]{8}-[A-Za-z0-9]{8}$`)
