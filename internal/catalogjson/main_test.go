package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestCatalogJSONIsCurrent fails when report/catalog.json is not what
// the code declares: run `go generate ./report`.
func TestCatalogJSONIsCurrent(t *testing.T) {
	want, err := build()
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join("..", "..", "report", "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("report/catalog.json is stale; run `go generate ./report`")
	}
}
