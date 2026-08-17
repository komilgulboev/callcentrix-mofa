package handlers

import (
	"net/http/httptest"
	"testing"
)

func TestBuildPhoneWSURIUsesForwardedHeaders(t *testing.T) {
	req := httptest.NewRequest("GET", "http://internal-service/api/phone/config", nil)
	req.Host = "internal-service"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "call.example.com")

	got := buildPhoneWSURI(req)
	want := "wss://call.example.com/ws/phone"
	if got != want {
		t.Fatalf("buildPhoneWSURI() = %q, want %q", got, want)
	}
}
