// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kube

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ResourceFromYAML(scheme *runtime.Scheme, b []byte) (client.Object, error) {
	codec := serializer.NewCodecFactory(scheme)
	// Ignore returned schema. It's redundant since it's already encoded in obj.
	obj, _, err := codec.UniversalDeserializer().Decode(b, nil, nil)
	if err != nil {
		return nil, err
	}
	clientObj, ok := obj.(client.Object)
	if !ok {
		return nil, fmt.Errorf("unable to convert resource into client object: %s", obj.GetObjectKind().GroupVersionKind().String())
	}
	return clientObj, err
}

func ResourcesFromYAML(scheme *runtime.Scheme, b []byte) ([]client.Object, error) {
	var objs []client.Object
	var errs []error
	for i, doc := range strings.Split(string(b), "---") {
		obj, err := ResourceFromYAML(scheme, []byte(doc))
		if err != nil {
			// Ignore header comments.
			if i == 0 {
				node := yaml.Node{}
				if err := yaml.Unmarshal([]byte(doc), node); err != nil {
					errs = append(errs, fmt.Errorf("decode yaml %d: %w", i, err))
					continue
				}
				if node.IsZero() {
					continue
				}
			}

			errs = append(errs, fmt.Errorf("decode resource yaml %d: %w", i, err))
			continue
		}
		objs = append(objs, obj)
	}

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	return objs, nil
}
