// SPDX-License-Identifier: MIT

package e2e

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	dnp3 "github.com/otfabric/go-dnp3"
	"github.com/otfabric/go-dnp3/app"
	"github.com/otfabric/go-dnp3/server"
)

// octetGate is a server.WriteHandler that refuses the octet string "no".
type octetGate struct {
	server.NopHandler
	mu   sync.Mutex
	seen []uint32
}

func (g *octetGate) WriteAttribute(context.Context, uint8, app.Attribute) error { return nil }

func (g *octetGate) WriteOctetString(_ context.Context, index uint32, data []byte) error {
	g.mu.Lock()
	g.seen = append(g.seen, index)
	g.mu.Unlock()
	if string(data) == "no" {
		return errors.New("refused by the application")
	}
	return nil
}

// TestOctetStringWriteRefused: the master writes two octet strings in one
// WRITE and the outstation's WriteHandler refuses the first. The master
// gets IIN2.2 (PARAMETER_ERROR); the refused point keeps its value and
// makes no event, the accepted one is stored and reported as one octet
// string event (g111).
func TestOctetStringWriteRefused(t *testing.T) {
	g := &octetGate{}
	p := newPair(t, func(db *server.Database) {
		_ = db.Add(server.OctetString, 0, server.PointConfig{Class: 1}, app.OctetString{Data: []byte("old")})
		_ = db.Add(server.OctetString, 1, server.PointConfig{Class: 1}, app.OctetString{Data: []byte("old")})
	}, []server.Option{server.WithHandler(g)})

	_, err := p.c.Write(ctx(t),
		app.Ranged(app.GV(110, 2), 0, app.OctetString{Data: []byte("no")}),
		app.Ranged(app.GV(110, 3), 1, app.OctetString{Data: []byte("yes")}))
	var ie *dnp3.IINError
	if !errors.As(err, &ie) || !ie.IIN.Has(app.IINParameterError) {
		t.Fatalf("WRITE with a refused octet string: %v, want IIN2.2", err)
	}
	g.mu.Lock()
	seen := slices.Clone(g.seen)
	g.mu.Unlock()
	if !slices.Equal(seen, []uint32{0, 1}) {
		t.Fatalf("the handler was asked about %v, want [0 1]", seen)
	}
	for i, want := range []string{"old", "yes"} {
		v, _ := p.db.Get(server.OctetString, uint32(i))
		if got := string(v.(app.OctetString).Data); got != want {
			t.Errorf("octet string %d is %q, want %q", i, got, want)
		}
	}
	res, err := p.c.EventPoll(ctx(t), 1)
	if err != nil {
		t.Fatal(err)
	}
	var events []uint32
	for _, m := range res.Measurements() {
		if m.GV.Group() == 111 {
			events = append(events, m.Index)
		}
	}
	if !slices.Equal(events, []uint32{1}) {
		t.Errorf("octet string events of %v, want [1]", events)
	}
}
