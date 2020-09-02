package controller

import (
	"math/rand"
	"strconv"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/kubernetes/pkg/controller"

	"github.com/longhorn/longhorn-manager/types"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"

	. "gopkg.in/check.v1"
)

const (
	TestNamespace                 = "default"
	TestIP1                       = "1.2.3.4"
	TestIP2                       = "5.6.7.8"
	TestPort1                     = 9501
	TestNode1                     = "test-node-name-1"
	TestNode2                     = "test-node-name-2"
	TestDisk1                     = "test-disk-name-1"
	TestDisk2                     = "test-disk-name-2"
	TestOwnerID1                  = TestNode1
	TestOwnerID2                  = TestNode2
	TestEngineImage               = "longhorn-engine:latest"
	TestUpgradedEngineImage       = "longhorn-engine:upgraded"
	TestInstanceManagerImage      = "longhorn-instance-manager:latest"
	TestExtraInstanceManagerImage = "longhorn-instance-manager:upgraded"
	TestManagerImage              = "longhorn-manager:latest"
	TestServiceAccount            = "longhorn-service-account"

	TestInstanceManagerName1 = "instance-manager-engine-image-name-1"
	TestEngineManagerName    = "instance-manager-e-test-name"
	TestReplicaManagerName   = "instance-manager-r-test-name"

	TestPod1 = "test-pod-name-1"
	TestPod2 = "test-pod-name-2"

	TestVolumeName         = "test-volume"
	TestVolumeSize         = 1073741824
	TestVolumeStaleTimeout = 60
	TestEngineName         = "test-volume-engine"

	TestPVName  = "test-pv"
	TestPVCName = "test-pvc"

	TestVAName = "test-volume-attachment"

	TestTimeNow = "2015-01-02T00:00:00Z"

	TestDefaultDataPath   = "/var/lib/longhorn"
	TestInvalidDataPath   = "/invalid"
	TestDaemon1           = "longhorn-manager-1"
	TestDaemon2           = "longhorn-manager-2"
	TestDiskSize          = 5000000000
	TestDiskAvailableSize = 3000000000
	TestDefaultDiskUUID   = "default-disk-uuid"
	TestDefaultDiskFSID   = "default-disk-fsid"

	TestBackupTarget     = "s3://backupbucket@us-east-1/backupstore"
	TestBackupVolumeName = "test-backup-volume-for-restoration"
	TestBackupName       = "test-backup-for-restoration"
)

var (
	alwaysReady = func() bool { return true }
)

func Test(t *testing.T) { TestingT(t) }

type TestSuite struct {
}

var _ = Suite(&TestSuite{})

func (s *TestSuite) SetUpTest(c *C) {
	logrus.SetLevel(logrus.DebugLevel)
}

func newNode(name, namespace string, allowScheduling bool, status types.ConditionStatus, reason string) *longhorn.Node {
	return &longhorn.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: types.NodeSpec{
			AllowScheduling: allowScheduling,
		},
		Status: types.NodeStatus{
			Conditions: map[string]types.Condition{
				types.NodeConditionTypeSchedulable: newNodeCondition(types.NodeConditionTypeSchedulable, status, reason),
				types.NodeConditionTypeReady:       newNodeCondition(types.NodeConditionTypeReady, status, reason),
			},
		},
	}
}

func newNodeCondition(conditionType string, status types.ConditionStatus, reason string) types.Condition {
	return types.Condition{
		Type:    conditionType,
		Status:  status,
		Reason:  reason,
		Message: "",
	}
}

func newDisk(name, namespace, nodeID string) *longhorn.Disk {
	return &longhorn.Disk{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				types.LonghornNodeKey: nodeID,
			},
		},
		Spec: types.DiskSpec{
			Path:              TestDefaultDataPath,
			AllowScheduling:   true,
			EvictionRequested: false,
			StorageReserved:   0,
			NodeID:            nodeID,
		},
		Status: types.DiskStatus{
			OwnerID:          nodeID,
			StorageAvailable: TestDiskAvailableSize,
			StorageScheduled: 0,
			StorageMaximum:   TestDiskSize,
			Conditions: map[string]types.Condition{
				types.DiskConditionTypeReady:       newNodeCondition(types.DiskConditionTypeReady, types.ConditionStatusTrue, ""),
				types.DiskConditionTypeSchedulable: newNodeCondition(types.DiskConditionTypeSchedulable, types.ConditionStatusTrue, ""),
			},
			ScheduledReplica: map[string]int64{},
			DiskUUID:         TestDefaultDiskUUID,
			FSID:             TestDefaultDiskFSID,
			State:            types.DiskStateConnected,
		},
	}
}

func newKubernetesNode(name string, readyStatus, diskPressureStatus, memoryStatus, outOfDiskStatus, pidStatus, networkStatus, kubeletStatus corev1.ConditionStatus) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: readyStatus,
				},
				{
					Type:   corev1.NodeDiskPressure,
					Status: diskPressureStatus,
				},
				{
					Type:   corev1.NodeMemoryPressure,
					Status: memoryStatus,
				},
				{
					Type:   corev1.NodeOutOfDisk,
					Status: outOfDiskStatus,
				},
				{
					Type:   corev1.NodePIDPressure,
					Status: pidStatus,
				},
				{
					Type:   corev1.NodeNetworkUnavailable,
					Status: networkStatus,
				},
			},
		},
	}
}

func newDaemonPod(phase corev1.PodPhase, name, namespace, nodeID, podIP string, mountpropagation *corev1.MountPropagationMode) *corev1.Pod {
	podStatus := corev1.ConditionFalse
	if phase == corev1.PodRunning {
		podStatus = corev1.ConditionTrue
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app": "longhorn-manager",
			},
		},
		Spec: corev1.PodSpec{
			NodeName: nodeID,
			Containers: []corev1.Container{
				{
					Name:  "test-container",
					Image: TestEngineImage,
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:             "longhorn",
							MountPath:        TestDefaultDataPath,
							MountPropagation: mountpropagation,
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: podStatus,
				},
			},
			Phase: phase,
			PodIP: podIP,
		},
	}
}

