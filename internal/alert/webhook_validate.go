package alert

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

type WebhookValidationOptions struct {
	AllowLoopback   bool
	AllowPrivateIPs bool
	AllowlistCIDRs  []netip.Prefix
}

func ValidateWebhookURL(ctx context.Context, raw string, opts WebhookValidationOptions) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return errors.New("webhook url empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid webhook url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("invalid webhook url scheme (expected http|https)")
	}
	host := strings.TrimSpace(u.Host)
	if host == "" {
		return errors.New("invalid webhook url host")
	}
	hostname := u.Hostname()
	if strings.EqualFold(strings.TrimSpace(hostname), "localhost") && !opts.AllowLoopback {
		return errors.New("webhook url blocked (localhost not allowed)")
	}

	ip := net.ParseIP(hostname)
	if ip != nil {
		return validateIP(ip, opts)
	}

	lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	ips, err := net.DefaultResolver.LookupIP(lookupCtx, "ip", hostname)
	if err != nil {
		return fmt.Errorf("resolve webhook host: %w", err)
	}
	if len(ips) == 0 {
		return errors.New("resolve webhook host: no ip")
	}
	for _, ip := range ips {
		if err := validateIP(ip, opts); err != nil {
			return err
		}
	}
	return nil
}

func validateIP(ip net.IP, opts WebhookValidationOptions) error {
	if ip == nil {
		return errors.New("webhook url blocked (invalid ip)")
	}
	ip = ip.To16()
	if ip == nil {
		return errors.New("webhook url blocked (invalid ip)")
	}

	// Explicitly block cloud metadata IP.
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 169 && ip4[1] == 254 && ip4[2] == 169 && ip4[3] == 254 {
		return errors.New("webhook url blocked (metadata ip not allowed)")
	}

	// Allowlist overrides private/loopback restrictions (but not metadata).
	if isAllowedByCIDRAllowlist(ip, opts.AllowlistCIDRs) {
		return nil
	}

	// Block common non-routable targets.
	if ip.IsUnspecified() || ip.IsMulticast() {
		return errors.New("webhook url blocked (non-routable ip)")
	}
	if ip.IsLoopback() && !opts.AllowLoopback {
		return errors.New("webhook url blocked (loopback not allowed)")
	}
	if ip.IsPrivate() && !opts.AllowPrivateIPs {
		return errors.New("webhook url blocked (private ip not allowed)")
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return errors.New("webhook url blocked (link-local ip not allowed)")
	}
	return nil
}

func isAllowedByCIDRAllowlist(ip net.IP, allowlist []netip.Prefix) bool {
	if len(allowlist) == 0 || ip == nil {
		return false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	for _, p := range allowlist {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}
