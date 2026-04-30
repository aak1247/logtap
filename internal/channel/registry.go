package channel

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

var ErrAlreadyRegistered = errors.New("channel already registered")

// Registry stores and retrieves ChannelPlugin instances by type.
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]ChannelPlugin
	meta    map[string]Descriptor
}

func NewRegistry() *Registry {
	return &Registry{
		plugins: map[string]ChannelPlugin{},
		meta:    map[string]Descriptor{},
	}
}

func (r *Registry) RegisterStatic(p ChannelPlugin) error {
	return r.register(p, ModeStatic, "")
}

func (r *Registry) RegisterDynamic(path string, p ChannelPlugin) error {
	return r.register(p, ModePlugin, strings.TrimSpace(path))
}

func (r *Registry) register(p ChannelPlugin, mode RegistrationMode, path string) error {
	if r == nil {
		return errors.New("registry is nil")
	}
	if p == nil {
		return errors.New("plugin is nil")
	}
	typ := strings.ToLower(strings.TrimSpace(p.Type()))
	if typ == "" {
		return errors.New("plugin type is empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.plugins[typ]; ok {
		return fmt.Errorf("%w: %s", ErrAlreadyRegistered, typ)
	}
	r.plugins[typ] = p
	r.meta[typ] = Descriptor{Type: typ, Mode: mode, Path: path}
	return nil
}

func (r *Registry) Get(typ string) (ChannelPlugin, bool) {
	if r == nil {
		return nil, false
	}
	key := strings.ToLower(strings.TrimSpace(typ))
	if key == "" {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[key]
	return p, ok
}

func (r *Registry) List() []Descriptor {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Descriptor, 0, len(r.meta))
	for _, d := range r.meta {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Type < out[j].Type
	})
	return out
}