func newSetting(name, value string) *longhorn.Setting {
	return &longhorn.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: TestNamespace,
		},
		Setting: types.Setting{
			Value: value,
		},
	}
}

func newDefaultInstanceManagerImageSetting() *longhorn.Setting {
	return &longhorn.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: string(types.SettingNameDefaultInstanceManagerImage),
		},
		Setting: types.Setting{
			Value: TestInstanceManagerImage,
		},
	}
}

func newEngineImage(image string, state types.EngineImageState) *longhorn.EngineImage {
	return &longhorn.EngineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:       types.GetEngineImageChecksumName(image),
			Namespace:  TestNamespace,
			UID:        uuid.NewUUID(),
			Finalizers: []string{longhornFinalizerKey},
		},
		Spec: types.EngineImageSpec{
			Image: image,
		},
		Status: types.EngineImageStatus{
			OwnerID: TestNode1,
			State:   state,
			EngineVersionDetails: types.EngineVersionDetails{
				Version:   "latest",
				GitCommit: "latest",

				CLIAPIVersion:           4,
				CLIAPIMinVersion:        3,
				ControllerAPIVersion:    3,
				ControllerAPIMinVersion: 3,
				DataFormatVersion:       1,
				DataFormatMinVersion:    1,
			},
			Conditions: map[string]types.Condition{
				types.EngineImageConditionTypeReady: {
					Type:   types.EngineImageConditionTypeReady,
					Status: types.ConditionStatusTrue,
				},
			},
		},
	}
}

func newEngineImageDaemonSet() *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getTestEngineImageDaemonSetName(),
			Namespace: TestNamespace,
			Labels:    types.GetEngineImageLabels(getTestEngineImageName()),
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: types.GetEngineImageLabels(getTestEngineImageName()),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:   getTestEngineImageDaemonSetName(),
					Labels: types.GetEngineImageLabels(getTestEngineImageName()),
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: TestServiceAccount,
					Containers: []corev1.Container{
						{
							Name:  getTestEngineImageDaemonSetName(),
							Image: TestEngineImage,
						},
					},
				},
			},
		},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 1,
			NumberAvailable:        1,
		},
	}
}

func getKey(obj interface{}, c *C) string {
	key, err := controller.KeyFunc(obj)
	c.Assert(err, IsNil)
	return key
}

func getOwnerReference(obj runtime.Object) *metav1.OwnerReference {
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return nil
	}
	return &metadata.GetOwnerReferences()[0]
}

func getTestNow() string {
	return TestTimeNow
}

func randomIP() string {
	b := []string{}
	for i := 0; i < 4; i++ {
		b = append(b, strconv.Itoa(int(rand.Uint32()%255)))
	}
	return strings.Join(b, ".")
}

func getTestEngineImageName() string {
	return types.GetEngineImageChecksumName(TestEngineImage)
}

func getTestEngineImageDaemonSetName() string {
	return types.GetDaemonSetNameFromEngineImageName(types.GetEngineImageChecksumName(TestEngineImage))
}

func randomPort() int {
	return rand.Int() % 30000
}

func fakeEngineBinaryChecker(image string) bool {
	return true
}

func fakeEngineImageUpdater(ei *longhorn.EngineImage) error {
	return nil
}

func (s *TestSuite) TestIsSameGuaranteedCPURequirement(c *C) {
	var (
		a, b *corev1.ResourceRequirements
		err  error
	)

	c.Assert(IsSameGuaranteedCPURequirement(a, b), Equals, true)

	b = &corev1.ResourceRequirements{}
	c.Assert(IsSameGuaranteedCPURequirement(a, b), Equals, true)

	b.Requests = corev1.ResourceList{}
	c.Assert(IsSameGuaranteedCPURequirement(a, b), Equals, true)

	b.Requests[corev1.ResourceCPU], err = resource.ParseQuantity("0")
	c.Assert(err, IsNil)
	c.Assert(IsSameGuaranteedCPURequirement(a, b), Equals, true)

	b.Requests[corev1.ResourceCPU], err = resource.ParseQuantity("0m")
	c.Assert(err, IsNil)
	c.Assert(IsSameGuaranteedCPURequirement(a, b), Equals, true)

	a = &corev1.ResourceRequirements{}
	c.Assert(IsSameGuaranteedCPURequirement(a, b), Equals, true)

	a.Requests = corev1.ResourceList{}
	c.Assert(IsSameGuaranteedCPURequirement(a, b), Equals, true)

	a.Requests[corev1.ResourceCPU], err = resource.ParseQuantity("0")
	c.Assert(err, IsNil)
	c.Assert(IsSameGuaranteedCPURequirement(a, b), Equals, true)

	a.Requests[corev1.ResourceCPU], err = resource.ParseQuantity("0m")
	c.Assert(err, IsNil)
	c.Assert(IsSameGuaranteedCPURequirement(a, b), Equals, true)

	b.Requests[corev1.ResourceCPU], err = resource.ParseQuantity("250m")
	a = &corev1.ResourceRequirements{}
	c.Assert(IsSameGuaranteedCPURequirement(a, b), Equals, false)

	b.Requests[corev1.ResourceCPU], err = resource.ParseQuantity("250m")
	a.Requests = corev1.ResourceList{}
	a.Requests[corev1.ResourceCPU], err = resource.ParseQuantity("0.25")
	c.Assert(IsSameGuaranteedCPURequirement(a, b), Equals, true)
}
