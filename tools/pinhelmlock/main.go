// pinhelmlock translates GoReleaser checksums into Bazel pins and verifies them.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
)

var platforms = []string{"darwin-amd64", "darwin-arm64", "linux-amd64", "linux-arm64"}

type metadata struct {
	Protocol     int               `json:"protocol"`
	RulesVersion string            `json:"rules_version"`
	Tag          string            `json:"tag"`
	SourceCommit string            `json:"source_commit"`
	SHA256       map[string]string `json:"sha256"`
}

var (
	versionPattern  = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)
	snapshotPattern = regexp.MustCompile(`^v0\.0\.0-main\.([0-9a-f]{40})$`)
	checksumPattern = regexp.MustCompile(`^([0-9a-f]{64})\s+\*?helmlock-(darwin|linux)-(amd64|arm64)$`)
	modulePattern   = regexp.MustCompile(`(module\(\s*name\s*=\s*"rules_helm",\s*version\s*=\s*)"([^"]+)"`)
)

func checksums(directory string) (map[string]string, error) {
	content, err := os.ReadFile(filepath.Join(directory, "SHA256SUMS"))
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		if line == "" {
			continue
		}
		match := checksumPattern.FindStringSubmatch(line)
		if match == nil {
			return nil, fmt.Errorf("unexpected GoReleaser checksum entry: %q", line)
		}
		platform := match[2] + "-" + match[3]
		if _, ok := result[platform]; ok {
			return nil, fmt.Errorf("duplicate checksum for %s", platform)
		}
		file, err := os.Open(filepath.Join(directory, "helmlock-"+platform))
		if err != nil {
			return nil, err
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return nil, copyErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		digest := hex.EncodeToString(hash.Sum(nil))
		if digest != match[1] {
			return nil, fmt.Errorf("checksum mismatch for %s", platform)
		}
		result[platform] = digest
	}
	if len(result) != len(platforms) {
		return nil, fmt.Errorf("missing platform artifacts")
	}
	return result, nil
}

func checkVersion(version string) error {
	if !versionPattern.MatchString(version) {
		return fmt.Errorf("version must be a semantic version without a leading v")
	}
	return nil
}

func pin(root, directory, tag, version string) error {
	if err := checkVersion(version); err != nil {
		return err
	}
	match := snapshotPattern.FindStringSubmatch(tag)
	if match == nil {
		return fmt.Errorf("expected an immutable main snapshot tag")
	}
	digests, err := checksums(directory)
	if err != nil {
		return err
	}
	modulePath := filepath.Join(root, "MODULE.bazel")
	module, err := os.ReadFile(modulePath)
	if err != nil {
		return err
	}
	if !modulePattern.Match(module) {
		return fmt.Errorf("could not find rules_helm version in MODULE.bazel")
	}
	updated := modulePattern.ReplaceAllString(string(module), `${1}"`+version+`"`)
	value := metadata{Protocol: 1, RulesVersion: version, Tag: tag, SourceCommit: match[1], SHA256: digests}
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "private", "helmlock.json"), append(content, '\n'), 0644); err != nil {
		return err
	}
	return os.WriteFile(modulePath, []byte(updated), 0644)
}

func verify(root, directory string) error {
	content, err := os.ReadFile(filepath.Join(root, "private", "helmlock.json"))
	if err != nil {
		return err
	}
	var value metadata
	if err := json.Unmarshal(content, &value); err != nil {
		return err
	}
	if err := checkVersion(value.RulesVersion); err != nil {
		return err
	}
	match := snapshotPattern.FindStringSubmatch(value.Tag)
	if value.Protocol != 1 || match == nil || match[1] != value.SourceCommit {
		return fmt.Errorf("invalid pinned snapshot identity")
	}
	digests, err := checksums(directory)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(digests, value.SHA256) {
		return fmt.Errorf("downloaded artifacts differ from Bazel pins")
	}
	module, err := os.ReadFile(filepath.Join(root, "MODULE.bazel"))
	if err != nil {
		return err
	}
	moduleMatch := modulePattern.FindSubmatch(module)
	if moduleMatch == nil || string(moduleMatch[2]) != value.RulesVersion {
		return fmt.Errorf("MODULE.bazel and release metadata versions differ")
	}
	return nil
}

func run(args []string) error {
	if len(args) == 0 || (args[0] != "pin" && args[0] != "verify") {
		return fmt.Errorf("usage: pinhelmlock <pin|verify> --artifacts <directory> [--tag <snapshot> --version <version>]")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	directory := flags.String("artifacts", "", "directory containing GoReleaser binaries and SHA256SUMS")
	tag := flags.String("tag", "", "immutable main snapshot tag")
	version := flags.String("version", "", "rules version without a leading v")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if *directory == "" || flags.NArg() != 0 {
		return fmt.Errorf("--artifacts is required and positional arguments are not accepted")
	}
	// bazel run starts in the runfiles tree, but pins belong to the checkout.
	root := os.Getenv("BUILD_WORKSPACE_DIRECTORY")
	if root == "" {
		root = "."
	}
	if !filepath.IsAbs(*directory) {
		*directory = filepath.Join(root, *directory)
	}
	if args[0] == "pin" {
		return pin(root, *directory, *tag, *version)
	}
	return verify(root, *directory)
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
