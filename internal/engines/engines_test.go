package engines

import (
	"context"
	"testing"
	"time"
)

type fakeEngine struct {
	id    ID
	info  Info
	delay time.Duration
}

func (f fakeEngine) ID() ID { return f.id }

func (f fakeEngine) Probe(ctx context.Context) Info {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return Info{ID: f.id, Available: false, Err: ctx.Err()}
		}
	}
	return f.info
}

func TestProbeAllPreservesRegistrationOrder(t *testing.T) {
	slow := fakeEngine{id: "slow", delay: 40 * time.Millisecond, info: Info{ID: "slow", Available: true}}
	fast := fakeEngine{id: "fast", info: Info{ID: "fast", Available: true}}
	reg := NewRegistry(slow, fast)

	infos := reg.ProbeAll(context.Background())
	if len(infos) != 2 {
		t.Fatalf("got %d infos, want 2", len(infos))
	}
	if infos[0].ID != "slow" || infos[1].ID != "fast" {
		t.Errorf("order = %s, %s; want slow, fast", infos[0].ID, infos[1].ID)
	}
}

func TestProbeAllRespectsCancellation(t *testing.T) {
	slow := fakeEngine{id: "slow", delay: time.Second}
	reg := NewRegistry(slow)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	infos := reg.ProbeAll(ctx)
	if len(infos) != 1 || infos[0].Available {
		t.Fatalf("infos = %+v, want one unavailable engine", infos)
	}
	if infos[0].Err == nil {
		t.Error("cancelled probe did not report an error")
	}
}

func TestNilRegistry(t *testing.T) {
	var reg *Registry
	if reg.Len() != 0 {
		t.Errorf("Len() = %d, want 0", reg.Len())
	}
	if got := reg.ProbeAll(context.Background()); got != nil {
		t.Errorf("ProbeAll() = %v, want nil", got)
	}
}

func TestBuiltinIsAlwaysReady(t *testing.T) {
	info := Builtin{}.Probe(context.Background())
	if !info.Available {
		t.Fatal("builtin engine is not available")
	}
	if info.ID != Internal {
		t.Errorf("ID = %q, want %q", info.ID, Internal)
	}
}
