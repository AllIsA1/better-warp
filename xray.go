// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// WARP as an outbound of this server's Xray. The subscription nodes send a client's traffic straight to Cloudflare; this
// is the other direction: the server's own Xray gets an outbound that leaves through WARP, and routing rules (kept
// here) say what goes into it. The Xray configuration is patched on every build and never stored, so switching the
// plugin off removes the outbound and the rules with it.

const (
	xrayRuleTagPrefix = "lucx-warp-"
	maxXrayRules      = 50
	maxMatchers       = 200
	defaultXrayTag    = "warp"
	endpointTTL       = 10 * time.Minute
)

// xraySet is the operator's choice for the Xray outbound.
type xraySet struct {
	Enabled  bool       `json:"enabled"`
	Tag      string     `json:"tag"`      // the outbound's tag
	Endpoint string     `json:"endpoint"` // host:port; empty means the first endpoint of the connection settings
	Rules    []xrayRule `json:"rules"`
}

// xrayRule sends what it matches through WARP. A request matches when it satisfies every list the rule has.
type xrayRule struct {
	Name     string   `json:"name"`
	Enabled  bool     `json:"enabled"`
	Domains  []string `json:"domains,omitempty"`  // Xray domain matchers: geosite:openai, domain:example.com, full:, keyword:, regexp:
	IPs      []string `json:"ips,omitempty"`      // geoip:us, an address or a CIDR
	Inbounds []string `json:"inbounds,omitempty"` // tags of the inbounds the traffic came in at
}

