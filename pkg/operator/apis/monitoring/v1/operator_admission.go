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

package v1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type OperatorConfigValidator struct {
	Namespace    string
	Name         string
	VPAAvailable bool
}

func (v *OperatorConfigValidator) ValidateCreate(_ context.Context, o runtime.Object) (admission.Warnings, error) {
	oc := o.(*OperatorConfig)

	if oc.Namespace != v.Namespace || oc.Name != v.Name {
		return nil, fmt.Errorf("OperatorConfig must be in namespace %q with name %q", v.Namespace, v.Name)
	}
	if oc.Scaling.VPA.Enabled && !v.VPAAvailable {
		return nil, fmt.Errorf("vertical pod autoscaling is not available - install vpa support and restart the operator")
	}
	return nil, oc.Validate()
}

func (v *OperatorConfigValidator) ValidateUpdate(ctx context.Context, _, o runtime.Object) (admission.Warnings, error) {
	return v.ValidateCreate(ctx, o)
}

func (v *OperatorConfigValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}
