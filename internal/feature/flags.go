package feature

import (
	"sync"
)

type Flags struct {
	mu    sync.RWMutex
	flags map[string]bool
}

func NewFlags() *Flags {
	return &Flags{
		flags: make(map[string]bool),
	}
}

func (f *Flags) Set(name string, enabled bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.flags[name] = enabled
}

func (f *Flags) Enabled(name string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.flags[name]
}

func (f *Flags) All() map[string]bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	result := make(map[string]bool, len(f.flags))
	for k, v := range f.flags {
		result[k] = v
	}
	return result
}

const (
	FlagRiskEngine   = "risk_engine"
	FlagBehavior     = "behavior"
	FlagAdaptiveRate = "adaptive_rate"
	FlagReplayDetect = "replay_detect"
	FlagAdminPanel   = "admin_panel"
)
