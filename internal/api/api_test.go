package api

import (
	"testing"
)

func TestStripExt(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"my-vpn.conf", "my-vpn"},
		{"config.wg.conf", "config.wg"},
		{"noext", "noext"},
		{"/path/to/file.conf", "/path/to/file"},
		{".hidden", ""},
		{"conf", "conf"},
	}

	for _, tt := range tests {
		result := stripExt(tt.input)
		if result != tt.expected {
			t.Errorf("stripExt(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestIsMultipart(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"multipart/form-data; boundary=123", true},
		{"multipart/form-data", true},
		{"application/json", false},
		{"text/plain", false},
		{"", false},
		{"multipart/mixed", false},
	}

	for _, tt := range tests {
		result := isMultipart(tt.input)
		if result != tt.expected {
			t.Errorf("isMultipart(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestImportRequest(t *testing.T) {
	// Verify importRequest struct works as expected
	req := importRequest{
		ConfigText: "[Interface]\nPrivateKey = test\n\n[Peer]\nPublicKey = peer",
		Name:       "test-vpn",
	}

	if req.ConfigText == "" {
		t.Error("ConfigText should not be empty")
	}
	if req.Name != "test-vpn" {
		t.Errorf("expected Name 'test-vpn', got %q", req.Name)
	}
}
