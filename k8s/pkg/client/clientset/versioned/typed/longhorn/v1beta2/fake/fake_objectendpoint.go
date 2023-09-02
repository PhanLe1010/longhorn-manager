/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"

	v1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeObjectEndpoints implements ObjectEndpointInterface
type FakeObjectEndpoints struct {
	Fake *FakeLonghornV1beta2
	ns   string
}

var objectendpointsResource = schema.GroupVersionResource{Group: "longhorn.io", Version: "v1beta2", Resource: "objectendpoints"}

var objectendpointsKind = schema.GroupVersionKind{Group: "longhorn.io", Version: "v1beta2", Kind: "ObjectEndpoint"}

// Get takes name of the objectEndpoint, and returns the corresponding objectEndpoint object, and an error if there is any.
func (c *FakeObjectEndpoints) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1beta2.ObjectEndpoint, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(objectendpointsResource, c.ns, name), &v1beta2.ObjectEndpoint{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta2.ObjectEndpoint), err
}

// List takes label and field selectors, and returns the list of ObjectEndpoints that match those selectors.
func (c *FakeObjectEndpoints) List(ctx context.Context, opts v1.ListOptions) (result *v1beta2.ObjectEndpointList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(objectendpointsResource, objectendpointsKind, c.ns, opts), &v1beta2.ObjectEndpointList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1beta2.ObjectEndpointList{ListMeta: obj.(*v1beta2.ObjectEndpointList).ListMeta}
	for _, item := range obj.(*v1beta2.ObjectEndpointList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested objectEndpoints.
func (c *FakeObjectEndpoints) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(objectendpointsResource, c.ns, opts))

}

// Create takes the representation of a objectEndpoint and creates it.  Returns the server's representation of the objectEndpoint, and an error, if there is any.
func (c *FakeObjectEndpoints) Create(ctx context.Context, objectEndpoint *v1beta2.ObjectEndpoint, opts v1.CreateOptions) (result *v1beta2.ObjectEndpoint, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(objectendpointsResource, c.ns, objectEndpoint), &v1beta2.ObjectEndpoint{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta2.ObjectEndpoint), err
}

// Update takes the representation of a objectEndpoint and updates it. Returns the server's representation of the objectEndpoint, and an error, if there is any.
func (c *FakeObjectEndpoints) Update(ctx context.Context, objectEndpoint *v1beta2.ObjectEndpoint, opts v1.UpdateOptions) (result *v1beta2.ObjectEndpoint, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(objectendpointsResource, c.ns, objectEndpoint), &v1beta2.ObjectEndpoint{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta2.ObjectEndpoint), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeObjectEndpoints) UpdateStatus(ctx context.Context, objectEndpoint *v1beta2.ObjectEndpoint, opts v1.UpdateOptions) (*v1beta2.ObjectEndpoint, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(objectendpointsResource, "status", c.ns, objectEndpoint), &v1beta2.ObjectEndpoint{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta2.ObjectEndpoint), err
}

// Delete takes name of the objectEndpoint and deletes it. Returns an error if one occurs.
func (c *FakeObjectEndpoints) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(objectendpointsResource, c.ns, name), &v1beta2.ObjectEndpoint{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeObjectEndpoints) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(objectendpointsResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1beta2.ObjectEndpointList{})
	return err
}

// Patch applies the patch and returns the patched objectEndpoint.
func (c *FakeObjectEndpoints) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1beta2.ObjectEndpoint, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(objectendpointsResource, c.ns, name, pt, data, subresources...), &v1beta2.ObjectEndpoint{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta2.ObjectEndpoint), err
}