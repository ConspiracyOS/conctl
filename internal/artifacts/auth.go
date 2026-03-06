package artifacts

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// DefaultAuthSocket is the default Unix socket path for the auth_request backend.
const DefaultAuthSocket = "/run/conos/artifact-auth.sock"

// AuthConfig holds configuration for the artifact auth server.
type AuthConfig struct {
	SocketPath    string
	SecretFile    string
	ArtifactsRoot string
}

// ListenAndServeAuth starts the auth_request backend on a Unix socket.
// For each request it extracts the artifact ID from the original URI path
// (/artifacts/<id>/...), reads exp and sig from query params, loads the
// artifact manifest, and verifies the signature.
// Returns 200 on success, 403 on invalid/expired, 401 if params missing.
func ListenAndServeAuth(cfg AuthConfig) error {
	secret, err := os.ReadFile(cfg.SecretFile)
	if err != nil {
		return fmt.Errorf("reading secret file: %w", err)
	}
	secret = trimSpace(secret)
	if len(secret) == 0 {
		return fmt.Errorf("secret file is empty: %s", cfg.SecretFile)
	}

	handler := newAuthHandler(secret, cfg.ArtifactsRoot)

	// Remove stale socket if present.
	os.Remove(cfg.SocketPath)

	listener, err := net.Listen("unix", cfg.SocketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.SocketPath, err)
	}
	// Allow nginx worker (typically www-data) to connect.
	if err := os.Chmod(cfg.SocketPath, 0666); err != nil {
		listener.Close()
		return fmt.Errorf("chmod socket: %w", err)
	}

	log.Printf("artifact-auth: listening on %s", cfg.SocketPath)
	return http.Serve(listener, handler)
}

func newAuthHandler(secret []byte, artifactsRoot string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// nginx sends the original client URI in X-Original-URI.
		uri := r.Header.Get("X-Original-URI")
		if uri == "" {
			// Fallback: use the request URI itself (for testing).
			uri = r.URL.RequestURI()
		}

		artifactID, err := extractArtifactID(uri)
		if err != nil {
			http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Parse query params from the original URI.
		exp, sig := extractQueryParams(uri)
		if exp == "" || sig == "" {
			http.Error(w, "missing exp or sig", http.StatusUnauthorized)
			return
		}

		artifact, err := Show(artifactsRoot, artifactID)
		if err != nil {
			http.Error(w, "artifact not found", http.StatusForbidden)
			return
		}

		if err := VerifySignedLink(artifact, secret, exp, sig, time.Now().UTC()); err != nil {
			http.Error(w, "forbidden: "+err.Error(), http.StatusForbidden)
			return
		}

		w.WriteHeader(http.StatusOK)
	})
}

// extractArtifactID parses an artifact ID from a URI path like /artifacts/<id>/filename.
func extractArtifactID(uri string) (string, error) {
	// Strip query string.
	path := uri
	if idx := strings.IndexByte(path, '?'); idx >= 0 {
		path = path[:idx]
	}
	path = strings.TrimPrefix(path, "/")
	parts := strings.SplitN(path, "/", 3) // ["artifacts", "<id>", "filename"]
	if len(parts) < 2 || parts[0] != "artifacts" || parts[1] == "" {
		return "", fmt.Errorf("cannot extract artifact ID from path: %s", uri)
	}
	return parts[1], nil
}

// extractQueryParams parses exp and sig from a raw URI string.
func extractQueryParams(uri string) (exp, sig string) {
	idx := strings.IndexByte(uri, '?')
	if idx < 0 {
		return "", ""
	}
	query := uri[idx+1:]
	for _, pair := range strings.Split(query, "&") {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "exp":
			exp = kv[1]
		case "sig":
			sig = kv[1]
		}
	}
	return exp, sig
}

func trimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}
