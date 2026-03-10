package detector

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"plugin"
	"sort"
	"strings"
)

type LoadedPlugin struct {
	Path   string
	Plugin DetectorPlugin
}

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
		out = append(out, LoadedPlugin{
			Path:   path,
			Plugin: p,
		})
	}
	return out, nil
}

func LoadPluginFile(path string) (DetectorPlugin, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("path is empty")
	}
	p, err := plugin.Open(path)
	if err != nil {
		return nil, fmt.Errorf("plugin open %s: %w", path, err)
	}
	sym, err := p.Lookup("Plugin")
	if err != nil {
		return nil, fmt.Errorf("plugin lookup %s symbol Plugin: %w", path, err)
	}
	loaded, err := resolvePluginSymbol(sym)
	if err != nil {
		return nil, fmt.Errorf("plugin resolve %s symbol Plugin: %w", path, err)
	}
	return loaded, nil
}

func resolvePluginSymbol(sym any) (DetectorPlugin, error) {
	switch v := sym.(type) {
	case DetectorPlugin:
		if strings.TrimSpace(v.Type()) == "" {
			return nil, errors.New("plugin type is empty")
		}
		return v, nil
	case *DetectorPlugin:
		if v == nil || *v == nil {
			return nil, errors.New("plugin pointer is nil")
		}
		if strings.TrimSpace((*v).Type()) == "" {
			return nil, errors.New("plugin type is empty")
		}
		return *v, nil
	default:
		return nil, fmt.Errorf("symbol type %T does not implement DetectorPlugin", sym)
	}
}
