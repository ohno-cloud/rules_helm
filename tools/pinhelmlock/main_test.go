package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func artifacts(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	var sums strings.Builder
	for _, platform := range platforms {
		write(t, filepath.Join(directory, "helmlock-"+platform), platform)
		fmt.Fprintf(&sums, "%x  helmlock-%s\n", sha256.Sum256([]byte(platform)), platform)
	}
	write(t, filepath.Join(directory, "SHA256SUMS"), sums.String())
	return directory
}

func TestChecksumsAndTampering(t *testing.T) {
	directory := artifacts(t)
	if result, err := checksums(directory); err != nil || len(result) != 4 {
		t.Fatalf("checksums = %v, error = %v", result, err)
	}
	write(t, filepath.Join(directory, "helmlock-linux-arm64"), "tampered")
	if _, err := checksums(directory); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
}

func TestPinAndVerify(t *testing.T) {
	directory := artifacts(t)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "private"), 0700); err != nil {
		t.Fatal(err)
	}
	module := filepath.Join(root, "MODULE.bazel")
	write(t, module, "module(\n    name = \"rules_helm\",\n    version = \"0.4.0\",\n)\n")
	tag := "v0.0.0-main." + strings.Repeat("a", 40)
	if err := pin(root, directory, tag, "0.5.0"); err != nil {
		t.Fatal(err)
	}
	if err := verify(root, directory); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(root, "private", "helmlock.json"))
	if err != nil {
		t.Fatal(err)
	}
	var value metadata
	if err := json.Unmarshal(content, &value); err != nil {
		t.Fatal(err)
	}
	if value.SourceCommit != strings.Repeat("a", 40) || value.RulesVersion != "0.5.0" {
		t.Fatalf("unexpected metadata: %+v", value)
	}
	write(t, module, `module(name = "rules_helm", version = "0.6.0")`)
	if err := verify(root, directory); err == nil || !strings.Contains(err.Error(), "versions differ") {
		t.Fatalf("expected mismatched module version, got %v", err)
	}
}

func TestVersions(t *testing.T) {
	for _, version := range []string{"0.5.0", "1.0.0-rc.1"} {
		if err := checkVersion(version); err != nil {
			t.Error(err)
		}
	}
	for _, version := range []string{"v0.5.0", "main", "1.2", "1.0.0\nmalicious", "$(command)"} {
		if err := checkVersion(version); err == nil {
			t.Errorf("accepted invalid version %q", version)
		}
	}
}

func TestInvalidChecksumEntries(t *testing.T) {
	for _, tc := range []struct{ name, content, want string }{
		{"path", strings.Repeat("a", 64) + "  ../../bad\n", "unexpected"},
		{"missing", "", "missing platform"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			write(t, filepath.Join(directory, "SHA256SUMS"), tc.content)
			if _, err := checksums(directory); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %s, got %v", tc.want, err)
			}
		})
	}
}

func TestCommandArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"unknown"}, {"verify"}, {"pin", "--artifacts", "."}} {
		if err := run(args); err == nil {
			t.Errorf("accepted arguments %v", args)
		}
	}
}
