package channel

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"plugin"
	"sort"
	"strings"
)

// LoadedPlugin holds a dynamically loaded channel plugin.
type LoadedPlugin struct {
	Path   string
	Plugin ChannelPlugin
}

// PluginFiles returns all .so files in the given directory.
func PluginFiles(dir string) ([]string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(e.Name()))
		if !strings.HasSuffix(name, ".so") {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	sort.Strings(out)
	return out, nil
}

// LoadPluginDir loads all .so channel plugins from a directory.
func LoadPluginDir(dir string) ([]LoadedPlugin, error) {
	files, err := PluginFiles(dir)
	if err != nil {
		return nil, err
	}
	out := make([]LoadedPlugin, 0, len(files))
	for _, path := range files {
		p, loadErr := LoadPluginFile(path)
		if loadErr != nil {
			return nil, loadErr
		}
		out = append(out, LoadedPlugin{Path: path, Plugin: p})
	}
	return out, nil
}

// LoadPluginFile loads a single .so file as a ChannelPlugin.
func LoadPluginFile(path string) (ChannelPlugin, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("path is empty")
	}
	p, err := plugin.Open(path)
	if err != nil {
		return nil, fmt.Errorf("plugin open %s: %w", path, err)
	}
	sym, err := p.Lookup("ChannelPlugin")
	if err != nil {
		return nil, fmt.Errorf("plugin lookup %s symbol ChannelPlugin: %w", path, err)
	}
	return resolvePluginSymbol(sym)
}

func resolvePluginSymbol(sym any) (ChannelPlugin, error) {
	switch v := sym.(type) {
	case ChannelPlugin:
		if strings.TrimSpace(v.Type()) == "" {
			return nil, errors.New("plugin type is empty")
		}
		return v, nil
	case *ChannelPlugin:
		if v == nil || *v == nil {
			return nil, errors.New("plugin pointer is nil")
		}
		if strings.TrimSpace((*v).Type()) == "" {
			return nil, errors.New("plugin type is empty")
		}
		return *v, nil
	default:
		return nil, fmt.Errorf("symbol type %T does not implement ChannelPlugin", sym)
	}
}
