package engines

import "context"

// Builtin covers everything implemented in-process: image transforms,
// archives, and hashing. It has no external dependency, so it is always ready.
type Builtin struct{}

func (Builtin) ID() ID { return Internal }

func (Builtin) Probe(context.Context) Info {
	return Info{
		ID:        Internal,
		Name:      "internal",
		Path:      "—",
		Version:   "built-in",
		Available: true,
		Detail:    "images, archives, copies, hashes",
	}
}
