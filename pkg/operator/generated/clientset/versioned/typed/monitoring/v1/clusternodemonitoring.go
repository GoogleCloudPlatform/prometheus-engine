// Copyright 2022 Google LLC
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

// Code generated by client-gen. DO NOT EDIT.

package v1

import (
	"context"
	"time"

	v1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	scheme "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/generated/clientset/versioned/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// ClusterNodeMonitoringsGetter has a method to return a ClusterNodeMonitoringInterface.
// A group's client should implement this interface.
type ClusterNodeMonitoringsGetter interface {
	ClusterNodeMonitorings() ClusterNodeMonitoringInterface
}

// ClusterNodeMonitoringInterface has methods to work with ClusterNodeMonitoring resources.
type ClusterNodeMonitoringInterface interface {
	Create(ctx context.Context, clusterNodeMonitoring *v1.ClusterNodeMonitoring, opts metav1.CreateOptions) (*v1.ClusterNodeMonitoring, error)
	Update(ctx context.Context, clusterNodeMonitoring *v1.ClusterNodeMonitoring, opts metav1.UpdateOptions) (*v1.ClusterNodeMonitoring, error)
	UpdateStatus(ctx context.Context, clusterNodeMonitoring *v1.ClusterNodeMonitoring, opts metav1.UpdateOptions) (*v1.ClusterNodeMonitoring, error)
	Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*v1.ClusterNodeMonitoring, error)
	List(ctx context.Context, opts metav1.ListOptions) (*v1.ClusterNodeMonitoringList, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v1.ClusterNodeMonitoring, err error)
	ClusterNodeMonitoringExpansion
}

// clusterNodeMonitorings implements ClusterNodeMonitoringInterface
type clusterNodeMonitorings struct {
	client rest.Interface
}

// newClusterNodeMonitorings returns a ClusterNodeMonitorings
func newClusterNodeMonitorings(c *MonitoringV1Client) *clusterNodeMonitorings {
	return &clusterNodeMonitorings{
		client: c.RESTClient(),
	}
}

// Get takes name of the clusterNodeMonitoring, and returns the corresponding clusterNodeMonitoring object, and an error if there is any.
func (c *clusterNodeMonitorings) Get(ctx context.Context, name string, options metav1.GetOptions) (result *v1.ClusterNodeMonitoring, err error) {
	result = &v1.ClusterNodeMonitoring{}
	err = c.client.Get().
		Resource("clusternodemonitorings").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do(ctx).
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of ClusterNodeMonitorings that match those selectors.
func (c *clusterNodeMonitorings) List(ctx context.Context, opts metav1.ListOptions) (result *v1.ClusterNodeMonitoringList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1.ClusterNodeMonitoringList{}
	err = c.client.Get().
		Resource("clusternodemonitorings").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do(ctx).
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested clusterNodeMonitorings.
func (c *clusterNodeMonitorings) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	return c.client.Get().
		Resource("clusternodemonitorings").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch(ctx)
}

// Create takes the representation of a clusterNodeMonitoring and creates it.  Returns the server's representation of the clusterNodeMonitoring, and an error, if there is any.
func (c *clusterNodeMonitorings) Create(ctx context.Context, clusterNodeMonitoring *v1.ClusterNodeMonitoring, opts metav1.CreateOptions) (result *v1.ClusterNodeMonitoring, err error) {
	result = &v1.ClusterNodeMonitoring{}
	err = c.client.Post().
		Resource("clusternodemonitorings").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(clusterNodeMonitoring).
		Do(ctx).
		Into(result)
	return
}

// Update takes the representation of a clusterNodeMonitoring and updates it. Returns the server's representation of the clusterNodeMonitoring, and an error, if there is any.
func (c *clusterNodeMonitorings) Update(ctx context.Context, clusterNodeMonitoring *v1.ClusterNodeMonitoring, opts metav1.UpdateOptions) (result *v1.ClusterNodeMonitoring, err error) {
	result = &v1.ClusterNodeMonitoring{}
	err = c.client.Put().
		Resource("clusternodemonitorings").
		Name(clusterNodeMonitoring.Name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(clusterNodeMonitoring).
		Do(ctx).
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *clusterNodeMonitorings) UpdateStatus(ctx context.Context, clusterNodeMonitoring *v1.ClusterNodeMonitoring, opts metav1.UpdateOptions) (result *v1.ClusterNodeMonitoring, err error) {
	result = &v1.ClusterNodeMonitoring{}
	err = c.client.Put().
		Resource("clusternodemonitorings").
		Name(clusterNodeMonitoring.Name).
		SubResource("status").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(clusterNodeMonitoring).
		Do(ctx).
		Into(result)
	return
}

// Delete takes name of the clusterNodeMonitoring and deletes it. Returns an error if one occurs.
func (c *clusterNodeMonitorings) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return c.client.Delete().
		Resource("clusternodemonitorings").
		Name(name).
		Body(&opts).
		Do(ctx).
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *clusterNodeMonitorings) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	var timeout time.Duration
	if listOpts.TimeoutSeconds != nil {
		timeout = time.Duration(*listOpts.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Resource("clusternodemonitorings").
		VersionedParams(&listOpts, scheme.ParameterCodec).
		Timeout(timeout).
		Body(&opts).
		Do(ctx).
		Error()
}

// Patch applies the patch and returns the patched clusterNodeMonitoring.
func (c *clusterNodeMonitorings) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v1.ClusterNodeMonitoring, err error) {
	result = &v1.ClusterNodeMonitoring{}
	err = c.client.Patch(pt).
		Resource("clusternodemonitorings").
		Name(name).
		SubResource(subresources...).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return
}
