package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"

	"gopkg.in/yaml.v3"
)

// A subset of the Helm Charts.yaml file
// https://helm.sh/docs/topics/charts
type Chart struct {
	//# A list of the chart requirements
	Dependencies []struct {
		// The name of the chart (nginx)
		Name string `yaml:"name",json:"name"`
		// The version of the chart ("1.2.3")
		Version string `yaml:"version",json:"version"`
		// The repository URL ("https://example.com/charts") or alias ("@repo-name")
		Repository string `yaml:"repository",json:"repository"`
	} `yaml:"dependencies",json:"dependencies"`
}

type Lockfile struct {
	Repositories map[string]LockEntry `yaml:"repositories",json:"repositories"`
}

type LockEntry struct {
	Digest string   `yaml:"digest",json:"digest"`
	Urls   []string `yaml:"urls",json:"urls"`
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

var inputFile = flag.String("chart", "", "the species we are studying")

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
	fmt.Printf("%s", lockBuf)
}

func (c *Chart) GenerateLockFile(ctx context.Context, client *http.Client) (*Lockfile, error) {
	var lock Lockfile
	lock.Repositories = make(map[string]LockEntry)

	for _, dep := range c.Dependencies {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/index.yaml", dep.Repository), nil)
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

		discovered := false
	DISCOVERY:
		for chart, versions := range repo.Entries {
			// Filter out chart entries we're not after
			if dep.Name != chart {
				continue
			}

			for _, version := range versions {
				if version.Version == dep.Version {
					lock.Repositories[fmt.Sprintf("%s_%s", dep.Name, dep.Version)] = LockEntry{
						Digest: version.Digest,
						Urls:   version.Urls,
					}
					discovered = true
					break DISCOVERY
				}
			}
		}

		if !discovered {
			return nil, fmt.Errorf("did not find %s %s at %s", dep.Name, dep.Version, dep.Repository)
		}
	}

	return &lock, nil
}
