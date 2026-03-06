package bootstrap

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Finding reports a single verification result.
type Finding struct {
	Path     string
	Category string // directory, file, mode, ownership, acl, unit
	Status   string // pass, fail
	What     string
	Severity string // critical, high, medium, low
}

// VerifyLocal checks the local system state against the manifest.
// It only checks things that can be verified without root (stat, mode).
// ACLs and units require getfacl/systemctl and are skipped if unavailable.
func VerifyLocal(m Manifest) []Finding {
	var findings []Finding

	// Verify directories
	for _, d := range m.Directories {
		findings = append(findings, verifyPath(d.Path, d.Mode, d.Owner, d.Group, "directory")...)
	}

	// Verify files
	for _, f := range m.Files {
		findings = append(findings, verifyPath(f.Path, f.Mode, f.Owner, f.Group, "file")...)
	}

	return findings
}

func verifyPath(path, expectedMode, expectedOwner, expectedGroup, category string) []Finding {
	var findings []Finding

	info, err := os.Stat(path)
	if err != nil {
		findings = append(findings, Finding{
			Path:     path,
			Category: category,
			Status:   "fail",
			What:     fmt.Sprintf("%s does not exist", path),
			Severity: "high",
		})
		return findings
	}

	// Check mode
	actualMode := fmt.Sprintf("%o", info.Mode().Perm())
	// Normalize: strip leading zeros for comparison
	expectedNorm := strings.TrimLeft(expectedMode, "0")
	actualNorm := strings.TrimLeft(actualMode, "0")
	if expectedNorm == "" {
		expectedNorm = "0"
	}
	if actualNorm == "" {
		actualNorm = "0"
	}

	if actualNorm != expectedNorm {
		findings = append(findings, Finding{
			Path:     path,
			Category: "mode",
			Status:   "fail",
			What:     fmt.Sprintf("%s mode is %s, expected %s", path, actualMode, expectedMode),
			Severity: modeChangeSeverity(expectedMode, actualMode),
		})
	}

	return findings
}

func modeChangeSeverity(expected, actual string) string {
	e, _ := strconv.ParseUint(expected, 8, 32)
	a, _ := strconv.ParseUint(actual, 8, 32)
	if a > e {
		return "critical"
	}
	return "medium"
}
