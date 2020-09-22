package v102to110

import (
	"fmt"
	"github.com/longhorn/longhorn-manager/util"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"

	"github.com/longhorn/longhorn-manager/types"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	lhclientset "github.com/longhorn/longhorn-manager/k8s/pkg/client/clientset/versioned"
)

// This upgrade is needed because we use separate the disk resources from node
// object (in v1.0.2) and use CRs to track them (in v1.1.0).
// Since the naming of the disk resources may change, the related replicas
// should be updated, too.
// Link to the original issue: https://github.com/longhorn/longhorn/issues/1269

const (
	upgradeLogPrefix = "upgrade from v1.0.2 to v1.1.0: "
)

func UpgradeCRDs(namespace string, lhClient *lhclientset.Clientset) error {
	if err := createDisksAndUpdateNodes(namespace, lhClient); err != nil {
		return err
	}
	if err := doReplicaUpgrade(namespace, lhClient); err != nil {
		return err
	}

	return nil
}

func createDisksAndUpdateNodes(namespace string, lhClient *lhclientset.Clientset) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade instance manager failed")
	}()

	nodeList, err := lhClient.LonghornV1beta1().Nodes(namespace).List(metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, upgradeLogPrefix+"failed to list all existing nodes during the upgrade")
	}

	for _, node := range nodeList.Items {
		if node.Spec.Disks == nil {
			continue
		}
		for diskID, diskSpec := range node.Spec.Disks {
			// Disks on different nodes may be using the same disk ID.
			// And the generated name for each node disk during the upgrade is fixed.
			diskName := generateFixedDiskNameForExistingDisks(node.Name, diskID)
			if _, err := lhClient.LonghornV1beta1().Disks(namespace).Get(diskName, metav1.GetOptions{}); err == nil {
				continue
			} else {
				if !apierrors.IsNotFound(err) {
					return errors.Wrapf(err, upgradeLogPrefix+"failed to get disk %v for node %v during the upgrade", diskName, node.Name)
				}
			}
			disk := &longhorn.Disk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      diskName,
					Namespace: node.Namespace,
					Labels: map[string]string{
						types.LonghornNodeKey: node.Name,
					},
				},
				Spec: types.DiskSpec{
					NodeID:            node.Name,
					Path:              diskSpec.Path,
					AllowScheduling:   diskSpec.AllowScheduling,
					EvictionRequested: diskSpec.EvictionRequested,
					StorageReserved:   diskSpec.StorageReserved,
					Tags:              diskSpec.Tags,
				},
			}
			if _, err := lhClient.LonghornV1beta1().Disks(namespace).Create(disk); err != nil {
				return errors.Wrapf(err, upgradeLogPrefix+"failed to create disk %v during the upgrade", diskName)
			}
		}

		node.Spec.Disks = nil
		node.Status.DiskStatus = nil
		if _, err := lhClient.LonghornV1beta1().Nodes(namespace).Update(&node); err != nil {
			return errors.Wrapf(err, upgradeLogPrefix+"failed to clean up the deprecated field node.Spec.Disks for node %v during the upgrade", node.Name)
		}
	}
	return nil
}

func getNodeLabelSelector(nodeName string) string {
	return fmt.Sprintf("%s=%s", types.LonghornNodeKey, nodeName)
}

func generateFixedDiskNameForExistingDisks(nodeName, diskID string) string {
	return fmt.Sprintf("%s-%s", diskID, util.GetStringChecksum(strings.TrimSpace(nodeName))[:8])
}

func doReplicaUpgrade(namespace string, lhClient *lhclientset.Clientset) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade instance manager failed")
	}()

	replicaList, err := lhClient.LonghornV1beta1().Replicas(namespace).List(metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, upgradeLogPrefix+"failed to list all existing replicas during the upgrade")
	}

	for _, replica := range replicaList.Items {
		if _, exist := replica.Labels[types.LonghornDiskKey]; exist {
			continue
		}
		replica.Spec.DiskID = generateFixedDiskNameForExistingDisks(replica.Spec.NodeID, replica.Spec.DiskID)
		replica.Labels[types.LonghornDiskKey] = replica.Spec.DiskID
		if _, err := lhClient.LonghornV1beta1().Replicas(namespace).Update(&replica); err != nil {
			return err
		}
	}
	return nil
}
