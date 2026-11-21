//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSignatureAnswerAcceptsOnlyValidMicrosoft(t *testing.T) {
	ok := "Valid|CN=Microsoft Corporation, O=Microsoft Corporation, L=Redmond, S=Washington, C=US"
	if err := checkSignatureAnswer(ok); err != nil {
		t.Fatalf("valid Microsoft signature rejected: %v", err)
	}
	bad := []string{
		"NotSigned|",
		"HashMismatch|CN=Microsoft Corporation, O=Microsoft Corporation",
		"UnknownError|CN=Microsoft Corporation, O=Microsoft Corporation",
		"Valid|CN=Someone Else, O=Someone Else Ltd",
		"Valid|",
		"",
		"Valid|CN=Fake, O=Not Microsoft Corporation Inc",
	}
	for _, b := range bad {
		if err := checkSignatureAnswer(b); err == nil {
			t.Errorf("accepted %q", b)
		}
	}
}

// The real check, run on a file that is validly signed by Microsoft (a Windows system file),
// and on one that is not signed at all.
func TestVerifyMicrosoftSignatureOnRealFiles(t *testing.T) {
	sys := filepath.Join(os.Getenv("SystemRoot"), "System32", "notepad.exe")
	if err := verifyMicrosoftSignature(sys); err != nil {
		t.Fatalf("a Microsoft-signed system file was rejected: %v", err)
	}
	plain := filepath.Join(t.TempDir(), "it's unsigned.exe") // also covers a quote in the path
	if err := os.WriteFile(plain, []byte("MZ not a real program"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyMicrosoftSignature(plain); err == nil {
		t.Fatal("an unsigned file was accepted")
	}
}

// Downloads Microsoft's real bootstrapper (about 1.7 MB) and checks it (needs internet; skipped
// when offline). It does not run it.
func TestRealBootstrapperDownloadAndSignature(t *testing.T) {
	path := filepath.Join(t.TempDir(), "MicrosoftEdgeWebview2Setup.exe")
	if err := downloadFile(webView2BootstrapperURL, path, 60*time.Second); err != nil {
		t.Skipf("offline or blocked, skipped: %v", err)
	}
	if err := verifyMicrosoftSignature(path); err != nil {
		t.Fatalf("Microsoft's real bootstrapper failed the check: %v", err)
	}
}

func TestSignatureScriptQuotesThePath(t *testing.T) {
	s := signatureScript(`C:\a b\it's.exe`)
	if !strings.Contains(s, `-LiteralPath 'C:\a b\it''s.exe';`) {
		t.Fatalf("path not quoted safely: %s", s)
	}
}

func TestDownloadRefusesPlainHTTP(t *testing.T) {
	err := downloadFile("http://example.com/x.exe", filepath.Join(t.TempDir(), "x.exe"), 1)
	if err == nil || !strings.Contains(err.Error(), "non-https") {
		t.Fatalf("plain http must be refused, got %v", err)
	}
}

func TestBootstrapperLinkIsMicrosoftHTTPS(t *testing.T) {
	if !strings.HasPrefix(webView2BootstrapperURL, "https://go.microsoft.com/") {
		t.Fatalf("bootstrapper URL changed: %s", webView2BootstrapperURL)
	}
}

func TestWebView2VersionDoesNotPanic(t *testing.T) {
	v := webView2Version() // "" when not installed; this PC has it, so a dotted version is expected
	t.Logf("installed WebView2 Runtime: %q", v)
	if v != "" && !strings.Contains(v, ".") {
		t.Fatalf("unexpected version text %q", v)
	}
}
