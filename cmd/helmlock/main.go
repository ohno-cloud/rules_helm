package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"gopkg.in/yaml.v3"
)

// A subset of the Helm Charts.yaml file
// https://helm.sh/docs/topics/charts
type Chart struct {
	//# A list of the chart requirements
	Dependencies []Dependency `yaml:"dependencies" json:"dependencies"`
}

type Dependency struct {
	// The name of the chart (nginx)
	Name string `yaml:"name" json:"name"`
	// The version of the chart ("1.2.3")
	Version string `yaml:"version" json:"version"`
	// The repository URL ("https://example.com/charts") or alias ("@repo-name")
	Repository string `yaml:"repository" json:"repository"`
}

type Lockfile struct {
	// Repositories is a map of bazel repository names to lock file contents
	Repositories map[string]LockEntry `yaml:"repositories" json:"repositories"`
}

type LockEntry struct {
	Digest     string   `yaml:"digest" json:"digest"`
	Urls       []string `yaml:"urls,omitempty" json:"urls,omitempty"`
	Repository string   `yaml:"repository,omitempty" json:"repository,omitempty"`
	Chart      string   `yaml:"chart" json:"chart"`
	Version    string   `yaml:"version" json:"version"`
}

type RepositoryIndex struct {
	ApiVersion string `yaml:"apiVersion"`
	Entries    map[string][]struct {
		ApiVersion string   `yaml:"apiVersion"`
		Name       string   `yaml:"name"`
		Urls       []string `yaml:"urls"`
		Version    string   `yaml:"version"`
		Digest     string   `yaml:"digest"`
	} `yaml:"entries"`
}

const helmChartLayerMediaType = "application/vnd.cncf.helm.chart.content.v1.tar+gzip"

var inputFile = flag.String("chart", "", "path to Chart.yaml to generate from")
var outputFile = flag.String("output", "", "path to write lock file to")

func main() {
	flag.Parse()

	file, err := os.Open(*inputFile)
	if err != nil {
		fmt.Printf("failed to open file: %s - %s", *inputFile, err)
		os.Exit(1)
	}
	defer file.Close()

	buf, err := io.ReadAll(file)
	if err != nil {
		fmt.Printf("failed read file: %s", err)
		os.Exit(2)
	}

	var chart Chart
	if err = yaml.Unmarshal(buf, &chart); err != nil {
		fmt.Printf("failed unmarshal file: %s", err)
		os.Exit(3)
	}

	lock, err := chart.GenerateLockFile(context.Background(), http.DefaultClient)
	if err != nil {
		fmt.Printf("failed to generate lock file: %s", err)
		fmt.Fprintf(os.Stderr, "failed to generate lock file: %s", err)
		os.Exit(4)
	}

	lockBuf, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		fmt.Printf("failed to marshal lock file to json: %s", err)
		os.Exit(5)
	}

	if *outputFile == "" {
		fmt.Printf("%s", lockBuf)
	} else {
		out, err := os.OpenFile(*outputFile, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0666)
		if err != nil {
			fmt.Printf("failed to open output file (%s): %s", *outputFile, err)
			os.Exit(6)
		}
		defer out.Close()

		if _, err = out.Write(lockBuf); err != nil {
			fmt.Printf("failed to write to output file (%s): %s", *outputFile, err)
			os.Exit(7)
		}
	}
}

// groupByRepository groups the Dependencies field group repository, then
// by charts and by wanted versions.
func (c *Chart) groupByRepository() map[string]map[string][]string {
	output := make(map[string]map[string][]string)
	for _, dep := range c.Dependencies {
		if repo, ok := output[dep.Repository]; ok {
			if chart, ok := repo[dep.Name]; ok {
				chart = append(chart, dep.Version)
				repo[dep.Name] = chart
			} else {
				repo[dep.Name] = []string{dep.Version}
			}
			output[dep.Repository] = repo
		} else {
			val := map[string][]string{
				dep.Name: {dep.Version},
			}
			output[dep.Repository] = val
		}
	}

	return output
}

func (c *Chart) GenerateLockFile(ctx context.Context, client *http.Client) (*Lockfile, error) {
	lock := &Lockfile{
		Repositories: make(map[string]LockEntry),
	}

	wants := c.groupByRepository()

	for repo, wantCharts := range wants {
		var err error
		if isOCIRepository(repo) {
			err = generateOCILockEntries(ctx, lock, repo, wantCharts)
		} else {
			err = generateHTTPLockEntries(ctx, client, lock, repo, wantCharts)
		}
		if err != nil {
			return nil, err
		}
	}

	return lock, nil
}

