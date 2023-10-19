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
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type DelegatingClient interface {
	client.Client
	Base() client.Client
}

// delegatingWriteSubResourceClient delegates all mutating functions to a writer client.
type delegatingWriteSubResourceClient struct {
	base   client.SubResourceClient
	writer client.SubResourceWriter
}

func (c *delegatingWriteSubResourceClient) Get(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceGetOption) error {
	return c.base.Get(ctx, obj, subResource, opts...)
}

func (c *delegatingWriteSubResourceClient) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return c.writer.Create(ctx, obj, subResource, opts...)
}

func (c *delegatingWriteSubResourceClient) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	return c.writer.Update(ctx, obj, opts...)
}

func (c *delegatingWriteSubResourceClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return c.writer.Patch(ctx, obj, patch, opts...)
}

// WriterClient comprises of all mutating functions in client.Client.
type WriterClient interface {
	client.Writer
	client.StatusClient
	SubResource(subResource string) client.SubResourceWriter
}

// delegatingWriteClient delegates all mutating functions to a writer client.
type delegatingWriteClient struct {
	base   client.Client
	writer WriterClient
}

func (c *delegatingWriteClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return c.base.Get(ctx, key, obj, opts...)
}

func (c *delegatingWriteClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return c.base.List(ctx, list, opts...)
}

func (c *delegatingWriteClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return c.writer.Create(ctx, obj, opts...)
}

func (c *delegatingWriteClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return c.writer.Delete(ctx, obj, opts...)
}

func (c *delegatingWriteClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return c.writer.Update(ctx, obj, opts...)
}

func (c *delegatingWriteClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return c.writer.Patch(ctx, obj, patch, opts...)
}

func (c *delegatingWriteClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return c.writer.DeleteAllOf(ctx, obj, opts...)
}

func (c *delegatingWriteClient) SubResource(subResource string) client.SubResourceClient {
	return &delegatingWriteSubResourceClient{
		base:   c.base.SubResource(subResource),
		writer: c.writer.SubResource(subResource),
	}
}

func (c *delegatingWriteClient) Status() client.SubResourceWriter {
	return c.writer.Status()
}

func (c *delegatingWriteClient) Scheme() *runtime.Scheme {
	return c.base.Scheme()
}

func (c *delegatingWriteClient) RESTMapper() meta.RESTMapper {
	return c.base.RESTMapper()
}

func (c *delegatingWriteClient) Base() client.Client {
	return c.base
}

func newDelegatingWriteClient(c client.Client, w WriterClient) DelegatingClient {
	return &delegatingWriteClient{
		base:   c,
		writer: w,
	}
}

// labelWriterClient adds common labels to all objects mutated by this client.
type labelWriterClient struct {
	base   client.Client
	labels map[string]string
}

func (c *labelWriterClient) setLabels(obj client.Object) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	for k, v := range c.labels {
		labels[k] = v
	}
	obj.SetLabels(labels)
}

func (c *labelWriterClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	c.setLabels(obj)
	return c.base.Create(ctx, obj, opts...)
}

func (c *labelWriterClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return c.base.Delete(ctx, obj, opts...)
}

func (c *labelWriterClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	c.setLabels(obj)
	return c.base.Update(ctx, obj, opts...)
}

func (c *labelWriterClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	// Ensure the patch doesn't remove the labels by applying the new patch.
	latestObj := obj.DeepCopyObject().(client.Object)
	if err := c.base.Get(ctx, client.ObjectKeyFromObject(obj), latestObj); err != nil {
		return err
	}
	fakeClient := fake.
		NewClientBuilder().
		WithRESTMapper(c.base.RESTMapper()).
		WithScheme(c.base.Scheme()).
		WithObjects(latestObj).
		Build()
	if err := fakeClient.Patch(ctx, obj, patch, opts...); err != nil {
		return err
	}
	return c.Update(ctx, obj)
}

func (c *labelWriterClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return c.base.DeleteAllOf(ctx, obj, opts...)
}

func (c *labelWriterClient) SubResource(subResource string) client.SubResourceWriter {
	return c.base.SubResource(subResource)
}

func (c *labelWriterClient) Status() client.SubResourceWriter {
	return c.base.Status()
}

func NewLabelWriterClient(c client.Client, labels map[string]string) DelegatingClient {
	return newDelegatingWriteClient(c, &labelWriterClient{
		base:   c,
		labels: labels,
	})
}
