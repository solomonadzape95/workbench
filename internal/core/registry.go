package core

import (
	"fmt"
	"sync"
)

// Registry holds the operations available in this build. Registration order is
// preserved so the operation picker is stable between runs.
type Registry struct {
	mu    sync.RWMutex
	ops   map[string]Operation
	order []string
}

func NewRegistry(ops ...Operation) *Registry {
	r := &Registry{ops: make(map[string]Operation, len(ops))}
	for _, op := range ops {
		if err := r.Add(op); err != nil {
			panic(err) // duplicate IDs are a programming error, caught at boot
		}
	}
	return r
}

func (r *Registry) Add(op Operation) error {
	spec := op.Spec()
	if spec.ID == "" {
		return fmt.Errorf("operation %T has an empty ID", op)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.ops[spec.ID]; exists {
		return fmt.Errorf("operation %q is registered twice", spec.ID)
	}
	r.ops[spec.ID] = op
	r.order = append(r.order, spec.ID)
	return nil
}

func (r *Registry) Get(id string) (Operation, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	op, ok := r.ops[id]
	return op, ok
}

func (r *Registry) All() []Operation {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Operation, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.ops[id])
	}
	return out
}

func (r *Registry) Len() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.order)
}
