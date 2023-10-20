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

// Package e2e contains tests that validate the behavior of gmp-operator against a cluster.
// To make tests simple and fast, the test suite runs the operator internally. The CRDs
// are expected to be installed out of band (along with the operator deployment itself in
// a real world setup).
package kubeutil

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ParseResourceYAML(scheme *runtime.Scheme, b []byte) (client.Object, error) {
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

func ResourcesFromFile(scheme *runtime.Scheme, filepath string) ([]client.Object, error) {
	fileBytes, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("read YAML at path %q: %w", filepath, err)
	}

	var objs []client.Object
	var errs []error
	for i, doc := range strings.Split(string(fileBytes), "---") {
		obj, err := ParseResourceYAML(scheme, []byte(doc))
		if err != nil {
			errs = append(errs, fmt.Errorf("decode resource yaml %d: %w", i, err))
			continue
		}
		objs = append(objs, obj)
	}

	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("at path %q: %w", filepath, err)
	}

	return objs, nil
}

func ResourceFromFile(scheme *runtime.Scheme, filepath string) (client.Object, error) {
	objs, err := ResourcesFromFile(scheme, filepath)
	if err != nil {
		return nil, err
	}
	if len(objs) != 1 {
		return nil, fmt.Errorf("expected 1 resource but found %d at path %q", len(objs), filepath)
	}
	return objs[0], nil
}
