// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
)

const seamRoute = "/links/:subid"

const pageDataMarker = "window.__SUB_PAGE_DATA__="

// appendMissing adds the lines that are not there yet at the end.
func appendMissing(lines, extra []string) []string {
	present := make(map[string]bool, len(lines)+len(extra))
	for _, l := range lines {
		present[strings.TrimSpace(l)] = true
	}
	for _, l := range extra {
		if !present[l] {
			present[l] = true
			lines = append(lines, l)
		}
	}
	return lines
}

// warpLines is the part of the plugin that changes a subscription: it adds the WARP nodes of the subscriber.
type warpLines struct {
	store  *store
	roster *roster
}

// linesFor are the WARP lines of one subscription, none when it is not meant to have any (WARP is off, the subscription
// is out of scope or switched off, or its device is not there yet).
func (w *warpLines) linesFor(subID string) []string {
	s := w.store.settings()
	if !s.Enabled || !s.inScope(subID) || !w.roster.active(subID) {
		return nil
	}
	dev := w.store.main()
	if s.Mode == "perClient" {
		dev = w.store.device(subID)
	}
	if dev == nil {
		return nil
	}
	var out []string
	for i, ep := range s.Endpoints {
		out = append(out, s.lines(node{Dev: *dev, Endpoint: ep, Name: s.nodeName(i)})...)
	}
	return out
}

func (w *warpLines) apply(subID string, lines []string) []string {
	return appendMissing(lines, w.linesFor(subID))
}

// subscriptionSeam answers the seam declared in lucx-plugin.json: the raw subscription gets the lines, and so does the
// list of links the panel embeds in the page a browser shows (window.__SUB_PAGE_DATA__).
func subscriptionSeam(c SeamCall, w *warpLines) (SeamReply, error) {
	if c.Seam.Engine != "sub" || c.Seam.Mode != "after" {
		return SeamReply{}, fmt.Errorf("no seam %s/%s/%s is declared", c.Seam.Engine, c.Seam.Mode, c.Seam.Route)
	}
	if c.Seam.Route != seamRoute || c.Response == nil || c.Response.Status != http.StatusOK || len(c.Response.Body) == 0 {
		return Keep(), nil
	}
	subID := c.Request.Params["subid"]
	if subID == "" {
		return Keep(), nil
	}
	if isHTML(c.Response) {
		body, ok := pageWithLines(c.Response.Body, subID, w)
		if !ok {
			return Keep(), nil
		}
		return Modify(ResponseChange{Body: Bytes(body)}), nil
	}
	text, wrapped, ok := subscriptionText(c.Response)
	if !ok {
		return Keep(), nil
	}
	original := strings.Split(strings.TrimRight(text, "\n"), "\n")
	lines := w.apply(subID, slices.Clone(original))
	if slices.Equal(lines, original) {
		return Keep(), nil
	}
	changed := strings.Join(lines, "\n")
	if wrapped {
		changed = base64.StdEncoding.EncodeToString([]byte(changed))
	}
	return Modify(ResponseChange{Body: Bytes([]byte(changed))}), nil
}

// isHTML says whether the answer is the page a browser shows.
func isHTML(r *SeamResponse) bool {
	for name, values := range r.Headers {
		if !strings.EqualFold(name, "Content-Type") {
			continue
		}
		for _, v := range values {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(v)), "text/html") {
				return true
			}
		}
	}
	return false
}

// subscriptionText returns the lines of a subscription: base64 of one link per line, or the same as plain text (the
// panel's "encrypt subscription" setting off). Anything else is not one.
func subscriptionText(r *SeamResponse) (text string, wrapped, ok bool) {
	if decoded, err := base64.StdEncoding.DecodeString(string(r.Body)); err == nil {
		return string(decoded), true, true
	}
	for name, values := range r.Headers {
		if !strings.EqualFold(name, "Content-Type") {
			continue
		}
		for _, v := range values {
			if strings.HasPrefix(strings.ToLower(v), "text/plain") && strings.Contains(string(r.Body), "://") {
				return string(r.Body), false, true
			}
		}
	}
	return "", false, false
}

// pageWithLines puts the lines into the page's list of links. The page is left alone when anything about it is not as
// expected (a custom theme without the data, a changed format).
func pageWithLines(body []byte, subID string, w *warpLines) ([]byte, bool) {
	at := bytes.Index(body, []byte(pageDataMarker))
	if at < 0 {
		return body, false
	}
	start := at + len(pageDataMarker)
	dec := json.NewDecoder(bytes.NewReader(body[start:]))
	var data map[string]json.RawMessage
	if err := dec.Decode(&data); err != nil {
		return body, false
	}
	end := start + int(dec.InputOffset())
	var links []string
	if raw, ok := data["links"]; !ok || json.Unmarshal(raw, &links) != nil {
		return body, false
	}
	lines := w.apply(subID, slices.Clone(links))
	if slices.Equal(lines, links) {
		return body, false
	}
	rawLinks, err := json.Marshal(lines)
	if err != nil {
		return body, false
	}
	data["links"] = rawLinks
	out, err := json.Marshal(data)
	if err != nil {
		return body, false
	}
	var b bytes.Buffer
	b.Grow(len(body) + len(out))
	b.Write(body[:start])
	b.Write(out)
	b.Write(body[end:])
	return b.Bytes(), true
}
