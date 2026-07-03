package detector

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

var ErrAlreadyRegistered = errors.New("detector already registered")

type Registry struct {
	mu       sync.RWMutex
	plugins  map[string]DetectorPlugin
	meta     map[string]Descriptor
	packages map[string]PackageManifest
	views    map[string][]ViewDescriptor
}

func NewRegistry() *Registry {
	return &Registry{
		plugins:  map[string]DetectorPlugin{},
		meta:     map[string]Descriptor{},
		packages: map[string]PackageManifest{},
		views:    map[string][]ViewDescriptor{},
	}
}

func (r *Registry) RegisterStatic(p DetectorPlugin) error {
	return r.register(p, ModeStatic, "", "")
}

func (r *Registry) RegisterDynamic(path string, p DetectorPlugin) error {
	return r.register(p, ModePlugin, strings.TrimSpace(path), "")
}

func (r *Registry) RegisterPackage(pkg DetectorPackage) error {
	if r == nil {
		return errors.New("registry is nil")
	}
	if pkg == nil {
		return errors.New("package is nil")
	}
	manifest := pkg.Manifest()
	manifest.ID = strings.TrimSpace(manifest.ID)
	if manifest.ID == "" {
		return errors.New("package id is empty")
	}
	for _, p := range pkg.Detectors() {
		if err := r.register(p, ModeStatic, "", manifest.ID); err != nil {
			return err
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if len(manifest.Detectors) == 0 {
		for _, p := range pkg.Detectors() {
			manifest.Detectors = append(manifest.Detectors, strings.ToLower(strings.TrimSpace(p.Type())))
		}
	}
	r.packages[manifest.ID] = manifest
	for _, v := range pkg.Views() {
		v.PackageID = manifest.ID
		r.views[manifest.ID] = append(r.views[manifest.ID], v)
	}
	return nil
}

func (r *Registry) register(p DetectorPlugin, mode RegistrationMode, path string, packageID string) error {
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
	r.meta[typ] = Descriptor{Type: typ, Mode: mode, Path: path, PackageID: packageID}
	return nil
}

func (r *Registry) Get(typ string) (DetectorPlugin, bool) {
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

func (r *Registry) ListPackages() []PackageManifest {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]PackageManifest, 0, len(r.packages))
	for _, p := range r.packages {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func (r *Registry) ListViews(detectorType string) []ViewDescriptor {
	if r == nil {
		return nil
	}
	detectorType = strings.ToLower(strings.TrimSpace(detectorType))
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []ViewDescriptor{}
	for _, views := range r.views {
		for _, v := range views {
			if detectorType == "" || strings.EqualFold(v.DetectorType, detectorType) {
				out = append(out, v)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}
