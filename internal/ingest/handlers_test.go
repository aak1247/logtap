package ingest

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDecodeOneOrMany(t *testing.T) {
	t.Parallel()

	if _, err := decodeOneOrMany[CustomLogPayload]([]byte(" ")); err == nil {
		t.Fatalf("expected error for empty body")
	}
	if _, err := decodeOneOrMany[CustomLogPayload]([]byte("[]")); err == nil {
		t.Fatalf("expected error for empty array")
	}

	items, err := decodeOneOrMany[CustomLogPayload]([]byte(`[{"message":"a"},{"message":"b"}]`))
	if err != nil {
		t.Fatalf("decodeOneOrMany(array): %v", err)
	}
	if len(items) != 2 || items[0].Message != "a" || items[1].Message != "b" {
		t.Fatalf("unexpected items: %#v", items)
	}

	one, err := decodeOneOrMany[CustomLogPayload]([]byte(`{"message":"a"}`))
	if err != nil {
		t.Fatalf("decodeOneOrMany(object): %v", err)
	}
	if len(one) != 1 || one[0].Message != "a" {
		t.Fatalf("unexpected item: %#v", one)
	}

	if _, err := decodeOneOrMany[CustomLogPayload]([]byte(`{`)); err == nil {
		t.Fatalf("expected error for invalid json")
	}
}

func TestReadBody_GzipAndPlain(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	makeCtx := func(body []byte, gzipOn bool) *gin.Context {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		var rd *bytes.Reader
		var req *http.Request
		if !gzipOn {
			rd = bytes.NewReader(body)
			req = httptest.NewRequest(http.MethodPost, "/", rd)
			req.Header.Set("Content-Type", "application/json")
			c.Request = req
			return c
		}
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		if _, err := zw.Write(body); err != nil {
			t.Fatalf("gzip write: %v", err)
		}
		if err := zw.Close(); err != nil {
			t.Fatalf("gzip close: %v", err)
		}
		rd = bytes.NewReader(buf.Bytes())
		req = httptest.NewRequest(http.MethodPost, "/", rd)
		req.Header.Set("Content-Encoding", "gzip")
		c.Request = req
		return c
	}

	plain := []byte(`{"k":"v"}`)
	got, err := readBody(makeCtx(plain, false), 1024)
	if err != nil {
		t.Fatalf("readBody(plain): %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("expected %q, got %q", string(plain), string(got))
	}

	got2, err := readBody(makeCtx(plain, true), 1024)
	if err != nil {
		t.Fatalf("readBody(gzip): %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(got2), plain) {
		t.Fatalf("expected %q, got %q", string(plain), string(got2))
	}
}

