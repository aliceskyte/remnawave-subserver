package handler

import "testing"

func TestSplitLegacyPathQuery(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantPath  string
		wantQuery string
		wantOK    bool
	}{
		{
			name:      "plain uuid with core suffix",
			path:      "/0cb6c0a7beed&core=mihomo",
			wantPath:  "/0cb6c0a7beed",
			wantQuery: "core=mihomo",
			wantOK:    true,
		},
		{
			name:      "prefix path with multiple params",
			path:      "/sub/0cb6c0a7beed&core=mihomo&foo=bar",
			wantPath:  "/sub/0cb6c0a7beed",
			wantQuery: "core=mihomo&foo=bar",
			wantOK:    true,
		},
		{
			name:      "no legacy query",
			path:      "/0cb6c0a7beed",
			wantPath:  "/0cb6c0a7beed",
			wantQuery: "",
			wantOK:    false,
		},
		{
			name:      "ampersand without equals is ignored",
			path:      "/0cb6c0a7beed&core",
			wantPath:  "/0cb6c0a7beed&core",
			wantQuery: "",
			wantOK:    false,
		},
		{
			name:      "slash after ampersand is ignored",
			path:      "/0cb6c0a7beed&core=mihomo/extra",
			wantPath:  "/0cb6c0a7beed&core=mihomo/extra",
			wantQuery: "",
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath, gotQuery, gotOK := splitLegacyPathQuery(tt.path)
			if gotPath != tt.wantPath || gotQuery != tt.wantQuery || gotOK != tt.wantOK {
				t.Fatalf("splitLegacyPathQuery(%q) = (%q, %q, %t), want (%q, %q, %t)", tt.path, gotPath, gotQuery, gotOK, tt.wantPath, tt.wantQuery, tt.wantOK)
			}
		})
	}
}
