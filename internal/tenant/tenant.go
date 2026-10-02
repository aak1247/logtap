// Package tenant carries the multi-tenant identity through request contexts
// and queued messages.
//
// The open-source edition is single-tenant: it never parses any HTTP header
// and every message is attributed to DefaultTenantID. Enterprise builds may
// populate the context from a trusted-proxy header before requests reach the
// storage layer; nothing outside the enterprise edition may set a
// non-default tenant.
package tenant

import "context"

// ID is the tenant identity (UUID, lowercase, no braces).
type ID string

// DefaultTenantID is the fixed tenant used by the open-source edition.
const DefaultTenantID ID = "00000000-0000-0000-0000-000000000001"

type ctxKey struct{}

// With stores id in ctx. An empty id falls back to DefaultTenantID so that
// storage code can always read a usable tenant from a context.
func With(ctx context.Context, id ID) context.Context {
	if id == "" {
		id = DefaultTenantID
	}
	return context.WithValue(ctx, ctxKey{}, id)
}

// From returns the tenant stored in ctx, if any.
func From(ctx context.Context) (ID, bool) {
	id, ok := ctx.Value(ctxKey{}).(ID)
	if !ok || id == "" {
		return "", false
	}
	return id, true
}

// FromOrDefault returns the tenant in ctx, falling back to DefaultTenantID.
func FromOrDefault(ctx context.Context) ID {
	if id, ok := From(ctx); ok {
		return id
	}
	return DefaultTenantID
}

// MustFrom returns the tenant in ctx and panics when absent. Use it only at
// strong-constraint call sites where the tenant must already have been set.
func MustFrom(ctx context.Context) ID {
	id, ok := From(ctx)
	if !ok {
		panic("tenant: no tenant in context")
	}
	return id
}