var (
	xrayTagPattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,31}$`)
	xrayInboundPattern = regexp.MustCompile(`^[A-Za-z0-9_.:@-]{1,128}$`)
	xrayGeoIPPattern   = regexp.MustCompile(`^(geoip|ext):[A-Za-z0-9!_.:,@-]{1,100}$`)
	xrayDomainPattern  = regexp.MustCompile(`^(?:(?:geosite|domain|full|keyword|regexp|ext|dotless):)?[^\s\x00-\x1f]{1,200}$`)
	// A tag of the panel's own outbounds or of Xray's usual ones: the plugin must not stand in for them.
	xrayReservedTags = map[string]bool{"direct": true, "blocked": true, "block": true, "api": true, "proxy": true, "dns-out": true}
)

func defaultXray() xraySet { return xraySet{Tag: defaultXrayTag, Rules: []xrayRule{}} }

func cleanList(in []string, limit int, ok func(string) bool, what string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		if !ok(v) {
			return nil, fmt.Errorf("%q is not a valid %s", v, what)
		}
		seen[v] = true
		out = append(out, v)
	}
	if len(out) > limit {
		return nil, fmt.Errorf("at most %d %s per rule", limit, what)
	}
	return out, nil
}

func validIPMatcher(v string) bool {
	if xrayGeoIPPattern.MatchString(v) {
		return true
	}
	if _, err := netip.ParsePrefix(v); err == nil {
		return true
	}
	_, err := netip.ParseAddr(v)
	return err == nil
}

// normalize fixes what can be fixed and rejects the rest; an empty set becomes the default.
func (x *xraySet) normalize() error {
	x.Tag = strings.TrimSpace(x.Tag)
	if x.Tag == "" {
		x.Tag = defaultXrayTag
	}
	if !xrayTagPattern.MatchString(x.Tag) || xrayReservedTags[strings.ToLower(x.Tag)] {
		return errors.New("the outbound's tag is up to 32 letters, digits, dots, dashes and underscores, and not one of direct, blocked, api, proxy")
	}
	x.Endpoint = strings.TrimSpace(x.Endpoint)
	if x.Endpoint != "" && !validHostPort(x.Endpoint) {
		return fmt.Errorf("endpoint %q must be host:port", x.Endpoint)
	}
	if len(x.Rules) > maxXrayRules {
		return fmt.Errorf("at most %d rules", maxXrayRules)
	}
	rules := make([]xrayRule, 0, len(x.Rules))
	for i, r := range x.Rules {
		r.Name = strings.TrimSpace(r.Name)
		if utf8.RuneCountInString(r.Name) > 60 || strings.ContainsAny(r.Name, "\r\n\x00") {
			return fmt.Errorf("rule %d: the name is at most 60 characters on one line", i+1)
		}
		var err error
		if r.Domains, err = cleanList(r.Domains, maxMatchers, xrayDomainPattern.MatchString, "domain"); err != nil {
			return fmt.Errorf("rule %d: %w", i+1, err)
		}
		if r.IPs, err = cleanList(r.IPs, maxMatchers, validIPMatcher, "address or geoip"); err != nil {
			return fmt.Errorf("rule %d: %w", i+1, err)
		}
		if r.Inbounds, err = cleanList(r.Inbounds, maxMatchers, xrayInboundPattern.MatchString, "inbound tag"); err != nil {
			return fmt.Errorf("rule %d: %w", i+1, err)
		}
		if len(r.Domains)+len(r.IPs)+len(r.Inbounds) == 0 {
			return fmt.Errorf("rule %d: name at least one domain, address or inbound", i+1)
		}
		rules = append(rules, r)
	}
	x.Rules = rules
	return nil
}

// reserved is the three bytes Cloudflare wants in every packet of a device it knows by client id.
func (d device) reserved() []int {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(d.ClientID))
	if err != nil || len(raw) != 3 {
		return nil
	}
	return []int{int(raw[0]), int(raw[1]), int(raw[2])}
}

// endpointResolver turns the endpoint's host name into an address. Xray looks the name up when it starts, and a
// failed lookup there stops the whole core: the plugin hands it an address it has seen work, never a name.
type endpointResolver struct {
	mu     sync.Mutex
	lookup func(ctx context.Context, host string) ([]string, error)
	cache  map[string]resolved
	now    func() time.Time
}

type resolved struct {
	ip string
	at time.Time
}

func newResolver() *endpointResolver {
	return &endpointResolver{
		lookup: func(ctx context.Context, host string) ([]string, error) {
			return net.DefaultResolver.LookupHost(ctx, host)
		},
		cache: map[string]resolved{},
		now:   time.Now,
	}
}

// resolve gives host:port with an address in place of the name. A failed lookup falls back to the last good answer.
func (r *endpointResolver) resolve(ctx context.Context, endpoint string, wantV6 bool) (string, error) {
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		return "", err
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return endpoint, nil
	}
	key := strings.ToLower(host)
	r.mu.Lock()
	c, ok := r.cache[key]
	r.mu.Unlock()
	if ok && r.now().Sub(c.at) < endpointTTL {
		return net.JoinHostPort(c.ip, port), nil
	}
	lctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	addrs, lerr := r.lookup(lctx, host)
	pick := ""
	for _, a := range addrs {
		ip, perr := netip.ParseAddr(a)
		if perr != nil {
			continue
		}
		if ip.Is4() || wantV6 {
			if pick == "" || ip.Is4() {
				pick = ip.String()
			}
		}
	}
	if pick == "" {
		if ok {
			return net.JoinHostPort(c.ip, port), nil
		}
		if lerr == nil {
			lerr = errors.New("no usable address")
		}
		return "", fmt.Errorf("cannot look up %s: %w", host, lerr)
	}
	r.mu.Lock()
	r.cache[key] = resolved{ip: pick, at: r.now()}
	r.mu.Unlock()
	return net.JoinHostPort(pick, port), nil
}

// xrayEndpoint is the endpoint the server's WARP connects to.
func xrayEndpoint(s settings) string {
	if s.Xray.Endpoint != "" {
		return s.Xray.Endpoint
	}
	if len(s.Endpoints) > 0 {
		return s.Endpoints[0]
	}
	return defaultEndpoint
}

// xrayOutbound is the outbound as Xray reads it: its wireguard outbound, which speaks the plain WireGuard WARP needs
// (the junk packets of AmneziaWG are for the clients' side).
func xrayOutbound(s settings, d device, endpoint string) map[string]any {
	addr := []string{d.V4 + "/32"}
	allowed := []string{"0.0.0.0/0"}
	if s.IPv6 && d.V6 != "" {
		addr = append(addr, d.V6+"/128")
		allowed = append(allowed, "::/0")
	}
	peer := map[string]any{"publicKey": d.PeerKey, "allowedIPs": allowed, "endpoint": endpoint}
	if s.Keepalive > 0 {
		peer["keepAlive"] = s.Keepalive
	}
	set := map[string]any{"secretKey": d.PrivateKey, "address": addr, "peers": []any{peer}, "mtu": s.MTU}
	if res := d.reserved(); res != nil {
		set["reserved"] = res
	}
	return map[string]any{"tag": s.Xray.Tag, "protocol": "wireguard", "settings": set}
}

// xrayRules are the routing rules of the enabled entries, in order.
func xrayRules(x xraySet) []any {
	var out []any
	for i, r := range x.Rules {
		if !r.Enabled {
			continue
		}
		rule := map[string]any{"type": "field", "ruleTag": fmt.Sprintf("%s%d", xrayRuleTagPrefix, i+1), "outboundTag": x.Tag}
		if len(r.Domains) > 0 {
			rule["domain"] = r.Domains
		}
		if len(r.IPs) > 0 {
			rule["ip"] = r.IPs
		}
		if len(r.Inbounds) > 0 {
			rule["inboundTag"] = r.Inbounds
		}
		out = append(out, rule)
	}
	return out
}

// canon puts a JSON value in a form that compares equal when the values do.
func canon(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	var back any
	if json.Unmarshal(raw, &back) != nil {
		return string(raw)
	}
	out, _ := json.Marshal(back)
	return string(out)
}

// xrayPatch is the pipeline: it adds the outbound and the rules to the configuration the panel generated. It gives the
// same answer for a document that already has them, which is then no operations at all.
func (a *app) xrayPatch(doc json.RawMessage) ([]PatchOp, error) {
	s := a.store.settings()
	d := a.store.server()
	if !s.Xray.Enabled || d == nil {
		return nil, nil
	}
	var cfg struct {
		API struct {
			Tag string `json:"tag"`
		} `json:"api"`
		Outbounds []json.RawMessage `json:"outbounds"`
		Routing   *struct {
			Rules []json.RawMessage `json:"rules"`
		} `json:"routing"`
	}
	if err := json.Unmarshal(doc, &cfg); err != nil {
		return nil, err
	}
	var ops []PatchOp

	have := false
	for _, raw := range cfg.Outbounds {
		var o struct {
			Tag string `json:"tag"`
		}
		if json.Unmarshal(raw, &o) == nil && o.Tag == s.Xray.Tag {
			have = true
		}
	}
	if !have {
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		defer cancel()
		ep, err := a.res.resolve(ctx, xrayEndpoint(s), s.IPv6)
		if err != nil {
			// An outbound Xray cannot start with would take the whole core down: leave it out and say so on the page.
			a.noteXray(err.Error())
			return nil, nil
		}
		ob := xrayOutbound(s, *d, ep)
		if len(cfg.Outbounds) == 0 {
			ops = append(ops, PatchOp{Op: "add", Path: "/outbounds", Value: []any{ob}})
		} else {
			ops = append(ops, PatchOp{Op: "add", Path: "/outbounds/-", Value: ob})
		}
	}

	want := xrayRules(s.Xray)
	var rules []json.RawMessage
	if cfg.Routing != nil {
		rules = cfg.Routing.Rules
	}
	var ours, others []int // indices in the document's rule list
	for i, raw := range rules {
		var r struct {
			RuleTag string `json:"ruleTag"`
		}
		if json.Unmarshal(raw, &r) == nil && strings.HasPrefix(r.RuleTag, xrayRuleTagPrefix) {
			ours = append(ours, i)
		} else {
			others = append(others, i)
		}
	}
	same := len(ours) == len(want)
	for k, i := range ours {
		if !same {
			break
		}
		var cur any
		if json.Unmarshal(rules[i], &cur) != nil || canon(cur) != canon(want[k]) {
			same = false
		}
	}
	if !same {
		for k := len(ours) - 1; k >= 0; k-- {
			ops = append(ops, PatchOp{Op: "remove", Path: fmt.Sprintf("/routing/rules/%d", ours[k])})
		}
		if len(want) > 0 {
			// After the panel's own API rule, which must keep the API's traffic for itself; ahead of everything else.
			apiTag := cfg.API.Tag
			if apiTag == "" {
				apiTag = "api"
			}
			pos := 0
			for n, i := range others {
				var r struct {
					InboundTag []string `json:"inboundTag"`
				}
				if json.Unmarshal(rules[i], &r) == nil {
					for _, t := range r.InboundTag {
						if t == apiTag {
							pos = n + 1
						}
					}
				}
			}
			switch {
			case cfg.Routing == nil:
				ops = append(ops, PatchOp{Op: "add", Path: "/routing", Value: map[string]any{"rules": want}})
			case rules == nil:
				ops = append(ops, PatchOp{Op: "add", Path: "/routing/rules", Value: want})
			default:
				for k, r := range want {
					ops = append(ops, PatchOp{Op: "add", Path: fmt.Sprintf("/routing/rules/%d", pos+k), Value: r})
				}
			}
		}
	}
	a.noteXray("")
	return ops, nil
}

// noteXray keeps when the configuration was last built with the outbound and why it was not.
func (a *app) noteXray(problem string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.xrayAt, a.xrayErr = a.now(), problem
}

// ensureServerDevice gives the server a WARP device of its own. Sharing the main account's would make two tunnels
// with one key, and Cloudflare serves only one of them at a time.
func (a *app) ensureServerDevice(ctx context.Context) error {
	if a.store.server() != nil {
		return nil
	}
	rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	d, err := a.cf.Register(rctx, "lucx-warp-server")
	if err != nil {
		return err
	}
	if err := a.store.setServer(&d); err != nil {
		_ = a.cf.Delete(rctx, d)
		return err
	}
	return nil
}

// setXray saves the outbound's settings. Turning it on registers the server's device first, so it never saves a switch
// that has nothing behind it.
func (a *app) setXray(ctx context.Context, in xraySet) (xraySet, error) {
	if err := in.normalize(); err != nil {
		return xraySet{}, err
	}
	if in.Enabled {
		if err := a.ensureServerDevice(ctx); err != nil {
			return xraySet{}, err
		}
	}
	cur, err := a.store.update(func(s *settings) error {
		s.Xray = in
		return nil
	})
	if err != nil {
		return xraySet{}, err
	}
	a.noteXray("")
	return cur.Xray, nil
}

// deleteServerDevice takes the server's device back at Cloudflare, forgets it and switches the outbound off.
func (a *app) deleteServerDevice(ctx context.Context) error {
	d := a.store.server()
	if d == nil {
		return nil
	}
	dctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := a.cf.Delete(dctx, *d); err != nil && !strings.Contains(err.Error(), "refused") {
		return err
	}
	if _, err := a.store.update(func(s *settings) error { s.Xray.Enabled = false; return nil }); err != nil {
		return err
	}
	return a.store.setServer(nil)
}

// restartXray asks the panel to restart its Xray, which is when it builds the configuration again.
func (a *app) restartXray(ctx context.Context) error {
	return a.api.Do(ctx, "POST", "/server/restartXrayService", nil, nil)
}

type xrayView struct {
	Enabled    bool       `json:"enabled"`
	Registered bool       `json:"registered"`
	DeviceID   string     `json:"deviceId,omitempty"`
	Endpoint   string     `json:"endpoint"` // the one in use
	BuiltAt    *time.Time `json:"builtAt,omitempty"`
	Problem    string     `json:"problem,omitempty"`
}

func (a *app) xrayView(s settings) xrayView {
	v := xrayView{Enabled: s.Xray.Enabled, Endpoint: xrayEndpoint(s)}
	if d := a.store.server(); d != nil {
		v.Registered, v.DeviceID = true, shortID(d.ID)
	}
	a.mu.Lock()
	v.BuiltAt, v.Problem = tp(a.xrayAt), a.xrayErr
	a.mu.Unlock()
	return v
}
