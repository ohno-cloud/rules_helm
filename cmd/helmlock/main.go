package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"

	"gopkg.in/yaml.v3"
)

// A subset of the Helm Charts.yaml file
// https://helm.sh/docs/topics/charts
type Chart struct {
	//# A list of the chart requirements
	Dependencies []struct {
		// The name of the chart (nginx)
		Name string `yaml:"name" json:"name"`
		// The version of the chart ("1.2.3")
		Version string `yaml:"version" json:"version"`
		// The repository URL ("https://example.com/charts") or alias ("@repo-name")
		Repository string `yaml:"repository" json:"repository"`
	} `yaml:"dependencies" json:"dependencies"`
}

type Lockfile struct {
	// Repositories is a map of bazel repository names to lock file contents
	Repositories map[string]LockEntry `yaml:"repositories" json:"repositories"`
}

type LockEntry struct {
	Digest string   `yaml:"digest" json:"digest"`
	Urls   []string `yaml:"urls" json:"urls"`
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
		os.Exit(1)
	}

	var chart Chart
	if err = yaml.Unmarshal(buf, &chart); err != nil {
		fmt.Printf("failed unmarshal file: %s", err)
		os.Exit(1)
	}

	lock, err := chart.GenerateLockFile(context.Background(), http.DefaultClient)
	if err != nil {
		fmt.Printf("failed to generate lock file: %s", err)
		os.Exit(1)
	}

	lockBuf, err := json.Marshal(lock)
	if err != nil {
		fmt.Printf("failed to marshal lock file to json: %s", err)
		os.Exit(1)
	}

	if *outputFile == "" {
		fmt.Printf("%s", lockBuf)
	} else {
		out, err := os.OpenFile(*outputFile, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0666)
		if err != nil {
			fmt.Printf("failed to open output file (%s): %s", *outputFile, err)
			os.Exit(1)
		}
		defer out.Close()

		if _, err = out.Write(lockBuf); err != nil {
			fmt.Printf("failed to write to output file (%s): %s", *outputFile, err)
			os.Exit(1)
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
	var lock Lockfile
	lock.Repositories = make(map[string]LockEntry)

	wants := c.groupByRepository()

	for repo, wantCharts := range wants {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/index.yaml", repo), nil)
		if err != nil {
			return nil, err
		}

		res, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()

		buf, err := io.ReadAll(res.Body)
		if err != nil {
			return nil, err
		}

		var repo RepositoryIndex
		if err = yaml.Unmarshal(buf, &repo); err != nil {
			return nil, err
		}

		foundCharts := map[string]bool{}

		for chart, chartVersions := range repo.Entries {
			foundVersions := map[string]bool{}
			// Filter out chart entries we're not after
			if wantVersions, ok := wantCharts[chart]; ok {
				foundCharts[chart] = true
				for _, version := range chartVersions {

					if slices.Contains(wantVersions, version.Version) {
						var lockName string
						if len(wantVersions) > 1 {
							lockName = fmt.Sprintf("%s_%s", chart, version.Version)
						} else {
							lockName = fmt.Sprintf("%s", chart)
						}

						lock.Repositories[lockName] = LockEntry{
							Digest: version.Digest,
							Urls:   version.Urls,
						}

						foundVersions[version.Version] = true
					}

				}

				if len(foundVersions) != len(wantVersions) {
					return nil, fmt.Errorf("did not find all versions of chart %s, found %v", chart, foundVersions)
				}
			}

		}

		if len(foundCharts) != len(wantCharts) {
			return nil, fmt.Errorf("did not find all charts for repository %s, found %v", repo, foundCharts)
		}
	}

	return &lock, nil
}
