// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kind

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"unicode"

	"github.com/docker/docker/api/types"
	dockerclient "github.com/docker/docker/client"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

const (
	// ClusterNameMaxLength is the maximum character length of a kind cluster name.
	ClusterNameMaxLength = 49

	// DefaultClusterName is the default name of new kind clusters.
	DefaultClusterName = "kind"

	// ConfigLocalRegistryFilepath is a path to a config that uses the local registry.
	ConfigLocalRegistryFilepath = "../hack/kind-config.yaml"
)

// NormalizeClusterName turns the given string to kebab-case and removes invalid characters,
// ensuring the cluster name has valid characters.
func NormalizeClusterName(name string) string {
	// Uppercase characters are not allowed, but keep word-structure via kebab-case.
	kebabCase := toKebabCase(name)
	// Only lowercase alphanumeric characters, hyphens and period are allowed.
	validName := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '.' {
			return r
		}
		return -1
	}, kebabCase)
	// We need to ensure this name is not too long.
	// https://github.com/kubernetes-sigs/kind/issues/623
	return validName[:min(len(validName), ClusterNameMaxLength)]
}

func toKebabCase(name string) string {
	kebabCase := regexpUpperCase.ReplaceAllStringFunc(name, func(s string) string {
		return "-" + strings.ToLower(s)
	})
	return strings.TrimPrefix(kebabCase, "-")
}

var regexpUpperCase = regexp.MustCompile("[A-Z]")

type ClusterCreateOpts struct {
	ClusterName string
	Config      string
}

// runForwarded runs the given command, redirecting output to stdout.
func runForwarded(cmd *exec.Cmd) error {
	fmt.Printf("$ %s\n", strings.Join(cmd.Args, " "))
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	} else {
		cmd.Stdout = io.MultiWriter(cmd.Stdout, os.Stdout)
	}
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stderr = io.MultiWriter(cmd.Stderr, os.Stderr)
	}
	return cmd.Run()
}

func ClusterCreate(clusterName, config string) error {
	args := []string{"create", "cluster", "--name", clusterName, "--kubeconfig", ""}
	if config != "" {
		args = append(args, "--config", config)
	}
	cmd := exec.Command("kind", args...)
	if err := runForwarded(cmd); err != nil {
		return fmt.Errorf("failed to run create cluster command: %w", err)
	}
	return nil
}

func ClusterDelete(clusterName string) error {
	args := []string{"delete", "cluster", "--name", clusterName}
	cmd := exec.Command("kind", args...)
	return runForwarded(cmd)
}

func ClusterNodes(clusterName string) ([]string, error) {
	args := []string{"get", "nodes", "--name", clusterName}
	cmd := exec.Command("kind", args...)
	out := strings.Builder{}
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	nodes := strings.TrimSpace(out.String())
	if nodes == "" || strings.Contains(nodes, "No kind nodes found") {
		return nil, nil
	}
	return strings.Split(nodes, "\n"), nil
}

func Clusters() ([]string, error) {
	args := []string{"get", "clusters"}
	cmd := exec.Command("kind", args...)
	out := strings.Builder{}
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	clusters := strings.TrimSpace(out.String())
	if clusters == "" || clusters == "No kind clusters found." {
		return nil, nil
	}
	return strings.Split(clusters, "\n"), nil
}

// dockerExec runs `docker exec` programmatically.
func dockerExec(ctx context.Context, client *dockerclient.Client, container string, cmd []string) error {
	exec, err := client.ContainerExecCreate(ctx, container, types.ExecConfig{
		Cmd: cmd,
	})
	if err != nil {
		return err
	}
	response, err := client.ContainerExecAttach(ctx, exec.ID, types.ExecStartCheck{})
	if err != nil {
		return err
	}
	defer response.Close()
	if err := client.ContainerExecStart(ctx, exec.ID, types.ExecStartCheck{}); err != nil {
		return err
	}
	_, err = io.ReadAll(response.Reader)
	if err != nil {
		return err
	}
	result, err := client.ContainerExecInspect(ctx, exec.ID)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("failed to run docker exec: exit code %d", result.ExitCode)
	}
	return nil
}

func ClusterAddRegistry(ctx context.Context, clusterName string, client *dockerclient.Client, registryName string, registryPort int) error {
	// https://kind.sigs.k8s.io/docs/user/local-registry/
	registryDir := fmt.Sprintf("/etc/containerd/certs.d/localhost:%d", registryPort)
	registryFile := fmt.Sprintf("%s/hosts.toml", registryDir)

	nodes, err := ClusterNodes(clusterName)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		if err := dockerExec(ctx, client, node, []string{"mkdir", "-p", registryDir}); err != nil {
			return fmt.Errorf("failed to create node %q registry directory: %w", node, err)
		}
		if err := dockerExec(ctx, client, node, []string{"sh", "-c", fmt.Sprintf("echo \"[host.\\\"http://%s:5000\\\"]\n\" > %s", registryName, registryFile)}); err != nil {
			return fmt.Errorf("failed to create node %q registry file: %w", node, err)
		}
	}

	container, err := client.ContainerInspect(ctx, registryName)
	if err != nil {
		return err
	}
	if _, ok := container.NetworkSettings.Networks["kind"]; !ok {
		if err := client.NetworkConnect(ctx, "kind", registryName, nil); err != nil {
			return err
		}
	}

	restConfig, err := ClusterRestConfig(clusterName)
	if err != nil {
		return err
	}
	kubeClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return err
	}
	configMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "local-registry-hosting",
			Namespace: "kube-public",
		},
		Data: map[string]string{
			"localRegistryHosting.v1": fmt.Sprintf(`host: "localhost:%d\nhelp: "https://kind.sigs.k8s.io/docs/user/local-registry/"`, registryPort),
		},
	}
	if _, err := kubeClient.CoreV1().ConfigMaps(configMap.GetNamespace()).Create(ctx, &configMap, metav1.CreateOptions{}); err != nil {
		return err
	}

	return nil
}

func ClusterRestConfig(clusterName string) (*rest.Config, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{
		Context: api.Context{
			Cluster: fmt.Sprintf("kind-%s", clusterName),
		},
	}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, configOverrides).ClientConfig()
}
