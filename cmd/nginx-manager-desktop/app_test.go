package main

import "testing"

func TestNormalizeEndpoint(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"https://example.com/", "https://example.com", false},
		{"http://127.0.0.1:8020/", "http://127.0.0.1:8020", false},
		{"http://localhost:8020", "http://localhost:8020", false},
		{"http://192.168.1.20:8020", "", true},
		{"http://example.com", "", true},
		{"ftp://example.com", "", true},
	}
	for _, tt := range tests {
		got, err := normalizeEndpoint(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("normalizeEndpoint(%q) succeeded: %q", tt.in, got)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("normalizeEndpoint(%q) = %q, %v; want %q, nil", tt.in, got, err, tt.want)
		}
	}
}
