package main

import (
	"encoding/json"
	"io"
	"os"
	"testing"
)

func TestTaskContractOpenAndShow(t *testing.T) {
	root := t.TempDir()
	orig := taskContractsRoot
	taskContractsRoot = root
	defer func() { taskContractsRoot = orig }()

	oldStdout := os.Stdout
	defer func() { os.Stdout = oldStdout }()

	stdoutR, stdoutW, _ := os.Pipe()
	os.Stdout = stdoutW
	taskContractOpen([]string{"--id", "CON-TASK-CLI", "--title", "CLI Task"})
	stdoutW.Close()
	out, _ := io.ReadAll(stdoutR)

	var created taskContractJSON
	if err := json.Unmarshal(out, &created); err != nil {
		t.Fatalf("unmarshal open output: %v; output=%s", err, string(out))
	}
	if created.Status != "open" {
		t.Fatalf("unexpected created task: %+v", created)
	}

	stdoutR2, stdoutW2, _ := os.Pipe()
	os.Stdout = stdoutW2
	taskContractShow([]string{"CON-TASK-CLI"})
	stdoutW2.Close()
	out2, _ := io.ReadAll(stdoutR2)

	var shown taskContractJSON
	if err := json.Unmarshal(out2, &shown); err != nil {
		t.Fatalf("unmarshal show output: %v; output=%s", err, string(out2))
	}
	if shown.Title != "CLI Task" {
		t.Fatalf("unexpected shown task: %+v", shown)
	}
}
