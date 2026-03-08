package httpx

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

var (
	trustedProxyMu   sync.RWMutex
	trustedProxyNets = mustParseCIDRs("127.0.0.0/8", "::1/128")
)

type ResponseRecorder struct {
	http.ResponseWriter
	Status        int
	headerWritten bool
}

func NewResponseRecorder(w http.ResponseWriter, status int) *ResponseRecorder {
	return &ResponseRecorder{ResponseWriter: w, Status: status}
}

func (r *ResponseRecorder) WriteHeader(status int) {
	if r.headerWritten {
		return
	}
	r.headerWritten = true
	r.Status = status
	r.ResponseWriter.WriteHeader(status)
}

func IsStatusCode(value any, target int) bool {
	switch v := value.(type) {
	case int:
		return v == target
	case int64:
		return v == int64(target)
	case float64:
		return int(v) == target
	case json.Number:
		if parsed, err := v.Int64(); err == nil {
			return parsed == int64(target)
		}
		if parsed, err := v.Float64(); err == nil {
			return int(parsed) == target
		}
	case string:
		if parsed, err := strconv.Atoi(v); err == nil {
			return parsed == target
		}
	}
	return false
}

func ClientIP(r *http.Request) string {
	remoteIP := remoteAddrHost(r.RemoteAddr)
	if isTrustedProxy(remoteIP) {
		if forwarded := forwardedIP(r.Header.Get("X-Forwarded-For")); forwarded != "" {
			return forwarded
		}
		if realIP := cleanIPAddress(r.Header.Get("X-Real-IP")); realIP != "" {
			return realIP
		}
	}
	if remoteIP != "" {
		return remoteIP
	}
	if forwarded := forwardedIP(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		return forwarded
	}
	if realIP := cleanIPAddress(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	return r.RemoteAddr
}

func remoteAddrHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(remoteAddr)
}

func forwardedIP(value string) string {
	for _, part := range strings.Split(value, ",") {
		if ip := cleanIPAddress(part); ip != "" {
			return ip
		}
	}
	return ""
}

func cleanIPAddress(value string) string {
	candidate := strings.TrimSpace(value)
	if candidate == "" {
		return ""
	}
	if ip := net.ParseIP(candidate); ip != nil {
		return ip.String()
	}
	return ""
}

func isTrustedProxy(value string) bool {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil {
		return false
	}
	trustedProxyMu.RLock()
	nets := append([]*net.IPNet(nil), trustedProxyNets...)
	trustedProxyMu.RUnlock()
	for _, network := range nets {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func SetTrustedProxyCIDRs(cidrs []string) error {
	parsed, err := parseCIDRs(cidrs)
	if err != nil {
		return err
	}
	if len(parsed) == 0 {
		parsed = mustParseCIDRs("127.0.0.0/8", "::1/128")
	}
	trustedProxyMu.Lock()
	trustedProxyNets = parsed
	trustedProxyMu.Unlock()
	return nil
}

func parseCIDRs(values []string) ([]*net.IPNet, error) {
	nets := make([]*net.IPNet, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if !strings.Contains(value, "/") {
			ip := net.ParseIP(value)
			if ip == nil {
				return nil, fmt.Errorf("invalid CIDR or IP %q", value)
			}
			maskBits := 32
			if ip.To4() == nil {
				maskBits = 128
			}
			value = fmt.Sprintf("%s/%d", ip.String(), maskBits)
		}
		_, network, err := net.ParseCIDR(value)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", value, err)
		}
		nets = append(nets, network)
	}
	return nets, nil
}

func mustParseCIDRs(values ...string) []*net.IPNet {
	nets, err := parseCIDRs(values)
	if err != nil {
		panic(err)
	}
	return nets
}
