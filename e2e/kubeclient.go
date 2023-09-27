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
package e2e

import (
	"context"
	"fmt"

	"sync"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
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
type delegatingWriteClient[T WriterClient] struct {
	base   client.Client
	writer T
}

func (c *delegatingWriteClient[T]) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return c.base.Get(ctx, key, obj, opts...)
}

func (c *delegatingWriteClient[T]) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return c.base.List(ctx, list, opts...)
}

func (c *delegatingWriteClient[T]) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return c.writer.Create(ctx, obj, opts...)
}

func (c *delegatingWriteClient[T]) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return c.writer.Delete(ctx, obj, opts...)
}

func (c *delegatingWriteClient[T]) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return c.writer.Update(ctx, obj, opts...)
}

func (c *delegatingWriteClient[T]) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return c.writer.Patch(ctx, obj, patch, opts...)
}

func (c *delegatingWriteClient[T]) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return c.writer.DeleteAllOf(ctx, obj, opts...)
}

func (c *delegatingWriteClient[T]) SubResource(subResource string) client.SubResourceClient {
	return &delegatingWriteSubResourceClient{
		base:   c.base.SubResource(subResource),
		writer: c.writer.SubResource(subResource),
	}
}

func (c *delegatingWriteClient[T]) Status() client.SubResourceWriter {
	return c.writer.Status()
}

func (c *delegatingWriteClient[T]) Scheme() *runtime.Scheme {
	return c.base.Scheme()
}

func (c *delegatingWriteClient[T]) RESTMapper() meta.RESTMapper {
	return c.base.RESTMapper()
}

func (c *delegatingWriteClient[T]) Base() client.Client {
	return c.base
}

func newDelegatingWriteClient(c client.Client, w WriterClient) DelegatingClient {
	return &delegatingWriteClient[WriterClient]{
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

// TrackingClient is a Kubernetes client that tracks objects it created.
type TrackingClient interface {
	DelegatingClient
	// Cleanup deletes all objects created by this client.
	Cleanup(ctx context.Context) error
}

type trackingWriterClient struct {
	base    client.Client
	mutex   sync.Mutex
	objects map[schema.GroupVersionKind]map[client.ObjectKey]struct{}
}

func (c *trackingWriterClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if err := c.base.Create(ctx, obj, opts...); err != nil {
		return err
	}
	gvk, err := apiutil.GVKForObject(obj, c.base.Scheme())
	if err != nil {
		return err
	}
	key := client.ObjectKeyFromObject(obj)
	if _, ok := c.objects[gvk]; !ok {
		c.objects[gvk] = map[client.ObjectKey]struct{}{}
	}
	c.objects[gvk][key] = struct{}{}
	return nil
}

func (c *trackingWriterClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if err := c.base.Delete(ctx, obj, opts...); err != nil {
		return err
	}
	gvk, err := apiutil.GVKForObject(obj, c.base.Scheme())
	if err != nil {
		return err
	}
	key := client.ObjectKeyFromObject(obj)
	if _, ok := c.objects[gvk]; !ok {
		return fmt.Errorf("object type %s with key %s does not exist", gvk.String(), key)
	}
	delete(c.objects[gvk], key)
	return nil
}

func (c *trackingWriterClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return c.base.Update(ctx, obj, opts...)
}

func (c *trackingWriterClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return c.base.Patch(ctx, obj, patch, opts...)
}

func (c *trackingWriterClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	gvk, err := apiutil.GVKForObject(obj, c.base.Scheme())
	if err != nil {
		return err
	}
	objList := metav1.PartialObjectMetadataList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gvk.GroupVersion().String(),
			Kind:       gvk.Kind,
		},
	}
	deleteAllOfOptions := client.DeleteAllOfOptions{}
	for _, opt := range opts {
		opt.ApplyToDeleteAllOf(&deleteAllOfOptions)
	}
	if err := c.base.List(ctx, &objList, &deleteAllOfOptions.ListOptions); err != nil {
		return err
	}
	for _, obj := range objList.Items {
		if err := c.Delete(ctx, &obj, &deleteAllOfOptions.DeleteOptions); err != nil {
			return err
		}
	}
	return nil
}

func (c *trackingWriterClient) SubResource(subResource string) client.SubResourceWriter {
	return c.base.SubResource(subResource)
}

func (c *trackingWriterClient) Status() client.SubResourceWriter {
	return c.base.Status()
}

func (c *trackingWriterClient) Cleanup(ctx context.Context) error {
	for gvk, keys := range c.objects {
		for key := range keys {
			obj := metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{
					APIVersion: gvk.GroupVersion().String(),
					Kind:       gvk.Kind,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.Name,
					Namespace: key.Namespace,
				},
			}
			if err := c.base.Delete(ctx, &obj); err != nil {
				return err
			}
		}
	}
	return nil
}

type trackingClient struct {
	delegatingWriteClient[*trackingWriterClient]
}

func (c *trackingClient) Base() client.Client {
	return c.base
}

func (c *trackingClient) Cleanup(ctx context.Context) error {
	return c.writer.Cleanup(ctx)
}

func NewTrackingClient(c client.Client) TrackingClient {
	return &trackingClient{
		delegatingWriteClient: delegatingWriteClient[*trackingWriterClient]{
			base: c,
			writer: &trackingWriterClient{
				base:    c,
				objects: map[schema.GroupVersionKind]map[client.ObjectKey]struct{}{},
			},
		},
	}
}
