package channel

import (
	"github.com/aak1247/logtap/internal/config"
)

// Bootstrap creates a Registry with all built-in channels registered,
// and returns both the Registry and a Service wrapping it.
// Built-in channels are registered by the caller via RegisterBuiltinFunc.
func Bootstrap(cfg config.Config) (*Registry, *Service, error) {
	reg := NewRegistry()
	svc := NewService(reg)
	return reg, svc, nil
}
