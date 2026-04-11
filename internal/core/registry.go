package core

import (
	"sync"
)

// PluginRegistry holds registered Channel plugins.
type PluginRegistry struct {
	mu    sync.RWMutex
	chans map[string]Channel
}

func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{chans: make(map[string]Channel)}
}

func (r *PluginRegistry) Register(ch Channel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.chans[ch.Name()] = ch
}

func (r *PluginRegistry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.chans, name)
}

func (r *PluginRegistry) Get(name string) (Channel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ch, ok := r.chans[name]
	return ch, ok
}

func (r *PluginRegistry) List() []Channel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	chans := make([]Channel, 0, len(r.chans))
	for _, ch := range r.chans {
		chans = append(chans, ch)
	}
	return chans
}
