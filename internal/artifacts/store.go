package artifacts

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Exposure string

const (
	ExposurePrivate                Exposure = "private"
	ExposureDashboardLocal         Exposure = "dashboard_local"
	ExposureAuthenticatedDashboard Exposure = "authenticated_dashboard"
)

type Artifact struct {
	ID          string       `json:"artifact_id"`
	CreatedAt   time.Time    `json:"created_at"`
	CreatedBy   string       `json:"created_by,omitempty"`
	RunID       string       `json:"run_id,omitempty"`
	TaskID      string       `json:"task_id,omitempty"`
	Title       string       `json:"title"`
	Kind        string       `json:"kind"`
	ContentType string       `json:"content_type"`
	Filename    string       `json:"filename"`
	Path        string       `json:"path"`
	LinkPath    string       `json:"link_path,omitempty"`
	Audience    string       `json:"audience"`
	Exposure    Exposure     `json:"exposure"`
	Size        int64        `json:"size_bytes"`
	SHA256      string       `json:"sha256"`
	ExpiresAt   time.Time    `json:"expires_at,omitempty"`
	SignedLinks []SignedLink `json:"signed_links,omitempty"`
}

type SignedLink struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

type CreateInput struct {
	ArtifactsRoot string
	StatusRoot    string
	Title         string
	Kind          string
	ContentType   string
	Filename      string
	CreatedBy     string
	RunID         string
	TaskID        string
	Audience      string
	Exposure      Exposure
	ExpiresAt     time.Time
	Content       []byte
}

var invalidFilename = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func Create(in CreateInput) (*Artifact, error) {
	if in.ArtifactsRoot == "" {
		return nil, fmt.Errorf("artifacts root is required")
	}
	if in.StatusRoot == "" {
		return nil, fmt.Errorf("status root is required")
	}
	if len(in.Content) == 0 {
		return nil, fmt.Errorf("content is required")
	}

	id, err := newArtifactID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	filename := sanitizeFilename(in.Filename)
	if filename == "" {
		filename = defaultFilename(in.ContentType, in.Kind)
	}
	if in.ContentType == "" {
		in.ContentType = defaultContentType(filename)
	}
	if in.Kind == "" {
		in.Kind = "file"
	}
	if in.Audience == "" {
		in.Audience = "user"
	}
	if in.Exposure == "" {
		in.Exposure = ExposureDashboardLocal
	}

	relDir := filepath.Join(now.Format("2006"), now.Format("01"), id)
	artifactDir := filepath.Join(in.ArtifactsRoot, relDir)
	if err := os.MkdirAll(artifactDir, 0755); err != nil {
		return nil, err
	}
	filePath := filepath.Join(artifactDir, filename)
	if err := os.WriteFile(filePath, in.Content, 0644); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(in.Content)

	artifact := &Artifact{
		ID:          id,
		CreatedAt:   now,
		CreatedBy:   in.CreatedBy,
		RunID:       in.RunID,
		TaskID:      in.TaskID,
		Title:       firstNonEmpty(in.Title, strings.TrimSuffix(filename, filepath.Ext(filename))),
		Kind:        in.Kind,
		ContentType: in.ContentType,
		Filename:    filename,
		Path:        filePath,
		Audience:    in.Audience,
		Exposure:    in.Exposure,
		Size:        int64(len(in.Content)),
		SHA256:      hex.EncodeToString(sum[:]),
		ExpiresAt:   in.ExpiresAt,
	}
	if in.Exposure != ExposurePrivate {
		artifact.LinkPath = filepath.ToSlash(filepath.Join("/artifacts", artifact.ID, filename))
		if err := publishStatusView(in.StatusRoot, artifact, in.Content); err != nil {
			return nil, err
		}
	}

	manifestPath := filepath.Join(artifactDir, "artifact.json")
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		return nil, err
	}
	return artifact, nil
}

func Show(artifactsRoot, id string) (*Artifact, error) {
	matches, err := filepath.Glob(filepath.Join(artifactsRoot, "*", "*", id, "artifact.json"))
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("artifact %q not found", id)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		return nil, err
	}
	var artifact Artifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return nil, err
	}
	return &artifact, nil
}

