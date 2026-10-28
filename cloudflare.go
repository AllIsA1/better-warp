// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

// Cloudflare's WARP registration API: a device is registered with a public key, and Cloudflare answers with the
// addresses it assigned, the endpoint and its public key. An anonymous registration needs no account of any kind.

package main

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

// device is one registered WARP device with everything needed to build its configs and to manage it at Cloudflare.
type device struct {
	ID         string    `json:"id"`
	Token      string    `json:"token"` // Cloudflare's token for this device: license, account, deletion
	PrivateKey string    `json:"privateKey"`
	PeerKey    string    `json:"peerKey"`
	V4         string    `json:"v4"`
	V6         string    `json:"v6"`
	Host       string    `json:"host,omitempty"`    // the endpoint Cloudflare names for the device
	License    string    `json:"license,omitempty"` // the account's own key, from the registration
	PlusKey    string    `json:"plusKey,omitempty"` // a WARP+ key the operator applied
	Created    time.Time `json:"created"`
}

// accountInfo is what Cloudflare says about the account a device belongs to.
type accountInfo struct {
	Type    string `json:"type"` // free, limited or unlimited (WARP+)
	Plus    bool   `json:"plus"`
	Quota   int64  `json:"quota,omitempty"` // WARP+ data left, bytes
	License string `json:"license,omitempty"`
}

// cfProfile is one way of talking to the API; the first is tried, and the next one when it fails in a way that
// looks like the API generation being gone.
type cfProfile struct {
	Base          string
	ClientVersion string
	UserAgent     string
	DeviceType    string
}

var cfProfiles = []cfProfile{
	{"https://api.cloudflareclient.com/v0a4005", "a-6.30-3596", "okhttp/3.12.1", "PC"},
	{"https://api.cloudflareclient.com/v0a737", "", "okhttp/3.12.1", "Android"},
}

const maxCFResponse = 256 << 10

// errRateLimited is Cloudflare asking to slow down.
var errRateLimited = errors.New("Cloudflare is limiting registrations from this address: try again later")

type cfClient struct {
	http     *http.Client
	profiles []cfProfile
	now      func() time.Time
}

func newCFClient() *cfClient {
	return &cfClient{http: &http.Client{Timeout: 20 * time.Second}, profiles: cfProfiles, now: time.Now}
}

func newKeypair() (priv, pub string, err error) {
	var raw [32]byte
	if _, err = rand.Read(raw[:]); err != nil {
		return "", "", err
	}
	raw[0] &= 248
	raw[31] &= 127
	raw[31] |= 64
	key, err := ecdh.X25519().NewPrivateKey(raw[:])
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(raw[:]), base64.StdEncoding.EncodeToString(key.PublicKey().Bytes()), nil
}

func validKey(s string) bool {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	return err == nil && len(raw) == 32
}

type regResponse struct {
	ID      string `json:"id"`
	Token   string `json:"token"`
	Account struct {
		License string `json:"license"`
	} `json:"account"`
	Config struct {
		Peers []struct {
			PublicKey string `json:"public_key"`
			Endpoint  struct {
				Host string `json:"host"`
			} `json:"endpoint"`
		} `json:"peers"`
		Interface struct {
			Addresses struct {
				V4 string `json:"v4"`
				V6 string `json:"v6"`
			} `json:"addresses"`
		} `json:"interface"`
	} `json:"config"`
}

// toDevice checks what Cloudflare sent and makes a device of it.
func (r regResponse) toDevice(priv string, now time.Time) (device, error) {
	if r.ID == "" || r.Token == "" {
		return device{}, errors.New("Cloudflare's answer has no device id or token")
	}
	if len(r.Config.Peers) == 0 || !validKey(r.Config.Peers[0].PublicKey) {
		return device{}, errors.New("Cloudflare's answer has no valid peer key")
	}
	v4, err4 := netip.ParseAddr(strings.TrimSpace(r.Config.Interface.Addresses.V4))
	v6, err6 := netip.ParseAddr(strings.TrimSpace(r.Config.Interface.Addresses.V6))
	if err4 != nil || !v4.Is4() {
		return device{}, errors.New("Cloudflare's answer has no valid IPv4 address")
	}
	if err6 != nil || !v6.Is6() {
		return device{}, errors.New("Cloudflare's answer has no valid IPv6 address")
	}
	host := strings.TrimSpace(r.Config.Peers[0].Endpoint.Host)
	if !validHostPort(host) {
		host = ""
	}
	return device{
		ID: r.ID, Token: r.Token, PrivateKey: priv, PeerKey: strings.TrimSpace(r.Config.Peers[0].PublicKey),
		V4: v4.String(), V6: v6.String(), Host: host, License: strings.TrimSpace(r.Account.License), Created: now,
	}, nil
}

