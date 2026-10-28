// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

// The forms in which a WARP device reaches a client. They are the forms an AmneziaWG node has in the panel, in the order
// the panel's subscription page expects: the legacy amneziawg://key@host?… link, the vpn:// link (Amnezia's own),
// and amneziawg://<base64url config>#name (the config itself, which INCY imports). The page puts the three next to
// the config text as one row with a switch.

package main

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net"
	"net/url"
	"strconv"
	"strings"
)

const allowedIPs4, allowedIPs6 = "0.0.0.0/0", "::/0"

// node is a device at an endpoint, under a name.
type node struct {
	Dev      device
	Endpoint string
	Name     string
}

func oneLine(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ", "\x00", "").Replace(strings.TrimSpace(s))
}

// dnsList is the DNS servers in the form the config takes, without the IPv6 ones when the config has no IPv6.
func (s settings) dnsList() []string {
	var out []string
	for _, d := range strings.Split(s.DNS, ",") {
		d = strings.TrimSpace(d)
		if d == "" || (!s.IPv6 && strings.Contains(d, ":")) {
			continue
		}
		out = append(out, d)
	}
	return out
}

func (s settings) addresses(d device) []string {
	out := []string{d.V4 + "/32"}
	if s.IPv6 {
		out = append(out, d.V6+"/128")
	}
	return out
}

func (s settings) allowedIPs() string {
	if s.IPv6 {
		return allowedIPs4 + ", " + allowedIPs6
	}
	return allowedIPs4
}

// awgKeys lists the obfuscation lines of a config in the order awg-quick files have them. Only what differs from
// a plain WireGuard peer is written: the junk packets, and S/H/I when they are set.
func (a awgSet) lines() [][2]string {
	out := [][2]string{{"Jc", strconv.Itoa(a.Jc)}, {"Jmin", strconv.Itoa(a.Jmin)}, {"Jmax", strconv.Itoa(a.Jmax)}}
	if a.S1 != 0 || a.S2 != 0 {
		out = append(out, [2]string{"S1", strconv.Itoa(a.S1)}, [2]string{"S2", strconv.Itoa(a.S2)})
	}
	for _, kv := range [][2]string{{"H1", a.H1}, {"H2", a.H2}, {"H3", a.H3}, {"H4", a.H4}, {"I1", a.I1}, {"I2", a.I2}, {"I3", a.I3}, {"I4", a.I4}, {"I5", a.I5}} {
		if strings.TrimSpace(kv[1]) != "" {
			out = append(out, [2]string{kv[0], strings.TrimSpace(kv[1])})
		}
	}
	return out
}

// conf is the awg-quick config text of a node. It ends in exactly one newline.
func (s settings) conf(n node) string {
	var b strings.Builder
	b.WriteString("# " + oneLine(n.Name) + "\n[Interface]\n")
	b.WriteString("PrivateKey = " + n.Dev.PrivateKey + "\n")
	b.WriteString("Address = " + strings.Join(s.addresses(n.Dev), ", ") + "\n")
	if dns := s.dnsList(); len(dns) > 0 {
		b.WriteString("DNS = " + strings.Join(dns, ", ") + "\n")
	}
	b.WriteString("MTU = " + strconv.Itoa(s.MTU) + "\n")
	for _, kv := range s.AWG.lines() {
		b.WriteString(kv[0] + " = " + kv[1] + "\n")
	}
	b.WriteString("\n[Peer]\nPublicKey = " + n.Dev.PeerKey + "\n")
	b.WriteString("AllowedIPs = " + s.allowedIPs() + "\n")
	b.WriteString("Endpoint = " + n.Endpoint + "\n")
	if s.Keepalive > 0 {
		b.WriteString("PersistentKeepalive = " + strconv.Itoa(s.Keepalive) + "\n")
	}
	return b.String()
}

func escapeFragment(name string) string {
	return strings.ReplaceAll(url.QueryEscape(name), "+", "%20")
}

// userinfo escapes everything but the unreserved characters, so a key's '+', '/' and '=' survive a URL.
func userinfo(s string) string {
	const unreserved = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~"
	var b strings.Builder
	for _, c := range []byte(s) {
		if strings.IndexByte(unreserved, c) >= 0 {
			b.WriteByte(c)
		} else {
			b.WriteString("%" + strings.ToUpper(strconv.FormatInt(int64(c)+256, 16)[1:]))
		}
	}
	return b.String()
}

