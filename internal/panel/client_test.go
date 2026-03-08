package panel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUserByShortUUIDEscapesPathSegment(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		writePanelResponse(t, w, map[string]any{
			"vlessUuid": "uuid",
			"username":  "user",
			"uuid":      "panel-id",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", time.Second, nil)
	_, err := client.UserByShortUUID(context.Background(), "../weird/value")
	if err != nil {
		t.Fatalf("UserByShortUUID() error = %v", err)
	}
	want := "/api/users/by-short-uuid/..%2Fweird%2Fvalue"
	if gotPath != want {
		t.Fatalf("path = %q, want %q", gotPath, want)
	}
}

func TestClientRejectsRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/elsewhere", http.StatusFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", time.Second, nil)
	_, err := client.SubscriptionByShortUUID(context.Background(), "abc")
	if err == nil {
		t.Fatal("expected redirect error")
	}
	code, ok := Code(err)
	if !ok || code != http.StatusFound {
		t.Fatalf("Code(err) = (%v, %t), want (%d, true)", code, ok, http.StatusFound)
	}
}

func writePanelResponse(t *testing.T, w http.ResponseWriter, response map[string]any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"response": response}); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
