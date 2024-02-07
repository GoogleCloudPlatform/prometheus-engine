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
	"context"
	"errors"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

func waitForResourceReady[T client.Object](ctx context.Context, kubeClient client.Client, o T, readyFn func(o T) error) error {
	var err error
	if waitErr := wait.PollUntilContextCancel(ctx, 3*time.Second, false, func(ctx context.Context) (bool, error) {
		if err = kubeClient.Get(ctx, client.ObjectKeyFromObject(o), o); err != nil {
			//nolint:nilerr // return nil to continue polling.
			return false, nil
		}
		if err = readyFn(o); err != nil {
			//nolint:nilerr // return nil to continue polling.
			return false, nil
		}
		return true, nil
	}); waitErr != nil {
		if errors.Is(waitErr, context.DeadlineExceeded) && err != nil {
			waitErr = err
		}
		gvk, err := apiutil.GVKForObject(o, kubeClient.Scheme())
		if err != nil {
			return fmt.Errorf("unable to get GVK: %w", err)
		}
		return fmt.Errorf("resource %s %s/%s not ready: %w", gvk.String(), o.GetNamespace(), o.GetName(), waitErr)
	}
	return nil
}
