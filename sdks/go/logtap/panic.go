package logtap

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"
)

// Recover is a helper for capturing panics as a fatal log.
//
// Usage:
//
//	defer client.Recover(true) // report then re-panic
func (c *Client) Recover(repanic bool) {
	r := recover()
	if r == nil {
		return
	}

	c.Fatal("panic", map[string]any{
		"kind":  "panic",
		"panic": fmt.Sprint(r),
		"stack": string(debug.Stack()),
	}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	_ = c.Flush(ctx)
	cancel()

	if repanic {
		panic(r)
	}
}
