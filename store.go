// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// scanRecord is the last scan, kept so the page shows it after a reload and the schedule knows when it ran.
type scanRecord struct {
	At      time.Time       `json:"at"`
	Auto    bool            `json:"auto,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Applied []string        `json:"applied,omitempty"`
	Error   string          `json:"error,omitempty"`
}

type stateFile struct {
	Version  int                `json:"version"`
	Settings settings           `json:"settings"`
	Main     *device            `json:"main,omitempty"`
	Server   *device            `json:"server,omitempty"` // the device of the server's own Xray outbound
	Devices  map[string]*device `json:"devices"`          // by subscription id
	LastScan *scanRecord        `json:"lastScan,omitempty"`
}

// store holds the plugin's state in one file of its data directory. The file has the devices' private keys and
// Cloudflare's tokens: it is written readable by the plugin's own user alone.
type store struct {
	mu   sync.Mutex
	path string
	st   stateFile
}

func openStore(path string) (*store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	s := &store{path: path, st: stateFile{Version: 1, Settings: defaultSettings(), Devices: map[string]*device{}}}
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return s, nil
	case err != nil:
		return nil, err
	}
	var st stateFile
	if err := json.Unmarshal(raw, &st); err != nil {
		// Keep what cannot be read beside the file, and start empty: a plugin that cannot start is worse than one that
		// lost its settings, and the copy can be put back.
		_ = os.WriteFile(path+".broken", raw, 0o600)
		return s, nil
	}
	if st.Devices == nil {
		st.Devices = map[string]*device{}
	}
	def := defaultSettings()
	if err := st.Settings.normalize(); err != nil {
		st.Settings = def
	}
	st.Version = 1
	s.st = st
	return s, nil
}

// saveLocked writes the file; the caller holds the lock.
func (s *store) saveLocked() error {
	raw, err := json.MarshalIndent(s.st, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// settings returns a copy.
func (s *store) settings() settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.st.Settings
	out.Selected = append([]string(nil), out.Selected...)
	out.Endpoints = append([]string(nil), out.Endpoints...)
	out.Xray.Rules = append([]xrayRule(nil), out.Xray.Rules...)
	return out
}

func (s *store) setSettings(v settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.st.Settings
	s.st.Settings = v
	if err := s.saveLocked(); err != nil {
		s.st.Settings = prev
		return err
	}
	return nil
}

// update changes the settings with fn, validates them and saves; nothing changes when any step fails.
func (s *store) update(fn func(*settings) error) (settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.st.Settings
	next.Selected = append([]string(nil), next.Selected...)
	next.Endpoints = append([]string(nil), next.Endpoints...)
	next.Xray.Rules = append([]xrayRule(nil), next.Xray.Rules...)
	if err := fn(&next); err != nil {
		return settings{}, err
	}
	if err := next.normalize(); err != nil {
		return settings{}, err
	}
	prev := s.st.Settings
	s.st.Settings = next
	if err := s.saveLocked(); err != nil {
		s.st.Settings = prev
		return settings{}, err
	}
	return next, nil
}

func (s *store) main() *device {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.st.Main
}

func (s *store) setMain(d *device) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.st.Main
	s.st.Main = d
	if err := s.saveLocked(); err != nil {
		s.st.Main = prev
		return err
	}
	return nil
}

func (s *store) server() *device {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.st.Server
}

func (s *store) setServer(d *device) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.st.Server
	s.st.Server = d
	if err := s.saveLocked(); err != nil {
		s.st.Server = prev
		return err
	}
	return nil
}

func (s *store) device(subID string) *device {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.st.Devices[subID]
}

func (s *store) putDevice(subID string, d *device) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, had := s.st.Devices[subID]
	s.st.Devices[subID] = d
	if err := s.saveLocked(); err != nil {
		if had {
			s.st.Devices[subID] = prev
		} else {
			delete(s.st.Devices, subID)
		}
		return err
	}
	return nil
}

func (s *store) dropDevice(subID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, had := s.st.Devices[subID]
	if !had {
		return nil
	}
	delete(s.st.Devices, subID)
	if err := s.saveLocked(); err != nil {
		s.st.Devices[subID] = prev
		return err
	}
	return nil
}

// deviceIDs lists the subscriptions that have a device.
func (s *store) deviceIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.st.Devices))
	for id := range s.st.Devices {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func (s *store) lastScan() *scanRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.st.LastScan == nil {
		return nil
	}
	c := *s.st.LastScan
	return &c
}

func (s *store) setLastScan(r scanRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.st.LastScan
	s.st.LastScan = &r
	if err := s.saveLocked(); err != nil {
		s.st.LastScan = prev
		return err
	}
	return nil
}

func (s *store) String() string { return fmt.Sprintf("store(%s)", s.path) }
