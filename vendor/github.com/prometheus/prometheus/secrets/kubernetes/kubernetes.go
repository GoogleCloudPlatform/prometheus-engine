// Copyright 2023 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kubernetes

import (
	"fmt"

	"github.com/prometheus/common/config"
	"github.com/prometheus/common/version"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	// Http header
	userAgent = fmt.Sprintf("Prometheus/%s", version.Version)
)

type SecretConfig struct {
	Namespace string `yaml:"namespace"`
	Name      string `yaml:"name"`
	Key       string `yaml:"key"`
}

func (c *SecretConfig) objectKey() types.NamespacedName {
	return types.NamespacedName{
		Namespace: c.Namespace,
		Name:      c.Name,
	}
}

func getKey(s *corev1.Secret, key string) (string, error) {
	if value, ok := s.Data[key]; ok {
		return string(value), nil
	}
	if value, ok := s.StringData[key]; ok {
		return value, nil
	}
	return "", fmt.Errorf("secret %s/%s does not contain key: %s", s.Namespace, s.Name, key)
}

type ClientConfig struct {
	APIServer  config.URL `yaml:"api_server,omitempty"`
	KubeConfig string     `yaml:"kubeconfig_file,omitempty"`
}

// New creates a new Kubernetes discovery for the given role.
func (c *ClientConfig) client() (kubernetes.Interface, error) {
	var restClient *rest.Config
	switch {
	case c.KubeConfig != "":
		var err error
		restClient, err = clientcmd.BuildConfigFromFlags("", c.KubeConfig)
		if err != nil {
			return nil, err
		}
	case c.APIServer.URL == nil:
		var err error
		restClient, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
	}

	restClient.UserAgent = userAgent
	restClient.ContentType = "application/vnd.kubernetes.protobuf"

	return kubernetes.NewForConfig(restClient)
}
