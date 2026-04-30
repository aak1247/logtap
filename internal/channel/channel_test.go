package channel

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// mockPlugin is a test channel plugin.
type mockPlugin struct {
	typeStr    string
	sendErr    error
	lastMsg    Message
	lastCfg    json.RawMessage
	validErr   error
	schemaResp json.RawMessage
}

func (m *mockPlugin) Type() string                         { return m.typeStr }
func (m *mockPlugin) ConfigSchema() json.RawMessage        { return m.schemaResp }
func (m *mockPlugin) ValidateConfig(cfg json.RawMessage) error {
	m.lastCfg = cfg
	return m.validErr
}
func (m *mockPlugin) Send(_ context.Context, msg Message, cfg json.RawMessage) error {
	m.lastMsg = msg
	m.lastCfg = cfg
	return m.sendErr
}

func TestRegistry_RegisterStatic(t *testing.T) {
	r := NewRegistry()
	p := &mockPlugin{typeStr: "test_channel"}
	if err := r.RegisterStatic(p); err != nil {
		t.Fatal(err)
	}
	got, ok := r.Get("test_channel")
	if !ok {
		t.Fatal("expected to find test_channel")
	}
	if got.Type() != "test_channel" {
		t.Fatalf("expected test_channel, got %s", got.Type())
	}
}

func TestRegistry_DuplicateRegistration(t *testing.T) {
	r := NewRegistry()
	_ = r.RegisterStatic(&mockPlugin{typeStr: "dup"})
	err := r.RegisterStatic(&mockPlugin{typeStr: "dup"})
	if !errors.Is(err, ErrAlreadyRegistered) {
		t.Fatalf("expected ErrAlreadyRegistered, got %v", err)
	}
}

func TestRegistry_GetNotFound(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()
	_ = r.RegisterStatic(&mockPlugin{typeStr: "beta"})
	_ = r.RegisterStatic(&mockPlugin{typeStr: "alpha"})
	list := r.List()
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
	if list[0].Type != "alpha" || list[1].Type != "beta" {
		t.Fatalf("expected sorted: %v", list)
	}
}

func TestParseChannels(t *testing.T) {
	raw := json.RawMessage(`{"channels":[{"type":"wecom_bot","config":{"webhook_url":"https://example.com"}}]}`)
	chs, err := ParseChannels(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(chs) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(chs))
	}
	if chs[0].Type != "wecom_bot" {
		t.Fatalf("expected wecom_bot, got %s", chs[0].Type)
	}
}

func TestParseChannels_Empty(t *testing.T) {
	chs, err := ParseChannels(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(chs) != 0 {
		t.Fatalf("expected 0, got %d", len(chs))
	}
}

func TestIsNewFormat(t *testing.T) {
	if !IsNewFormat(json.RawMessage(`{"channels":[{"type":"x"}]}`)) {
		t.Fatal("expected new format")
	}
	if IsNewFormat(json.RawMessage(`{"wecomBotIds":[1]}`)) {
		t.Fatal("expected old format")
	}
}

func TestResolve(t *testing.T) {
	r := NewRegistry()
	p := &mockPlugin{typeStr: "test"}
	_ = r.RegisterStatic(p)

	chs := []ChannelConfig{
		{Type: "test", Config: json.RawMessage(`{"key":"val"}`)},
	}
	resolved, err := Resolve(r, chs)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 1 {
		t.Fatalf("expected 1, got %d", len(resolved))
	}
	if resolved[0].Plugin.Type() != "test" {
		t.Fatal("wrong plugin")
	}
}

func TestResolve_UnknownType(t *testing.T) {
	r := NewRegistry()
	chs := []ChannelConfig{{Type: "nonexistent", Config: json.RawMessage(`{}`)}}
	_, err := Resolve(r, chs)
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestService_Send(t *testing.T) {
	r := NewRegistry()
	p := &mockPlugin{typeStr: "email"}
	_ = r.RegisterStatic(p)
	svc := NewService(r)

	msg := Message{Title: "test", Content: "hello"}
	err := svc.Send(context.Background(), "email", msg, json.RawMessage(`{"recipients":["a@b.com"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if p.lastMsg.Title != "test" {
		t.Fatalf("expected test, got %s", p.lastMsg.Title)
	}
}

func TestService_NotFound(t *testing.T) {
	svc := NewService(NewRegistry())
	err := svc.Send(context.Background(), "missing", Message{}, json.RawMessage(`{}`))
	if !errors.Is(err, ErrChannelNotFound) {
		t.Fatalf("expected ErrChannelNotFound, got %v", err)
	}
}