func generateHTTPLockEntries(ctx context.Context, client *http.Client, lock *Lockfile, repo string, wantCharts map[string][]string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/index.yaml", repo), nil)
	if err != nil {
		return fmt.Errorf("failed request from repo %s: %w", repo, err)
	}

	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	buf, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	var repoIndex RepositoryIndex
	if err = yaml.Unmarshal(buf, &repoIndex); err != nil {
		return err
	}

	foundCharts := map[string]bool{}

	for chart, chartVersions := range repoIndex.Entries {
		foundVersions := map[string]bool{}
		seenVersions := []string{}

		if wantVersions, ok := wantCharts[chart]; ok {
			foundCharts[chart] = true
			for _, version := range chartVersions {
				seenVersions = append(seenVersions, version.Version)
				if slices.Contains(wantVersions, version.Version) {
					lockName := bzlRepoName(repo, chart, lockVersionSuffix(wantVersions, version.Version))
					lock.Repositories[lockName] = LockEntry{
						Chart:   chart,
						Version: version.Version,
						Digest:  normalizeDigest(version.Digest),
						Urls:    checkUrls(repo, version.Urls),
					}

					foundVersions[version.Version] = true
				}
			}

			if len(foundVersions) != len(wantVersions) {
				return fmt.Errorf("did not find all versions of chart %s, found %v and saw %v", chart, foundVersions, seenVersions)
			}
		}
	}

	if len(foundCharts) != len(wantCharts) {
		return fmt.Errorf("did not find all charts for repository %s, found %v", repo, foundCharts)
	}

	return nil
}

func generateOCILockEntries(ctx context.Context, lock *Lockfile, repo string, wantCharts map[string][]string) error {
	for chart, wantVersions := range wantCharts {
		for _, version := range wantVersions {
			digest, err := resolveOCIChartDigest(ctx, repo, chart, version)
			if err != nil {
				return err
			}

			lockName := bzlRepoName(repo, chart, lockVersionSuffix(wantVersions, version))
			lock.Repositories[lockName] = LockEntry{
				Chart:      chart,
				Version:    version,
				Digest:     digest,
				Repository: repo,
			}
		}
	}

	return nil
}

func resolveOCIChartDigest(ctx context.Context, repo, chart, version string) (string, error) {
	ref, err := name.ParseReference(fmt.Sprintf("%s/%s:%s", strings.TrimPrefix(repo, "oci://"), chart, version))
	if err != nil {
		return "", fmt.Errorf("parse oci reference for %s/%s:%s: %w", repo, chart, version, err)
	}

	desc, err := remote.Get(ref, remote.WithContext(ctx), remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return "", fmt.Errorf("resolve oci reference %s: %w", ref.Name(), err)
	}

	manifest, err := desc.ImageIndex()
	if err == nil {
		return resolveOCIChartDigestFromIndex(manifest)
	}

	image, err := desc.Image()
	if err != nil {
		return "", fmt.Errorf("read oci manifest for %s: %w", ref.Name(), err)
	}

	return resolveOCIChartDigestFromImage(image)
}

func resolveOCIChartDigestFromIndex(index v1.ImageIndex) (string, error) {
	indexManifest, err := index.IndexManifest()
	if err != nil {
		return "", err
	}

	for _, manifest := range indexManifest.Manifests {
		image, err := index.Image(manifest.Digest)
		if err != nil {
			return "", err
		}

		digest, err := resolveOCIChartDigestFromImage(image)
		if err == nil {
			return digest, nil
		}
	}

	return "", fmt.Errorf("did not find a helm chart layer in OCI index")
}

func resolveOCIChartDigestFromImage(image v1.Image) (string, error) {
	manifest, err := image.Manifest()
	if err != nil {
		return "", err
	}

	for _, layer := range manifest.Layers {
		if string(layer.MediaType) == helmChartLayerMediaType {
			return layer.Digest.String(), nil
		}
	}

	return "", fmt.Errorf("did not find a helm chart layer in OCI manifest")
}

func lockVersionSuffix(wantVersions []string, version string) string {
	if len(wantVersions) > 1 {
		return version
	}
	return ""
}

func isOCIRepository(repo string) bool {
	return strings.HasPrefix(strings.ToLower(repo), "oci://")
}

func normalizeDigest(digest string) string {
	digest = strings.TrimSpace(digest)
	if digest == "" {
		return digest
	}
	if strings.Contains(digest, ":") {
		return digest
	}
	return "sha256:" + digest
}

func bzlRepoName(repo, chartName, version string) string {
	segments := make([]string, 0)

	repoURL, err := url.Parse(repo)
	if err == nil {
		hostname := strings.ReplaceAll(repoURL.Hostname(), "-", "_")
		parts := strings.Split(hostname, ".")
		slices.Reverse(parts)
		segments = append(segments, parts...)

		if isOCIRepository(repo) {
			for _, part := range strings.Split(strings.Trim(repoURL.Path, "/"), "/") {
				if part != "" {
					segments = append(segments, strings.ReplaceAll(part, "-", "_"))
				}
			}
		}
	}

	segments = append(segments, strings.Split(chartName, "-")...)

	if version != "" {
		segments = append(segments, strings.Split(version, ".")...)
	}

	return strings.Join(segments, "_")
}

func checkUrls(repo string, urls []string) []string {
	checked := make([]string, len(urls))

	for idx, url := range urls {
		if strings.HasPrefix(url, "/") {
			checked[idx] = fmt.Sprintf("%s%s", repo, url)
		} else if strings.HasPrefix(strings.ToLower(url), "http") {
			checked[idx] = url
		} else {
			checked[idx] = fmt.Sprintf("%s/%s", repo, url)
		}
	}
	return checked
}
