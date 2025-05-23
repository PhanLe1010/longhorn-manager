/*
Copyright The Longhorn Authors.

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

// Code generated by lister-gen. DO NOT EDIT.

package v1beta2

import (
	longhornv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	labels "k8s.io/apimachinery/pkg/labels"
	listers "k8s.io/client-go/listers"
	cache "k8s.io/client-go/tools/cache"
)

// VolumeAttachmentLister helps list VolumeAttachments.
// All objects returned here must be treated as read-only.
type VolumeAttachmentLister interface {
	// List lists all VolumeAttachments in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*longhornv1beta2.VolumeAttachment, err error)
	// VolumeAttachments returns an object that can list and get VolumeAttachments.
	VolumeAttachments(namespace string) VolumeAttachmentNamespaceLister
	VolumeAttachmentListerExpansion
}

// volumeAttachmentLister implements the VolumeAttachmentLister interface.
type volumeAttachmentLister struct {
	listers.ResourceIndexer[*longhornv1beta2.VolumeAttachment]
}

// NewVolumeAttachmentLister returns a new VolumeAttachmentLister.
func NewVolumeAttachmentLister(indexer cache.Indexer) VolumeAttachmentLister {
	return &volumeAttachmentLister{listers.New[*longhornv1beta2.VolumeAttachment](indexer, longhornv1beta2.Resource("volumeattachment"))}
}

// VolumeAttachments returns an object that can list and get VolumeAttachments.
func (s *volumeAttachmentLister) VolumeAttachments(namespace string) VolumeAttachmentNamespaceLister {
	return volumeAttachmentNamespaceLister{listers.NewNamespaced[*longhornv1beta2.VolumeAttachment](s.ResourceIndexer, namespace)}
}

// VolumeAttachmentNamespaceLister helps list and get VolumeAttachments.
// All objects returned here must be treated as read-only.
type VolumeAttachmentNamespaceLister interface {
	// List lists all VolumeAttachments in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*longhornv1beta2.VolumeAttachment, err error)
	// Get retrieves the VolumeAttachment from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*longhornv1beta2.VolumeAttachment, error)
	VolumeAttachmentNamespaceListerExpansion
}

// volumeAttachmentNamespaceLister implements the VolumeAttachmentNamespaceLister
// interface.
type volumeAttachmentNamespaceLister struct {
	listers.ResourceIndexer[*longhornv1beta2.VolumeAttachment]
}
