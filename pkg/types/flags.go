/*
Copyright 2023 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package types

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"
)

func (c *Configuration) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.configFile, "config", "", "path to an optional base configuration file.")
	fs.StringVar(&c.Kubeconfig, "kubeconfig", c.Kubeconfig, "path to the kubeconfig file.")
	fs.IntVar(&c.Parallel, "parallel", c.Parallel, "number of parallel threads in test framework (automatically sets the --nodes Ginkgo flag).")
	fs.IntVar(&c.Verbosity, "verbosity", c.Verbosity, "verbosity of test framework (values >= 6 automatically sets the -v Ginkgo flag).")
	fs.StringVar(&c.OutputDir, "output-dir", c.OutputDir, "directory for logs.")
	fs.StringVar(&c.Skip, "skip", c.Skip, "skip specific tests. allows regular expressions.")
	fs.StringVar(&c.ConformanceImage, "conformance-image", c.ConformanceImage, "specify a conformance container image of your choice.")
	fs.StringVar(&c.BusyboxImage, "busybox-image", c.BusyboxImage, "specify an alternate busybox container image.")
	fs.StringVar(&c.Namespace, "namespace", c.Namespace, "the namespace where the conformance pod is created.")
	fs.BoolVar(&c.DryRun, "dry-run", c.DryRun, "run in dry run mode.")
	fs.StringVar(&c.TestRepoList, "test-repo-list", c.TestRepoList, "yaml file to override registries for test images.")
	fs.StringVar(&c.TestRepo, "test-repo", c.TestRepo, "skip specific tests. allows regular expressions.")
	fs.DurationVar(&c.StartupTimeout, "startup-timeout", c.StartupTimeout, "max time to wait for the conformance test pod to start up.")
	fs.StringSliceVar(&c.ExtraArgs, "extra-args", c.ExtraArgs, "Additional parameters to be provided to the conformance container. These parameters should be specified as key-value pairs, separated by commas. Each parameter should start with -- (e.g., --clean-start=true,--allowed-not-ready-nodes=2)")
	fs.StringSliceVar(&c.ExtraGinkgoArgs, "extra-ginkgo-args", c.ExtraGinkgoArgs, "Additional parameters to be provided to Ginkgo runner. This flag has the same format as --extra-args.")
}

func (c *Configuration) Complete(fs *pflag.FlagSet) (*Configuration, error) {
	result := c

	if c.configFile != "" {
		loaded, err := loadConfiguration(c.configFile)
		if err != nil {
			return nil, err
		}

		// overwrite config loaded from file with ad-hoc CLI flags
		overwrite(fs, "kubeconfig", &loaded.Kubeconfig, c.Kubeconfig)
		overwrite(fs, "parallel", &loaded.Parallel, c.Parallel)
		overwrite(fs, "verbosity", &loaded.Verbosity, c.Verbosity)
		overwrite(fs, "output-dir", &loaded.OutputDir, c.OutputDir)
		overwrite(fs, "skip", &loaded.Skip, c.Skip)
		overwrite(fs, "conformance-image", &loaded.ConformanceImage, c.ConformanceImage)
		overwrite(fs, "busybox-image", &loaded.BusyboxImage, c.BusyboxImage)
		overwrite(fs, "namespace", &loaded.Namespace, c.Namespace)
		overwrite(fs, "dry-run", &loaded.DryRun, c.DryRun)
		overwrite(fs, "startup-timeout", &loaded.StartupTimeout, c.StartupTimeout)
		overwrite(fs, "test-repo-list", &loaded.TestRepoList, c.TestRepoList)
		overwrite(fs, "test-repo", &loaded.TestRepo, c.TestRepo)
		overwriteSlice(fs, "extra-args", &loaded.ExtraArgs, c.ExtraArgs)
		overwriteSlice(fs, "extra-ginkgo-args", &loaded.ExtraGinkgoArgs, c.ExtraGinkgoArgs)

		result = loaded
	}

	if result.Kubeconfig == "" {
		if envvar := os.Getenv("KUBECONFIG"); envvar != "" {
			result.Kubeconfig = envvar
		} else {
			// only try to determine the home folder if absolutely needed to allow running
			// on systems where no home is configured
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("failed to determine home directory: %w", err)
			}

			result.Kubeconfig = filepath.Join(homeDir, ".kube", "config")
		}
	}

	// handle edge cases where $KUBECONFIG contains a literal '~'
	if strings.HasPrefix(result.Kubeconfig, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to determine home directory: %w", err)
		}

		result.Kubeconfig = filepath.Join(homeDir, c.Kubeconfig[1:])
	}

	if err := result.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return result, nil
}

func overwrite[T comparable](fs *pflag.FlagSet, flagName string, dst *T, src T) {
	empty := new(T)
	if src != *empty && (fs.Changed(flagName) || *dst == *empty) {
		*dst = src
	}
}

func overwriteSlice[T comparable](fs *pflag.FlagSet, flagName string, dst *[]T, src []T) {
	if len(src) > 0 && (fs.Changed(flagName) || len(*dst) == 0) {
		*dst = src
	}
}

func resolveKubeconfig(kubeconfig string) (string, error) {
	if kubeconfig == "" {
		if envvar := os.Getenv("KUBECONFIG"); envvar != "" {
			kubeconfig = envvar
		} else {
			// only try to determine the home folder if absolutely needed to allow running
			// on systems where no home is configured
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("failed to determine home directory: %w", err)
			}

			kubeconfig = filepath.Join(homeDir, ".kube", "config")
		}
	}

	// handle edge cases where $KUBECONFIG contains a literal '~'
	if strings.HasPrefix(kubeconfig, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to determine home directory: %w", err)
		}

		kubeconfig = strings.ReplaceAll(kubeconfig, "~", homeDir)
	}

	return kubeconfig, nil
}
