package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/ConspiracyOS/conctl/internal/artifacts"
	"github.com/ConspiracyOS/conctl/internal/strutil"
)

func runArtifact(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: conctl artifact <create|show> [args]")
		os.Exit(1)
	}
	switch args[0] {
	case "create":
		artifactCreate(args[1:])
	case "show":
		artifactShow(args[1:])
	case "link":
		artifactLink(args[1:])
	case "verify":
		artifactVerify(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown artifact command: %s\n", args[0])
		os.Exit(1)
	}
}

func artifactCreate(args []string) {
	fs := flag.NewFlagSet("artifact create", flag.ExitOnError)
	title := fs.String("title", "", "artifact title")
	kind := fs.String("kind", "file", "artifact kind")
	contentType := fs.String("content-type", "", "content type")
	filename := fs.String("name", "", "file name")
	fromPath := fs.String("from", "", "read artifact content from file")
	createdBy := fs.String("created-by", os.Getenv("CONOS_ACTOR"), "artifact creator")
	runID := fs.String("run-id", os.Getenv("CONOS_RUN_ID"), "run identifier")
	taskID := fs.String("task-id", "", "task identifier")
	audience := fs.String("audience", "user", "artifact audience")
	exposure := fs.String("exposure", string(artifacts.ExposureDashboardLocal), "private|dashboard_local|authenticated_dashboard")
	fs.Parse(args)

	var content []byte
	var err error
	if *fromPath != "" {
		content, err = os.ReadFile(*fromPath)
	} else {
		content, err = io.ReadAll(os.Stdin)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	artifact, err := artifacts.Create(artifacts.CreateInput{
		ArtifactsRoot: artifactCreateRoot,
		StatusRoot:    artifactStatusRoot,
		Title:         *title,
		Kind:          *kind,
		ContentType:   *contentType,
		Filename:      *filename,
		CreatedBy:     *createdBy,
		RunID:         *runID,
		TaskID:        *taskID,
		Audience:      *audience,
		Exposure:      artifacts.Exposure(*exposure),
		Content:       content,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	data, _ := json.MarshalIndent(artifact, "", "  ")
	fmt.Println(string(data))
}

func artifactShowIn(root, id string) (*artifacts.Artifact, error) {
	return artifacts.Show(root, id)
}

func artifactShow(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: conctl artifact show <artifact-id>")
		os.Exit(1)
	}
	artifact, err := artifactShowIn(artifactShowRoot, args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	data, _ := json.MarshalIndent(artifact, "", "  ")
	fmt.Println(string(data))
}

func artifactLink(args []string) {
	fs := flag.NewFlagSet("artifact link", flag.ExitOnError)
	baseURL := fs.String("base-url", strings.TrimRight(os.Getenv("CONOS_BASE_URL"), "/"), "base URL for the artifact link")
	ttl := fs.Duration("ttl", time.Hour, "signed link TTL")
	secretFile := fs.String("secret-file", strutil.FirstNonEmpty(os.Getenv("CONOS_ARTIFACT_SIGNING_KEY_FILE"), "/etc/conos/artifact-signing.key"), "path to HMAC signing key")
	fs.Parse(args)
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: conctl artifact link [--base-url URL] [--ttl 1h] <artifact-id>")
		os.Exit(1)
	}
	artifact, err := artifactShowIn(artifactShowRoot, fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	secret, err := os.ReadFile(*secretFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	link, err := artifacts.MintSignedLink(*baseURL, artifact, bytesTrimSpace(secret), *ttl, time.Now().UTC())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := artifacts.Save(artifact); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	data, _ := json.MarshalIndent(link, "", "  ")
	fmt.Println(string(data))
}

func artifactVerify(args []string) {
	fs := flag.NewFlagSet("artifact verify", flag.ExitOnError)
	exp := fs.String("exp", "", "expiry unix timestamp")
	sig := fs.String("sig", "", "hex signature")
	secretFile := fs.String("secret-file", strutil.FirstNonEmpty(os.Getenv("CONOS_ARTIFACT_SIGNING_KEY_FILE"), "/etc/conos/artifact-signing.key"), "path to HMAC signing key")
	fs.Parse(args)
	if fs.NArg() != 1 || *exp == "" || *sig == "" {
		fmt.Fprintln(os.Stderr, "usage: conctl artifact verify --exp <unix> --sig <hex> <artifact-id>")
		os.Exit(1)
	}
	artifact, err := artifactShowIn(artifactShowRoot, fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	secret, err := os.ReadFile(*secretFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := artifacts.VerifySignedLink(artifact, bytesTrimSpace(secret), *exp, *sig, time.Now().UTC()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("ok")
}

func runArtifactAuth(args []string) {
	fs := flag.NewFlagSet("artifact-auth", flag.ExitOnError)
	socketPath := fs.String("socket", artifacts.DefaultAuthSocket, "Unix socket path")
	secretFile := fs.String("secret-file", strutil.FirstNonEmpty(os.Getenv("CONOS_ARTIFACT_SIGNING_KEY_FILE"), "/etc/conos/artifact-signing.key"), "path to HMAC signing key")
	root := fs.String("artifacts-root", artifactShowRoot, "artifacts root directory")
	fs.Parse(args)

	if err := artifacts.ListenAndServeAuth(artifacts.AuthConfig{
		SocketPath:    *socketPath,
		SecretFile:    *secretFile,
		ArtifactsRoot: *root,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func bytesTrimSpace(b []byte) []byte {
	return bytes.TrimSpace(b)
}
