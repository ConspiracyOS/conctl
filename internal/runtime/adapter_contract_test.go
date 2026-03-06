package runtime

import "testing"

func TestValidateAdapterExecution(t *testing.T) {
	contract := AdapterContract{
		Name:           "exec",
		RequireRunID:   true,
		MaxOutputBytes: 8,
	}

	if err := ValidateAdapterExecution(contract, AdapterExecution{
		RunID:  "run-1",
		Stdout: []byte("ok"),
	}); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	if err := ValidateAdapterExecution(contract, AdapterExecution{
		Stdout: []byte("ok"),
	}); err == nil {
		t.Fatal("expected run_id validation error")
	}

	if err := ValidateAdapterExecution(contract, AdapterExecution{
		RunID:  "run-1",
		Stdout: []byte("0123456789"),
	}); err == nil {
		t.Fatal("expected max output validation error")
	}
}
