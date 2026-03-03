package contracts

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadDir_Contracts(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	testdataDir := filepath.Join(filepath.Dir(thisFile), "testdata")

	contracts, err := LoadDir(testdataDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(contracts) == 0 {
		t.Fatal("LoadDir returned no contracts")
	}

	for _, c := range contracts {
		if c.ID == "" {
			t.Error("contract has empty ID")
		}
		if c.Type != "detective" {
			t.Errorf("contract %s: type = %q, want detective", c.ID, c.Type)
		}
	}
}
