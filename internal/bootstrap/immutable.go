package bootstrap

import (
	"os/exec"
)

// WithMutable temporarily removes the immutable bit from path,
// calls fn, then re-applies the immutable bit regardless of outcome.
// Errors from chattr are silently ignored (e.g., filesystem doesn't support it).
func WithMutable(path string, fn func() error) error {
	exec.Command("chattr", "-i", path).Run()
	defer exec.Command("chattr", "+i", path).Run()
	return fn()
}

// SetImmutable sets the immutable bit on the given path.
func SetImmutable(path string) {
	exec.Command("chattr", "+i", path).Run()
}

// ClearImmutable removes the immutable bit from the given path.
func ClearImmutable(path string) {
	exec.Command("chattr", "-i", path).Run()
}
