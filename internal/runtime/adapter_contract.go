package runtime

import "fmt"

// AdapterContract defines minimum execution guarantees for external runners.
type AdapterContract struct {
	Name           string
	RequireRunID   bool
	MaxOutputBytes int
}

// AdapterExecution captures normalized adapter execution output.
type AdapterExecution struct {
	RunID    string
	ExitCode int
	Stdout   []byte
}

// ValidateAdapterExecution validates an adapter execution against the contract.
func ValidateAdapterExecution(contract AdapterContract, exec AdapterExecution) error {
	if contract.RequireRunID && exec.RunID == "" {
		return fmt.Errorf("adapter %s requires run_id", contract.Name)
	}
	if contract.MaxOutputBytes > 0 && len(exec.Stdout) > contract.MaxOutputBytes {
		return fmt.Errorf("adapter %s output exceeds %d bytes", contract.Name, contract.MaxOutputBytes)
	}
	return nil
}
