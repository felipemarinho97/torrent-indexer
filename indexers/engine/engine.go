package engine

import "net/http"

// Engine is the common interface implemented by indexers.
type Engine interface {
	ID() string
	Label() string
	Handler() http.HandlerFunc
}

// Registry holds all registered engines keyed by ID.
type Registry struct {
	engines map[string]Engine
}

func NewRegistry() *Registry {
	return &Registry{engines: make(map[string]Engine)}
}

// Register adds an engine to the registry.
// If an engine with the same ID is already registered, it is NOT overwritten.
func (r *Registry) Register(e Engine) {
	if _, exists := r.engines[e.ID()]; exists {
		return
	}
	r.engines[e.ID()] = e
}

// All returns a copy of all registered engines.
func (r *Registry) All() map[string]Engine {
	out := make(map[string]Engine, len(r.engines))
	for k, v := range r.engines {
		out[k] = v
	}
	return out
}
