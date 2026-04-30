package builtin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aak1247/logtap/internal/channel"
)

func TestWecomBot_Send(t *testing.T) {
	var gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]any
		json.NewDecoder(r.Body).Decode(&m)
		gotBody = m["msgtype"].(string)
		w.WriteHeader(200)
	}))
	defer ts.Close()

	p := &WecomBotPlugin{}
	cfg := json.RawMessage(`{"webhook_url":"` + ts.URL + `"}`)
	msg := channel.Message{Title: "alert", Content: "fired"}
	if err := p.Send(context.Background(), msg, cfg); err != nil {
		t.Fatal(err)
	}
	if gotBody != "text" {
		t.Fatalf("expected text, got %s", gotBody)
	}
}

func TestWebhook_Send(t *testing.T) {
	var gotTitle string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]any
		json.NewDecoder(r.Body).Decode(&m)
		gotTitle = m["title"].(string)
		w.WriteHeader(200)
	}))
	defer ts.Close()

	p := &WebhookPlugin{}
	cfg := json.RawMessage(`{"url":"` + ts.URL + `"}`)
	msg := channel.Message{Title: "hello", Content: "world"}
	if err := p.Send(context.Background(), msg, cfg); err != nil {
		t.Fatal(err)
	}
	if gotTitle != "hello" {
		t.Fatalf("expected hello, got %s", gotTitle)
	}
}

func TestFeishu_Send(t *testing.T) {
	var gotMsgType string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]any
		json.NewDecoder(r.Body).Decode(&m)
		gotMsgType = m["msg_type"].(string)
		w.Write([]byte(`{"code":0,"msg":"ok"}`))
	}))
	defer ts.Close()

	p := &FeishuPlugin{}
	cfg := json.RawMessage(`{"webhook_url":"` + ts.URL + `"}`)
	msg := channel.Message{Title: "test", Content: "body"}
	if err := p.Send(context.Background(), msg, cfg); err != nil {
		t.Fatal(err)
	}
	if gotMsgType != "text" {
		t.Fatalf("expected text, got %s", gotMsgType)
	}
}

func TestEmail_ValidateConfig(t *testing.T) {
	p := &EmailPlugin{}
	if err := p.ValidateConfig(json.RawMessage(`{"recipients":[]}`)); err == nil {
		t.Fatal("expected error for empty recipients")
	}
	if err := p.ValidateConfig(json.RawMessage(`{"recipients":["a@b.com"]}`)); err != nil {
		t.Fatal(err)
	}
}

func TestWecomBot_ValidateConfig_EmptyURL(t *testing.T) {
	p := &WecomBotPlugin{}
	if err := p.ValidateConfig(json.RawMessage(`{"webhook_url":""}`)); err == nil {
		t.Fatal("expected error for empty webhook_url")
	}
}

func TestWebhook_ValidateConfig_EmptyURL(t *testing.T) {
	p := &WebhookPlugin{}
	if err := p.ValidateConfig(json.RawMessage(`{"url":""}`)); err == nil {
		t.Fatal("expected error for empty url")
	}
}

func TestFeishu_ValidateConfig_EmptyURL(t *testing.T) {
	p := &FeishuPlugin{}
	if err := p.ValidateConfig(json.RawMessage(`{"webhook_url":""}`)); err == nil {
		t.Fatal("expected error for empty webhook_url")
	}
}
