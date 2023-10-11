// Copyright 2023 Google LLC
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

package e2e

import (
	"fmt"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
)

type ClusterMeta struct {
	ProjectID string
	Cluster   string
	Location  string
}

// ExtractGKEClusterMeta extracts the current GKE cluster meta-data using the local Kubernetes config.
func ExtractGKEClusterMeta() (ClusterMeta, error) {
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{
			CurrentContext: "",
		}).RawConfig()
	if err != nil {
		return ClusterMeta{}, err
	}
	prefix := "gke_"
	if !strings.HasPrefix(config.CurrentContext, prefix) {
		return ClusterMeta{}, fmt.Errorf("context is not GKE context: %s", config.CurrentContext)
	}
	// Google Cloud Projects, GKE cluster and locations don't allow underscores.
	context := strings.SplitN(config.CurrentContext[len(prefix):], "_", 3)
	return ClusterMeta{
		ProjectID: context[0],
		Cluster:   context[2],
		Location:  context[1],
	}, nil
}
