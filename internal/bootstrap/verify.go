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

	// Build ACL mask map: paths with ACLs will have inflated group bits in stat
	// because POSIX ACL mask replaces the traditional group permission bits.
	aclMask := aclMaskForPaths(m.ACLs)

	// Verify directories
	for _, d := range m.Directories {
		mode := effectiveMode(d.Mode, aclMask[d.Path])
		findings = append(findings, verifyPath(d.Path, mode, d.Owner, d.Group, "directory")...)
	}

	// Verify files
	for _, f := range m.Files {
		mode := effectiveMode(f.Mode, aclMask[f.Path])
		findings = append(findings, verifyPath(f.Path, mode, f.Owner, f.Group, "file")...)
	}

	return findings
}

// aclMaskForPaths computes the highest ACL permission bits per path.
// When setfacl adds group ACLs, the ACL mask (shown as group bits in stat)
// becomes the union of all ACL entries.
func aclMaskForPaths(acls []ACL) map[string]uint32 {
	masks := make(map[string]uint32)
	for _, a := range acls {
		if a.Default {
			continue // default ACLs don't affect the directory's own mode
		}
		var bits uint32
		if strings.Contains(a.Perms, "r") {
			bits |= 4
		}
		if strings.Contains(a.Perms, "w") {
			bits |= 2
		}
		if strings.Contains(a.Perms, "x") {
			bits |= 1
		}
		if bits > masks[a.Path] {
			masks[a.Path] = bits
		}
	}
	return masks
}

// effectiveMode adjusts the expected mode to account for ACL mask inflation.
// With ACLs, stat's group bits reflect the ACL mask, not the owning group.
func effectiveMode(baseMode string, aclGroupBits uint32) string {
	if aclGroupBits == 0 {
		return baseMode
	}
	mode, err := strconv.ParseUint(baseMode, 8, 32)
	if err != nil {
		return baseMode
	}
	// Replace group bits (middle octal digit) with the ACL mask
	existingGroup := uint32((mode >> 3) & 7)
	if aclGroupBits > existingGroup {
		mode = (mode &^ (7 << 3)) | (uint64(aclGroupBits) << 3)
	}
	return fmt.Sprintf("%o", mode)
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