// do sends a request through the profiles; the body is built again for each.
func (c *cfClient) do(ctx context.Context, method, path, token string, body any, out any) error {
	var lastErr error
	for i, p := range c.profiles {
		var rd io.Reader
		if body != nil {
			b := body
			if f, ok := body.(func(cfProfile) any); ok {
				b = f(p)
			}
			raw, err := json.Marshal(b)
			if err != nil {
				return err
			}
			rd = bytes.NewReader(raw)
		}
		req, err := http.NewRequestWithContext(ctx, method, p.Base+path, rd)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", p.UserAgent)
		if p.ClientVersion != "" {
			req.Header.Set("CF-Client-Version", p.ClientVersion)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		raw, rerr := io.ReadAll(io.LimitReader(resp.Body, maxCFResponse))
		resp.Body.Close()
		switch {
		case rerr != nil:
			lastErr = rerr
			continue
		case resp.StatusCode == http.StatusTooManyRequests:
			return errRateLimited
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			if out == nil || len(bytes.TrimSpace(raw)) == 0 {
				return nil
			}
			if err := json.Unmarshal(raw, out); err != nil {
				lastErr = fmt.Errorf("Cloudflare's answer is not what was expected: %w", err)
				continue
			}
			return nil
		case token != "" && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound):
			// The device or its token is not known: another API generation will not know it either.
			return fmt.Errorf("Cloudflare refused (HTTP %d): %s", resp.StatusCode, cfMessage(raw))
		default:
			lastErr = fmt.Errorf("Cloudflare answered HTTP %d: %s", resp.StatusCode, cfMessage(raw))
			if i == len(c.profiles)-1 {
				return lastErr
			}
		}
	}
	if lastErr == nil {
		lastErr = errors.New("Cloudflare did not answer")
	}
	return lastErr
}

func cfMessage(raw []byte) string {
	var e struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if json.Unmarshal(raw, &e) == nil && len(e.Errors) > 0 && e.Errors[0].Message != "" {
		return truncate(e.Errors[0].Message, 160)
	}
	return truncate(strings.TrimSpace(string(raw)), 160)
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// Register makes a new anonymous device.
func (c *cfClient) Register(ctx context.Context, name string) (device, error) {
	priv, pub, err := newKeypair()
	if err != nil {
		return device{}, err
	}
	var resp regResponse
	tos := c.now().UTC().Format("2006-01-02T15:04:05.000Z")
	err = c.do(ctx, http.MethodPost, "/reg", "", func(p cfProfile) any {
		body := map[string]any{"key": pub, "tos": tos, "type": p.DeviceType}
		if p.DeviceType == "Android" {
			body["install_id"], body["fcm_token"], body["warp_enabled"], body["locale"] = "", "", true, "en_US"
		} else {
			body["model"], body["name"] = "x-ui", name
		}
		return body
	}, &resp)
	if err != nil {
		return device{}, err
	}
	return resp.toDevice(priv, c.now())
}

// Account reads the account of a device.
func (c *cfClient) Account(ctx context.Context, d device) (accountInfo, error) {
	var resp struct {
		AccountType string `json:"account_type"`
		WarpPlus    bool   `json:"warp_plus"`
		PremiumData int64  `json:"premium_data"`
		Quota       int64  `json:"quota"`
		License     string `json:"license"`
	}
	if err := c.do(ctx, http.MethodGet, "/reg/"+d.ID+"/account", d.Token, nil, &resp); err != nil {
		return accountInfo{}, err
	}
	info := accountInfo{Type: resp.AccountType, Plus: resp.WarpPlus || resp.AccountType == "unlimited", Quota: max(resp.PremiumData, resp.Quota), License: resp.License}
	if info.Type == "" {
		info.Type = "free"
	}
	return info, nil
}

// SetLicense attaches a WARP+ license key to the device's account.
func (c *cfClient) SetLicense(ctx context.Context, d device, license string) error {
	var resp struct {
		ID string `json:"id"`
	}
	if err := c.do(ctx, http.MethodPut, "/reg/"+d.ID+"/account", d.Token, map[string]string{"license": license}, &resp); err != nil {
		return err
	}
	if resp.ID == "" {
		return errors.New("Cloudflare did not accept the license key")
	}
	return nil
}

// Delete removes the device at Cloudflare.
func (c *cfClient) Delete(ctx context.Context, d device) error {
	return c.do(ctx, http.MethodDelete, "/reg/"+d.ID, d.Token, nil, nil)
}
