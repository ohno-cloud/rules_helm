package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestGenerateLockFileHTTPRepository(t *testing.T) {
	chart := Chart{
		Dependencies: []Dependency{{
			Name:       "cert-manager",
			Version:    "v1.12.0",
			Repository: "https://charts.jetstack.io",
		}},
	}

	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://charts.jetstack.io/index.yaml" {
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(`
apiVersion: v1
entries:
  cert-manager:
    - version: v1.12.0
      digest: 1435a289262218111685cf311eb6b57f0e8d788abd4e0b513029043df68c17ae
      urls:
        - charts/cert-manager-v1.12.0.tgz
`)),
		}, nil
	})}

	lock, err := chart.GenerateLockFile(context.Background(), client)
	if err != nil {
		t.Fatalf("GenerateLockFile() error = %v", err)
	}

	entry, ok := lock.Repositories["io_jetstack_charts_cert_manager"]
	if !ok {
		t.Fatalf("missing lock entry: %#v", lock.Repositories)
	}

	if entry.Digest != "sha256:1435a289262218111685cf311eb6b57f0e8d788abd4e0b513029043df68c17ae" {
		t.Fatalf("unexpected digest: %s", entry.Digest)
	}

	if entry.Repository != "" {
		t.Fatalf("unexpected repository in HTTP entry: %s", entry.Repository)
	}

	if len(entry.Urls) != 1 || entry.Urls[0] != "https://charts.jetstack.io/charts/cert-manager-v1.12.0.tgz" {
		t.Fatalf("unexpected urls: %#v", entry.Urls)
	}
}

func TestBzlRepoNameIncludesOCIPath(t *testing.T) {
	got := bzlRepoName("oci://ghcr.io/woodpecker-ci/helm", "woodpecker", "")
	want := "io_ghcr_woodpecker_ci_helm_woodpecker"
	if got != want {
		t.Fatalf("bzlRepoName() = %q, want %q", got, want)
	}
}

func TestNormalizeDigest(t *testing.T) {
	if got := normalizeDigest("abc123"); got != "sha256:abc123" {
		t.Fatalf("normalizeDigest() = %q", got)
	}

	if got := normalizeDigest("sha256:abc123"); got != "sha256:abc123" {
		t.Fatalf("normalizeDigest() preserved digest incorrectly: %q", got)
	}
}
