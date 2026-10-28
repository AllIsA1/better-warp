// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"log"
	"os/signal"
	"path/filepath"
	"syscall"
)

func main() {
	var api panelAPI
	var eng engineAPI
	p, err := NewPanel()
	if err != nil {
		log.Printf("better-warp: %v; subscriptions are left as they are", err)
		api, eng = failingPanel{err}, failingEngine{errNoEngine}
	} else {
		api, eng = p, coresEngine{peer: p.Peer(enginePlugin)}
	}
	st, err := openStore(filepath.Join(StateDir(), "state.json"))
	if err != nil {
		log.Fatalf("better-warp: %v", err)
	}
	a := newApp(api, st, newCFClient(), eng)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	go a.prov.run(ctx)
	go a.scheduleLoop(ctx)

	log.Print("better-warp plugin ready")
	if err := Serve(newMux(a)); err != nil {
		log.Fatal(err)
	}
}

type failingPanel struct{ err error }

func (f failingPanel) Do(context.Context, string, string, any, any) error { return f.err }