func Save(artifact *Artifact) error {
	if artifact == nil {
		return fmt.Errorf("artifact is required")
	}
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(filepath.Dir(artifact.Path), "artifact.json"), data, 0644)
}

func MintSignedLink(baseURL string, artifact *Artifact, secret []byte, ttl time.Duration, now time.Time) (SignedLink, error) {
	if artifact == nil {
		return SignedLink{}, fmt.Errorf("artifact is required")
	}
	if len(secret) == 0 {
		return SignedLink{}, fmt.Errorf("secret is required")
	}
	if ttl <= 0 {
		return SignedLink{}, fmt.Errorf("ttl must be positive")
	}
	expiresAt := now.UTC().Add(ttl)
	exp := fmt.Sprintf("%d", expiresAt.Unix())
	payload := strings.Join([]string{artifact.ID, artifact.Filename, exp}, ":")
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	base := strings.TrimRight(baseURL, "/")
	linkPath := firstNonEmpty(artifact.LinkPath, filepath.ToSlash(filepath.Join("/artifacts", artifact.ID, artifact.Filename)))
	url := fmt.Sprintf("%s%s?exp=%s&sig=%s", base, linkPath, exp, sig)
	link := SignedLink{URL: url, ExpiresAt: expiresAt}
	artifact.SignedLinks = append(artifact.SignedLinks, link)
	return link, nil
}

func VerifySignedLink(artifact *Artifact, secret []byte, exp, sig string, now time.Time) error {
	if artifact == nil {
		return fmt.Errorf("artifact is required")
	}
	if len(secret) == 0 {
		return fmt.Errorf("secret is required")
	}
	expiry, err := time.Parse(time.RFC3339, exp)
	if err != nil {
		if unix, parseErr := parseUnixTimestamp(exp); parseErr == nil {
			expiry = unix
		} else {
			return fmt.Errorf("invalid expiry: %w", err)
		}
	}
	if now.UTC().After(expiry) {
		return fmt.Errorf("link expired at %s", expiry.Format(time.RFC3339))
	}
	payload := strings.Join([]string{artifact.ID, artifact.Filename, fmt.Sprintf("%d", expiry.Unix())}, ":")
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	want := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(strings.ToLower(sig)), []byte(want)) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

func publishStatusView(statusRoot string, artifact *Artifact, content []byte) error {
	webDir := filepath.Join(statusRoot, "artifacts", artifact.ID)
	if err := os.MkdirAll(webDir, 0755); err != nil {
		return err
	}
	webFile := filepath.Join(webDir, artifact.Filename)
	if err := os.WriteFile(webFile, content, 0644); err != nil {
		return err
	}
	indexHTML := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>%s</title></head>
<body>
<h1>%s</h1>
<p>Artifact ID: <code>%s</code></p>
<p>Created: %s</p>
<p>Type: %s</p>
<p><a href="%s">Open artifact</a></p>
</body>
</html>
`, htmlEscape(artifact.Title), htmlEscape(artifact.Title), artifact.ID, artifact.CreatedAt.Format(time.RFC3339), artifact.ContentType, artifact.Filename)
	return os.WriteFile(filepath.Join(webDir, "index.html"), []byte(indexHTML), 0644)
}

func newArtifactID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "art_" + hex.EncodeToString(b[:]), nil
}

func sanitizeFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = invalidFilename.ReplaceAllString(name, "-")
	name = strings.Trim(name, ".-")
	return name
}

func defaultFilename(contentType, kind string) string {
	ext := ".txt"
	if exts, _ := mime.ExtensionsByType(contentType); len(exts) > 0 {
		ext = exts[0]
	} else if kind == "report" {
		ext = ".md"
	}
	return "artifact" + ext
}

func defaultContentType(filename string) string {
	if ct := mime.TypeByExtension(filepath.Ext(filename)); ct != "" {
		return ct
	}
	return "text/plain; charset=utf-8"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func htmlEscape(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
	)
	return replacer.Replace(s)
}

func parseUnixTimestamp(raw string) (time.Time, error) {
	secs, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(secs, 0).UTC(), nil
}
