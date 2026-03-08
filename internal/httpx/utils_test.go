package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsStatusCode(t *testing.T) {
	tests := []struct {
		name   string
		value  any
		target int
		want   bool
	}{
		{"int match", 200, 200, true},
		{"int no match", 200, 404, false},
		{"int64 match", int64(401), 401, true},
		{"float64 match", float64(500), 500, true},
		{"json.Number match", json.Number("403"), 403, true},
		{"json.Number no match", json.Number("200"), 404, false},
		{"string match", "404", 404, true},
		{"string no match", "200", 404, false},
		{"string invalid", "abc", 200, false},
		{"nil", nil, 200, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsStatusCode(tt.value, tt.target)
			if got != tt.want {
				t.Errorf("IsStatusCode(%v, %d) = %v, want %v", tt.value, tt.target, got, tt.want)
			}
		})
	}
}

func TestClientIP(t *testing.T) {
	t.Cleanup(func() {
		if err := SetTrustedProxyCIDRs(nil); err != nil {
			t.Fatalf("reset trusted proxies: %v", err)
		}
	})
	tests := []struct {
		name       string
		trusted    []string
		forwarded  string
		realIP     string
		remoteAddr string
		want       string
	}{
		{"Default trusts loopback", nil, "1.2.3.4", "", "127.0.0.1:1234", "1.2.3.4"},
		{"Configured proxy uses X-Forwarded-For", []string{"172.18.0.0/16"}, "1.2.3.4", "", "172.18.0.9:1234", "1.2.3.4"},
		{"Configured proxy uses first forwarded IP", []string{"127.0.0.1/32"}, "1.2.3.4, 10.0.0.1", "", "127.0.0.1:1234", "1.2.3.4"},
		{"Configured proxy uses X-Real-IP", []string{"10.0.0.0/8"}, "", "9.8.7.6", "10.0.0.2:1234", "9.8.7.6"},
		{"Private client cannot spoof without explicit trust", nil, "1.2.3.4", "", "172.18.0.9:1234", "172.18.0.9"},
		{"Public client cannot spoof X-Forwarded-For", nil, "1.2.3.4", "", "5.6.7.8:1234", "5.6.7.8"},
		{"Public client cannot spoof X-Real-IP", nil, "", "9.8.7.6", "5.6.7.8:1234", "5.6.7.8"},
		{"RemoteAddr with port", nil, "", "", "5.6.7.8:1234", "5.6.7.8"},
		{"RemoteAddr without port", nil, "", "", "5.6.7.8", "5.6.7.8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := SetTrustedProxyCIDRs(tt.trusted); err != nil {
				t.Fatalf("SetTrustedProxyCIDRs() error = %v", err)
			}
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.forwarded != "" {
				r.Header.Set("X-Forwarded-For", tt.forwarded)
			}
			if tt.realIP != "" {
				r.Header.Set("X-Real-IP", tt.realIP)
			}
			r.RemoteAddr = tt.remoteAddr

			got := ClientIP(r)
			if got != tt.want {
				t.Errorf("ClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetTrustedProxyCIDRsRejectsInvalidValue(t *testing.T) {
	if err := SetTrustedProxyCIDRs([]string{"not-a-cidr"}); err == nil {
		t.Fatal("expected error for invalid CIDR")
	}
}

func TestResponseRecorder(t *testing.T) {
	w := httptest.NewRecorder()
	rec := NewResponseRecorder(w, http.StatusOK)

	if rec.Status != http.StatusOK {
		t.Errorf("initial Status = %d, want %d", rec.Status, http.StatusOK)
	}

	rec.WriteHeader(http.StatusNotFound)
	if rec.Status != http.StatusNotFound {
		t.Errorf("Status after WriteHeader = %d, want %d", rec.Status, http.StatusNotFound)
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("underlying recorder code = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestResponseRecorderDoubleWriteHeader(t *testing.T) {
	w := httptest.NewRecorder()
	rec := NewResponseRecorder(w, http.StatusOK)

	rec.WriteHeader(http.StatusNotFound)
	rec.WriteHeader(http.StatusInternalServerError) // should be ignored

	if rec.Status != http.StatusNotFound {
		t.Errorf("Status = %d, want %d (second WriteHeader should be ignored)", rec.Status, http.StatusNotFound)
	}
}
