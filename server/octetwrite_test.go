// SPDX-License-Identifier: MIT

package server

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/otfabric/go-dnp3/app"
)

// octetGate is a handler whose WriteOctetString refuses the octet string "no".
type octetGate struct {
	NopHandler
	seen []string
}

func (g *octetGate) WriteAttribute(context.Context, uint8, app.Attribute) error { return nil }

func (g *octetGate) WriteOctetString(_ context.Context, _ uint32, data []byte) error {
	g.seen = append(g.seen, string(data))
	if string(data) == "no" {
		return errors.New("refused")
	}
	return nil
}

// events are the octet string events (g111) of a class 1 read.
func octetEvents(t *testing.T, m *master, seq uint8) int {
	t.Helper()
	m.send(req(seq, app.FuncRead, app.AllObjects(app.G60V2)))
	n := 0
	for _, h := range m.recvFunc(app.FuncResponse).Headers {
		if h.GV.Group() == 111 {
			n += len(h.Objects)
		}
	}
	return n
}

// TestOctetStringWriteHandlerDecidesFirst: WriteOctetString is asked before
// the octet string is stored. One it refuses is answered with IIN2.2 and
// leaves the point and the event buffer as they were; one it accepts is
// stored and reported as an event; one of a point that does not exist is
// answered with IIN2.2 without asking it.
func TestOctetStringWriteHandlerDecidesFirst(t *testing.T) {
	g := &octetGate{}
	m := newMaster(t, WithHandler(g))
	db := m.srv.Database()
	if err := db.Add(OctetString, 0, PointConfig{Class: 1}, app.OctetString{Data: []byte("ok")}); err != nil {
		t.Fatal(err)
	}
	m.send(req(1, app.FuncWrite, app.Ranged(app.GV(110, 2), 0, app.OctetString{Data: []byte("no")})))
	if f := m.recvFunc(app.FuncResponse); !f.IIN.Has(app.IINParameterError) {
		t.Fatalf("a refused write: %v", f)
	}
	if v, _ := db.Get(OctetString, 0); !bytes.Equal(v.(app.OctetString).Data, []byte("ok")) {
		t.Fatalf("a refused write was stored: %q", v.(app.OctetString).Data)
	}
	if n := octetEvents(t, m, 2); n != 0 {
		t.Fatalf("a refused write made %d events", n)
	}
	m.send(req(3, app.FuncWrite, app.Ranged(app.GV(110, 3), 0, app.OctetString{Data: []byte("yes")})))
	if f := m.recvFunc(app.FuncResponse); f.IIN.Has(app.IINParameterError) {
		t.Fatalf("an accepted write: %v", f)
	}
	if v, _ := db.Get(OctetString, 0); !bytes.Equal(v.(app.OctetString).Data, []byte("yes")) {
		t.Fatalf("an accepted write was not stored: %q", v.(app.OctetString).Data)
	}
	if n := octetEvents(t, m, 4); n != 1 {
		t.Fatalf("an accepted write made %d events", n)
	}
	m.send(req(5, app.FuncWrite, app.Ranged(app.GV(110, 3), 9, app.OctetString{Data: []byte("abc")})))
	if f := m.recvFunc(app.FuncResponse); !f.IIN.Has(app.IINParameterError) {
		t.Fatalf("a write of a missing point: %v", f)
	}
	if len(g.seen) != 2 || g.seen[0] != "no" || g.seen[1] != "yes" {
		t.Fatalf("the handler was asked %q", g.seen)
	}
}
