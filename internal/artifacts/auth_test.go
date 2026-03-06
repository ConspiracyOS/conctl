package artifacts

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// setupTestArtifact creates a temp artifacts root with one artifact and returns
// the root path, the artifact, and a cleanup func.
func setupTestArtifact(t *testing.T) (string, string, *Artifact, []byte) {
	t.Helper()
	artifactsRoot := t.TempDir()
	statusRoot := t.TempDir()

	art, err := Create(CreateInput{
		ArtifactsRoot: artifactsRoot,
		StatusRoot:    statusRoot,
		Title:         "test artifact",
		Kind:          "file",
		ContentType:   "text/plain",
		Filename:      "hello.txt",
		Exposure:      ExposureDashboardLocal,
		Content:       []byte("hello world"),
	})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	secret := []byte("test-signing-secret-key")
	return artifactsRoot, statusRoot, art, secret
}

func TestAuthHandler_ValidSignature(t *testing.T) {
	artifactsRoot, _, art, secret := setupTestArtifact(t)

	now := time.Now().UTC()
	link, err := MintSignedLink("http://localhost", art, secret, time.Hour, now)
	if err != nil {
		t.Fatalf("mint link: %v", err)
	}

	handler := newAuthHandler(secret, artifactsRoot)

	// Extract path+query from the minted link URL.
	// Link format: http://localhost/artifacts/<id>/hello.txt?exp=...&sig=...
	uri := link.URL[len("http://localhost"):]

	req := httptest.NewRequest("GET", "http://localhost/auth/artifact", nil)
	req.Header.Set("X-Original-URI", uri)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuthHandler_ExpiredLink(t *testing.T) {
	artifactsRoot, _, art, secret := setupTestArtifact(t)

	// Mint a link that expired 1 hour ago.
	past := time.Now().UTC().Add(-2 * time.Hour)
	link, err := MintSignedLink("http://localhost", art, secret, time.Hour, past)
	if err != nil {
		t.Fatalf("mint link: %v", err)
	}

	handler := newAuthHandler(secret, artifactsRoot)
	uri := link.URL[len("http://localhost"):]

	req := httptest.NewRequest("GET", "http://localhost/auth/artifact", nil)
	req.Header.Set("X-Original-URI", uri)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for expired link, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuthHandler_MissingParams(t *testing.T) {
	artifactsRoot, _, art, secret := setupTestArtifact(t)
	_ = art

	handler := newAuthHandler(secret, artifactsRoot)

	// No exp or sig params.
	uri := fmt.Sprintf("/artifacts/%s/hello.txt", art.ID)

	req := httptest.NewRequest("GET", "http://localhost/auth/artifact", nil)
	req.Header.Set("X-Original-URI", uri)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing params, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuthHandler_TamperedSignature(t *testing.T) {
	artifactsRoot, _, art, secret := setupTestArtifact(t)

	now := time.Now().UTC()
	link, err := MintSignedLink("http://localhost", art, secret, time.Hour, now)
	if err != nil {
		t.Fatalf("mint link: %v", err)
	}

	handler := newAuthHandler(secret, artifactsRoot)
	uri := link.URL[len("http://localhost"):]

	// Tamper with the signature: replace last char.
	lastChar := uri[len(uri)-1]
	var replacement byte
	if lastChar == 'a' {
		replacement = 'b'
	} else {
		replacement = 'a'
	}
	uri = uri[:len(uri)-1] + string(replacement)

	req := httptest.NewRequest("GET", "http://localhost/auth/artifact", nil)
	req.Header.Set("X-Original-URI", uri)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for tampered sig, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuthHandler_FallbackURI(t *testing.T) {
	artifactsRoot, _, art, secret := setupTestArtifact(t)

	now := time.Now().UTC()
	link, err := MintSignedLink("http://localhost", art, secret, time.Hour, now)
	if err != nil {
		t.Fatalf("mint link: %v", err)
	}

	handler := newAuthHandler(secret, artifactsRoot)
	// Use the artifact path+query as the actual request URI (no X-Original-URI header).
	uri := link.URL[len("http://localhost"):]

	req := httptest.NewRequest("GET", "http://localhost"+uri, nil)
	// No X-Original-URI header set.
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with fallback URI, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuthHandler_BadArtifactPath(t *testing.T) {
	secret := []byte("test-secret")
	handler := newAuthHandler(secret, t.TempDir())

	req := httptest.NewRequest("GET", "http://localhost/auth/artifact", nil)
	req.Header.Set("X-Original-URI", "/not-artifacts/something?exp=123&sig=abc")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad path, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExtractArtifactID(t *testing.T) {
	tests := []struct {
		uri    string
		wantID string
		wantOK bool
	}{
		{"/artifacts/art_abc123/file.txt?exp=1&sig=2", "art_abc123", true},
		{"/artifacts/art_abc123/file.txt", "art_abc123", true},
		{"/artifacts/art_abc123/", "art_abc123", true},
		{"/not-artifacts/art_abc123/file.txt", "", false},
		{"/artifacts/", "", false},
		{"/artifacts", "", false},
	}
	for _, tt := range tests {
		id, err := extractArtifactID(tt.uri)
		if tt.wantOK {
			if err != nil {
				t.Errorf("extractArtifactID(%q) unexpected error: %v", tt.uri, err)
			}
			if id != tt.wantID {
				t.Errorf("extractArtifactID(%q) = %q, want %q", tt.uri, id, tt.wantID)
			}
		} else {
			if err == nil {
				t.Errorf("extractArtifactID(%q) expected error, got id=%q", tt.uri, id)
			}
		}
	}
}

func TestExtractQueryParams(t *testing.T) {
	exp, sig := extractQueryParams("/artifacts/art_123/f.txt?exp=12345&sig=abcdef")
	if exp != "12345" || sig != "abcdef" {
		t.Errorf("got exp=%q sig=%q, want 12345/abcdef", exp, sig)
	}

	exp, sig = extractQueryParams("/artifacts/art_123/f.txt")
	if exp != "" || sig != "" {
		t.Errorf("expected empty for no query, got exp=%q sig=%q", exp, sig)
	}
}

func TestListenAndServeAuth_EmptySecret(t *testing.T) {
	secretFile := filepath.Join(t.TempDir(), "empty.key")
	os.WriteFile(secretFile, []byte("  \n"), 0600)

	err := ListenAndServeAuth(AuthConfig{
		SocketPath:    filepath.Join(t.TempDir(), "test.sock"),
		SecretFile:    secretFile,
		ArtifactsRoot: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error for empty secret")
	}
}
