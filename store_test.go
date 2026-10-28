// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateSurvivesARestartAndIsPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "state.json")
	st, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.update(func(s *settings) error { s.Enabled, s.Name = true, "Mine"; return nil }); err != nil {
		t.Fatal(err)
	}
	_ = st.setMain(sampleDevice(1))
	_ = st.putDevice("sub1", sampleDevice(2))
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("the state has keys and tokens: mode %v, %v", info.Mode(), err)
	}
	again, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if s := again.settings(); !s.Enabled || s.Name != "Mine" {
		t.Errorf("settings = %+v", s)
	}
	if again.main() == nil || again.main().ID != "dev-1-aaaaaaaa" || again.device("sub1") == nil {
		t.Error("devices lost")
	}
	if ids := again.deviceIDs(); len(ids) != 1 || ids[0] != "sub1" {
		t.Errorf("ids = %v", ids)
	}
}

func TestABrokenStateIsKeptAndTheStoreStartsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	_ = os.WriteFile(path, []byte("{not json"), 0o600)
	st, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.settings().Enabled || st.main() != nil {
		t.Error("should start empty")
	}
	if raw, err := os.ReadFile(path + ".broken"); err != nil || string(raw) != "{not json" {
		t.Errorf("the unreadable file must be kept beside: %v", err)
	}
}

func TestInvalidSettingsOnDiskFallBackToTheDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	_ = os.WriteFile(path, []byte(`{"version":1,"settings":{"mtu":50,"mode":"perClient","scope":"all"},"devices":{}}`), 0o600)
	st, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.settings().MTU != 1280 {
		t.Errorf("settings = %+v", st.settings())
	}
}

func TestUpdateChangesNothingWhenValidationFails(t *testing.T) {
	st := newTestStore(t)
	before := st.settings()
	if _, err := st.update(func(s *settings) error { s.MTU = 5; return nil }); err == nil {
		t.Fatal("accepted")
	}
	after := st.settings()
	if after.MTU != before.MTU {
		t.Error("a refused update changed the settings")
	}
	if s := st.settings(); len(s.Endpoints) != 1 {
		t.Error("copies are independent")
	}
}
