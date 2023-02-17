package v1beta2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AttachmentSpec struct {
	// The unique ID of this attachment. Used to differentiate different attachments of the same volume.
	// +optional
	ID string `json:"id"`
	// +optional
	Type AttacherType `json:"type"`
	// The node that this attachment is requesting
	// +optional
	NodeID string `json:"nodeID"`
	// Optional additional parameter for this attachment
	// +optional
	Parameters map[string]string `json:"parameters"`
	//// +optional
	//Attached *bool `json:"attached,omitempty"`
	//// +optional
	//AttachError *VolumeError `json:"attachError,omitempty"`
	//// +optional
	//DetachError *VolumeError `json:"detachError,omitempty"`
	// A sequence number representing a specific generation of the desired state.
	// Populated by the system. Read-only.
	// +optional
	Generation int64 `json:"generation"`
}

type AttachmentStatus struct {
	// The unique ID of this attachment. Used to differentiate different attachments of the same volume.
	// +optional
	ID string `json:"id"`
	// Indicate whether this attachment ticket has been satisfied
	Satisfied bool `json:"satisfied"`
	// Record any error when trying to fulfill this attachment
	// +nullable
	Conditions []Condition `json:"conditions"`
	// A sequence number representing a specific generation of the desired state.
	// Populated by the system. Read-only.
	// +optional
	Generation int64 `json:"generation"`
}

// VolumeError captures an error encountered during a volume operation.
type VolumeError struct {
	// Time the error was encountered.
	// +optional
	Time metav1.Time `json:"time,omitempty"`

	// String detailing the error encountered during Attach or Detach operation.
	// This string may be logged, so it should not contain sensitive
	// information.
	// +optional
	Message string `json:"message,omitempty"`
}

type AttacherType string

const (
	AttacherTypeCSIAttacher                      = AttacherType("csi-attacher")
	AttacherTypeLonghornAPI                      = AttacherType("longhorn-api")
	AttacherTypeSnapshotController               = AttacherType("snapshot-controller")
	AttacherTypeBackupController                 = AttacherType("backup-controller")
	AttacherTypeVolumeCloneController            = AttacherType("volume-clone-controller")
	AttacherTypeSalvageController                = AttacherType("salvage-controller")
	AttacherTypeShareManagerController           = AttacherType("share-manager-controller")
	AttacherTypeLiveMigrationController          = AttacherType("live-migration-controller")
	AttacherTypeVolumeRestoreController          = AttacherType("volume-restore-controller")
	AttacherTypeVolumeEvictionController         = AttacherType("volume-eviction-controller")
	AttacherTypeVolumeExpansionController        = AttacherType("volume-expansion-controller")
	AttacherTypeBackingImageDataSourceController = AttacherType("bim-ds-controller")
	AttacherTypeLonghornUpgrader                 = AttacherType("longhorn-upgrader")
)

const (
	AttacherPriorityLevelVolumeRestoreController          = 2000
	AttacherPriorityLevelVolumeExpansionController        = 2000
	AttacherPriorityLevelLonghornAPI                      = 1000
	AttacherPriorityLevelCSIAttacher                      = 900
	AttacherPriorityLevelSalvageController                = 900
	AttacherPriorityLevelShareManagerController           = 900
	AttacherPriorityLevelLonghornUpgrader                 = 900
	AttacherPriorityLevelLiveMigrationController          = 800
	AttacherPriorityLevelSnapshotController               = 800
	AttacherPriorityLevelBackupController                 = 800
	AttacherPriorityLevelVolumeCloneController            = 800
	AttacherPriorityLevelVolumeEvictionController         = 800
	AttacherPriorityLevelBackingImageDataSourceController = 800
)

const (
	TrueValue  = "true"
	FalseValue = "false"
	AnyValue   = "any"

	AttachmentParameterDisableFrontend = "disableFrontend"
	AttachmentParameterLastAttachedBy  = "lastAttachedBy"
)

const (
	AttachmentStatusConditionTypeSatisfied = "Satisfied"
)

func GetAttacherPriorityLevel(t AttacherType) int {
	switch t {
	case AttacherTypeCSIAttacher:
		return AttacherPriorityLevelCSIAttacher
	case AttacherTypeLonghornAPI:
		return AttacherPriorityLevelLonghornAPI
	case AttacherTypeSnapshotController:
		return AttacherPriorityLevelSnapshotController
	case AttacherTypeBackupController:
		return AttacherPriorityLevelBackupController
	case AttacherTypeVolumeCloneController:
		return AttacherPriorityLevelVolumeCloneController
	case AttacherTypeSalvageController:
		return AttacherPriorityLevelSalvageController
	case AttacherTypeShareManagerController:
		return AttacherPriorityLevelShareManagerController
	case AttacherTypeLiveMigrationController:
		return AttacherPriorityLevelLiveMigrationController
	case AttacherTypeLonghornUpgrader:
		return AttacherPriorityLevelLonghornUpgrader
	case AttacherTypeVolumeRestoreController:
		return AttacherPriorityLevelVolumeRestoreController
	case AttacherTypeVolumeEvictionController:
		return AttacherPriorityLevelVolumeEvictionController
	case AttacherTypeVolumeExpansionController:
		return AttacherPriorityLevelVolumeExpansionController
	case AttacherTypeBackingImageDataSourceController:
		return AttacherPriorityLevelBackingImageDataSourceController
	default:
		return 0
	}
}

func GetAttachmentID(attacherType AttacherType, id string) string {
	retID := string(attacherType) + "-" + id
	if len(retID) > 253 {
		return retID[:253]
	}
	return retID
}

func IsAttachmentSatisfied(attachmentID string, va *VolumeAttachment) bool {
	if va == nil {
		return false
	}
	attachmentSpec, ok := va.Spec.AttachmentSpecs[attachmentID]
	if !ok {
		return false
	}
	attachmentStatus, ok := va.Status.AttachmentStatuses[attachmentID]
	if !ok {
		return false
	}

	return attachmentSpec.Generation == attachmentStatus.Generation && attachmentStatus.Satisfied
}

// VolumeAttachmentSpec defines the desired state of Longhorn VolumeAttachment
type VolumeAttachmentSpec struct {
	// +optional
	// TODO: maybe AttachmentRequest
	AttachmentSpecs map[string]*AttachmentSpec `json:"attachmentSpecs"`
	// The name of Longhorn volume of this VolumeAttachment
	Volume string `json:"volume"`
}

// VolumeAttachmentStatus defines the observed state of Longhorn VolumeAttachment
type VolumeAttachmentStatus struct {
	//// The current state of the volume. Synced from volume.status.state
	//CurrentVolumeState VolumeState `json:"currentVolumeState"`
	//// The node that volume is currently attached to. Synced from volume.status.currentNodeID
	//CurrentNodeID string `json:"currentNodeID"`
	//// The current attachment parameters of the volume. Synced from volume.Spec
	//Parameters map[string]string `json:"parameters"`
	// +optional
	AttachmentStatuses map[string]*AttachmentStatus `json:"attachmentStatuses"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhva
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// VolumeAttachment stores attachment information of a Longhorn volume
type VolumeAttachment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VolumeAttachmentSpec   `json:"spec,omitempty"`
	Status VolumeAttachmentStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VolumeAttachmentList contains a list of VolumeAttachments
type VolumeAttachmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VolumeAttachment `json:"items"`
}
