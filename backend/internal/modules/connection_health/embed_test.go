package connection_health

import "testing"

func TestNormalizeEmbedInterval(t *testing.T) {
	if got := normalizeEmbedInterval(0); got != defaultEmbedRefreshInterval {
		t.Fatalf("default interval = %d", got)
	}
	if got := normalizeEmbedInterval(minEmbedRefreshInterval - 1); got != defaultEmbedRefreshInterval {
		t.Fatalf("short interval = %d", got)
	}
	if got := normalizeEmbedInterval(maxEmbedRefreshInterval + 1); got != maxEmbedRefreshInterval {
		t.Fatalf("long interval = %d", got)
	}
}

func TestNormalizeAllowedOrigin(t *testing.T) {
	origin, err := normalizeAllowedOrigin("HTTPS://Example.com")
	if err != nil || origin != "https://example.com" {
		t.Fatalf("origin = %q, err = %v", origin, err)
	}
	if _, err := normalizeAllowedOrigin("https://example.com/path"); err == nil {
		t.Fatal("path origin should be rejected")
	}
}

func TestNormalizeEmbedPreviewHTML(t *testing.T) {
	got := normalizeEmbedPreviewHTML("```html\n<!doctype html><html><body>ok</body></html>\n```")
	if got != "<!doctype html><html><body>ok</body></html>" {
		t.Fatalf("unexpected normalized preview: %q", got)
	}
	if got := normalizeEmbedPreviewHTML("   "); got != "" {
		t.Fatalf("blank preview = %q", got)
	}
}
