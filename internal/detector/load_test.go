package detector

import "testing"

func TestLoadPluginFile_InvalidPath(t *testing.T) {
	t.Parallel()

	if _, err := LoadPluginFile(""); err == nil {
		t.Fatalf("expected error for empty path")
	}
	if _, err := LoadPluginFile("/tmp/not-exists-12345.so"); err == nil {
		t.Fatalf("expected error for missing file")
	}
}

func TestLoadPluginDir_EmptyDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	got, err := LoadPluginDir(dir)
	if err != nil {
		t.Fatalf("LoadPluginDir: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 plugins, got %d", len(got))
	}
}

func TestResolvePluginSymbol_InvalidAndPointerCases(t *testing.T) {
	t.Parallel()

	if _, err := resolvePluginSymbol(struct{}{}); err == nil {
		t.Fatalf("expected type mismatch error")
	}

	var nilPlugin *DetectorPlugin
	if _, err := resolvePluginSymbol(nilPlugin); err == nil {
		t.Fatalf("expected nil plugin pointer error")
	}

	p := DetectorPlugin(stubPlugin{typ: "http_check"})
	got, err := resolvePluginSymbol(&p)
	if err != nil {
		t.Fatalf("resolvePluginSymbol pointer: %v", err)
	}
	if got.Type() != "http_check" {
		t.Fatalf("unexpected plugin type %q", got.Type())
	}

	_, err = resolvePluginSymbol(stubPlugin{typ: ""})
	if err == nil || err.Error() == "" {
		t.Fatalf("expected empty type error")
	}
}
