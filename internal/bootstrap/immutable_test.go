package bootstrap

import (
	"errors"
	"testing"
)

func TestWithMutable_CallsFn(t *testing.T) {
	called := false
	err := WithMutable("/nonexistent/path", func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !called {
		t.Error("fn was not called")
	}
}

func TestWithMutable_PropagatesError(t *testing.T) {
	sentinel := errors.New("test error")
	err := WithMutable("/nonexistent/path", func() error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got %v", err)
	}
}

func TestWithMutable_NilFnReturn(t *testing.T) {
	err := WithMutable("/nonexistent/path", func() error {
		return nil
	})
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestSetImmutable_DoesNotPanic(t *testing.T) {
	// chattr will fail on macOS/non-root — verify it doesn't panic
	SetImmutable("/nonexistent/path")
}

func TestClearImmutable_DoesNotPanic(t *testing.T) {
	ClearImmutable("/nonexistent/path")
}
