package manager

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
)

func (m *VolumeManager) GetDisk(name string) (*longhorn.Disk, error) {
	return m.ds.GetDisk(name)
}

func (m *VolumeManager) ListDisks() (map[string]*longhorn.Disk, error) {
	diskMap, err := m.ds.ListDisks()
	if err != nil {
		return nil, err
	}
	return diskMap, nil
}

func (m *VolumeManager) ListDisksSorted() ([]*longhorn.Disk, error) {
	diskMap, err := m.ListDisks()
	if err != nil {
		return []*longhorn.Disk{}, err
	}

	disks := []*longhorn.Disk{}
	nodeDiskMap := map[string]map[string]*longhorn.Disk{}
	for _, disk := range diskMap {
		if nodeDiskMap[disk.Spec.NodeID] == nil {
			nodeDiskMap[disk.Spec.NodeID] = map[string]*longhorn.Disk{}
		}
		nodeDiskMap[disk.Spec.NodeID][disk.Name] = disk
	}
	sortedNodeNameList, err := sortKeys(nodeDiskMap)
	if err != nil {
		return []*longhorn.Disk{}, err
	}
	for _, nodeName := range sortedNodeNameList {
		sortedDiskNameList, err := sortKeys(nodeDiskMap[nodeName])
		if err != nil {
			return []*longhorn.Disk{}, err
		}
		for _, diskName := range sortedDiskNameList {
			disks = append(disks, diskMap[diskName])
		}
	}

	return disks, nil
}

func (m *VolumeManager) CreateDisk(diskManifest *longhorn.Disk) (*longhorn.Disk, error) {
	if err := m.validateDiskSpec(diskManifest); err != nil {
		return nil, err
	}

	if diskManifest.Spec.Path == "" {
		return nil, fmt.Errorf("invalid paramater: empty disk path")
	}

	if diskManifest.Spec.NodeID == "" {
		return nil, fmt.Errorf("invalid paramater: empty node ID")
	}
	node, err := m.GetNode(diskManifest.Spec.NodeID)
	if err != nil {
		return nil, err
	}
	readyCondition := types.GetCondition(node.Status.Conditions, types.NodeConditionTypeReady)
	if readyCondition.Status != types.ConditionStatusTrue {
		return nil, fmt.Errorf("node %v is not ready, couldn't add disk %v for it", node.Name, diskManifest.Name)
	}

	if diskManifest.Name == "" {
		diskManifest.Name = types.GenerateDiskName()
	}
	defer logrus.Infof("Create a new disk %v for node %v", diskManifest.Name, diskManifest.Spec.NodeID)

	return m.ds.CreateDisk(diskManifest)
}

func (m *VolumeManager) validateDiskSpec(d *longhorn.Disk) error {
	if d.Spec.StorageReserved < 0 {
		return fmt.Errorf("the reserved storage %v of disk %v(%v) is not valid, should be positive", d.Spec.StorageReserved, d.Name, d.Spec.Path)
	}

	tags, err := util.ValidateTags(d.Spec.Tags)
	if err != nil {
		return err
	}
	d.Spec.Tags = tags

	return nil
}

func (m *VolumeManager) DeleteDisk(name string) error {
	// only remove node from longhorn without any volumes on it
	disk, err := m.ds.GetDisk(name)
	if err != nil {
		return err
	}
	if disk.Status.State == types.DiskStateConnected && disk.Spec.AllowScheduling {
		return fmt.Errorf("need to disable the scheduling before deleting the connected disk")
	}

	if err := m.ds.DeleteDisk(name); err != nil {
		return err
	}
	logrus.Debugf("Deleted disk %v", name)
	return nil
}

func (m *VolumeManager) UpdateDisk(d *longhorn.Disk) (*longhorn.Disk, error) {
	if err := m.validateDiskSpec(d); err != nil {
		return nil, err
	}
	disk, err := m.ds.UpdateDisk(d)
	if err != nil {
		return nil, err
	}
	logrus.Debugf("Updated disk %v to %+v", disk.Name, disk.Spec)
	return disk, nil
}

func (m *VolumeManager) GetDiskTags() ([]string, error) {
	foundTags := make(map[string]struct{})
	var tags []string

	diskList, err := m.ds.ListDisks()
	if err != nil {
		return nil, errors.Wrapf(err, "fail to list disks")
	}
	for _, disk := range diskList {
		for _, tag := range disk.Spec.Tags {
			if _, ok := foundTags[tag]; !ok {
				foundTags[tag] = struct{}{}
				tags = append(tags, tag)
			}
		}
	}

	return tags, nil
}