// legacyLink is amneziawg://<key>@host:port?<settings>#name.
func (s settings) legacyLink(n node) string {
	host, port, err := net.SplitHostPort(n.Endpoint)
	if err != nil {
		return ""
	}
	q := url.Values{}
	q.Set("publickey", n.Dev.PeerKey)
	q.Set("address", strings.Join(s.addresses(n.Dev), ","))
	q.Set("mtu", strconv.Itoa(s.MTU))
	if dns := s.dnsList(); len(dns) > 0 {
		q.Set("dns", strings.Join(dns, ","))
	}
	if s.Keepalive > 0 {
		q.Set("keepalive", strconv.Itoa(s.Keepalive))
	}
	for _, kv := range s.AWG.lines() {
		q.Set(strings.ToLower(kv[0]), kv[1])
	}
	hostPort := net.JoinHostPort(host, port)
	return "amneziawg://" + userinfo(n.Dev.PrivateKey) + "@" + hostPort + "?" + q.Encode() + "#" + escapeFragment(n.Name)
}

// confLink is amneziawg://<base64url conf>#name.
func confLink(conf, name string) string {
	link := "amneziawg://" + base64.RawURLEncoding.EncodeToString([]byte(conf))
	if name != "" {
		link += "#" + escapeFragment(name)
	}
	return link
}

// vpnLink is Amnezia's share link: "vpn://" and the base64url of Qt's qCompress of a JSON container that holds the
// config and its parts (the structured keys matter: without them AmneziaVPN imports the container but cannot connect).
// The zlib data is stored, not compressed: the subscription page reads the label of a link out of it without a
// decompressor.
func (s settings) vpnLink(n node, conf string) (string, error) {
	host, port, _ := net.SplitHostPort(n.Endpoint)
	portNum, _ := strconv.Atoi(port)
	inner := map[string]any{
		"config":                conf,
		"hostName":              host,
		"port":                  portNum,
		"client_priv_key":       n.Dev.PrivateKey,
		"client_ip":             strings.Join(s.addresses(n.Dev), ", "),
		"server_pub_key":        n.Dev.PeerKey,
		"mtu":                   strconv.Itoa(s.MTU),
		"allowed_ips":           strings.Split(strings.ReplaceAll(s.allowedIPs(), " ", ""), ","),
		"persistent_keep_alive": strconv.Itoa(s.Keepalive),
	}
	for _, kv := range s.AWG.lines() {
		inner[kv[0]] = kv[1]
	}
	innerJSON, err := json.Marshal(inner)
	if err != nil {
		return "", err
	}
	awg := map[string]any{
		"last_config":        string(innerJSON),
		"isThirdPartyConfig": true,
		"transport_proto":    "udp",
		"port":               port,
	}
	if s.AWG.I1 != "" {
		awg["protocol_version"] = "2"
	}
	env := map[string]any{
		"defaultContainer": "amnezia-awg",
		"containers":       []any{map[string]any{"container": "amnezia-awg", "awg": awg}},
		"hostName":         host,
		"description":      oneLine(n.Name),
	}
	dns := s.dnsList()
	if len(dns) > 0 {
		env["dns1"] = dns[0]
	}
	if len(dns) > 1 {
		env["dns2"] = dns[1]
	}
	payload, err := json.Marshal(env)
	if err != nil {
		return "", err
	}
	packed, err := qCompress(payload)
	if err != nil {
		return "", err
	}
	return "vpn://" + base64.RawURLEncoding.EncodeToString(packed), nil
}

// qCompress is Qt's qCompress: the length of the data (four bytes, big endian) and the data as a zlib stream.
func qCompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(data)))
	buf.Write(size[:])
	w, err := zlib.NewWriterLevel(&buf, zlib.NoCompression)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// lines are the three lines of a node, in the order the page groups them.
func (s settings) lines(n node) []string {
	conf := s.conf(n)
	vpn, err := s.vpnLink(n, conf)
	legacy := s.legacyLink(n)
	if err != nil || legacy == "" {
		return nil
	}
	return []string{legacy, vpn, confLink(conf, oneLine(n.Name))}
}
