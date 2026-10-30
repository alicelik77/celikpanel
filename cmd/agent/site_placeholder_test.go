package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The alpha.112-and-earlier page is on installed sites; its bytes are pinned
// by the digests of the original function (computed from main at alpha.112).
// Eski sayfa kurulu sitelerde duruyor; baytları özgün işlevin özetiyle sabit.
func TestSitePlaceholderV1BytesNeverChange(t *testing.T) {
	for projectType, want := range map[string]string{
		"static": "a732fc90c2321c89ee9c928a8e50c9c3e1cf4f81a99208d13e2335486440b7e8",
		"php":    "53636d7bb43c0c3b4a29771cdcd979d6f674ede3db8e0d68cd25b91d7b4e3f6c",
	} {
		sum := sha256.Sum256(sitePlaceholderV1("wordpress.example", projectType))
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Fatalf("%s v1 placeholder digest = %s, want %s", projectType, got, want)
		}
	}
}

func TestSitePlaceholderV2(t *testing.T) {
	name, page := celikPanelSitePlaceholder("shop.example.com", "static")
	text := string(page)
	if name != "index.html" || !strings.Contains(text, "<h1>shop.example.com</h1>") ||
		!strings.Contains(text, "<code>index.html</code>") || strings.Contains(text, "<?php") {
		t.Fatalf("static page is not the v2 page:\n%s", text)
	}
	// Only PHP sites have the panel's Applications section.
	if strings.Contains(text, "Uygulamalar") || strings.Contains(text, "Applications") {
		t.Fatal("static page sends the owner to an Applications section it does not have")
	}
	for _, leak := range []string{"PHP_VERSION", "phpinfo", "/var/www", "http://", "https://", "<script", "<link"} {
		if strings.Contains(text, leak) {
			t.Fatalf("static page contains %q", leak)
		}
	}
	name, page = celikPanelSitePlaceholder("shop.example.com", "php")
	text = string(page)
	if name != "index.php" || !strings.Contains(text, "<code>index.php</code>") ||
		!strings.Contains(text, "<b>Uygulamalar</b>") || !strings.Contains(text, "<b>Applications</b>") {
		t.Fatalf("php page is not the v2 page:\n%s", text)
	}
	// Visitors learn nothing about the server's software, not even that PHP runs.
	for _, leak := range []string{"<?php", "PHP", "<script", "<link", "http://", "https://"} {
		if strings.Contains(text, leak) {
			t.Fatalf("php page contains %q", leak)
		}
	}
	_, page = celikPanelSitePlaceholder(`a<b>.example`, "static")
	if strings.Contains(string(page), "<b>.example") {
		t.Fatal("domain is not HTML-escaped")
	}
}

// A site created by alpha.112 keeps its old page; the one-click installer must
// still recognise it as CelikPanel's own, and still refuse anything else.
func TestInspectInstallableDocumentRootAcceptsEarlierPlaceholder(t *testing.T) {
	const domain = "wordpress.example"
	for _, content := range [][]byte{sitePlaceholderV1(domain, "php"), sitePlaceholderV2(domain, "php")} {
		documentRoot := t.TempDir()
		if err := os.WriteFile(filepath.Join(documentRoot, "index.php"), content, 0o640); err != nil {
			t.Fatal(err)
		}
		if _, err := inspectInstallableDocumentRoot(documentRoot, domain); err != nil {
			t.Fatalf("CelikPanel placeholder was rejected: %v", err)
		}
	}
	documentRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(documentRoot, "index.php"), sitePlaceholderV1("other.example", "php"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := inspectInstallableDocumentRoot(documentRoot, domain); err == nil {
		t.Fatal("another domain's placeholder was accepted")
	}
}
