package v14xto150

import (
	"context"
	"strconv"

	"github.com/pkg/errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	lhclientset "github.com/longhorn/longhorn-manager/k8s/pkg/client/clientset/versioned"
	"github.com/longhorn/longhorn-manager/types"
	upgradeutil "github.com/longhorn/longhorn-manager/upgrade/util"
)

const (
	upgradeLogPrefix = "upgrade from v1.4.x to v1.5.0: "
)

func UpgradeResources(namespace string, lhClient *lhclientset.Clientset, kubeClient *clientset.Clientset, resourceMaps map[string]interface{}) error {
	if err := upgradeVolumes(namespace, lhClient, resourceMaps); err != nil {
		return err
	}
	if err := upgradeWebhookAndRecoveryService(namespace, kubeClient); err != nil {
		return err
	}
	if err := upgradeVolumeAttachments(namespace, lhClient, resourceMaps); err != nil {
		return err
	}
	return nil
}

func upgradeVolumes(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade volume failed")
	}()

	volumeMap, err := upgradeutil.ListAndUpdateVolumesInProvidedCache(namespace, lhClient, resourceMaps)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to list all existing Longhorn volumes during the volume upgrade")
	}

	for _, v := range volumeMap {
		if v.Spec.BackupCompressionMethod == "" {
			v.Spec.BackupCompressionMethod = longhorn.BackupCompressionMethodGzip
		}
		if v.Spec.DataLocality == longhorn.DataLocalityStrictLocal {
			v.Spec.RevisionCounterDisabled = true
		}
		if v.Spec.ReplicaSoftAntiAffinity == "" {
			v.Spec.ReplicaSoftAntiAffinity = longhorn.ReplicaSoftAntiAffinityDefault
		}
		if v.Spec.ReplicaZoneSoftAntiAffinity == "" {
			v.Spec.ReplicaZoneSoftAntiAffinity = longhorn.ReplicaZoneSoftAntiAffinityDefault
		}
	}

	return nil
}

func upgradeVolumeAttachments(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade VolumeAttachment failed")
	}()

	volumeList, err := lhClient.LonghornV1beta2().Volumes(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to list all existing Longhorn volumes during the VolumeAttachment upgrade")
	}
	replicaList, err := lhClient.LonghornV1beta2().Replicas(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to list all existing replicas during the VolumeAttachment upgrade")
	}
	bidsList, err := lhClient.LonghornV1beta2().BackingImageDataSources(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to list all existing BackingImageDataSources during the VolumeAttachment upgrade")
	}

	volumeAttachments := map[string]*longhorn.VolumeAttachment{}

	for _, v := range volumeList.Items {
		va := longhorn.VolumeAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:   types.GetLHVolumeAttachmentNameFromVolumeName(v.Name),
				Labels: types.GetVolumeLabels(v.Name),
			},
			Spec: longhorn.VolumeAttachmentSpec{
				AttachmentTickets: generateVolumeAttachmentTickets(v, volumeList.Items, replicaList.Items, bidsList.Items, resourceMaps[types.LonghornKindVolume].(map[string]*longhorn.Volume)),
				Volume:            v.Name,
			},
		}
		volumeAttachments[va.Name] = &va
	}

	resourceMaps[types.LonghornKindVolumeAttachment] = volumeAttachments

	return nil
}

func generateVolumeAttachmentTickets(vol longhorn.Volume, volumes []longhorn.Volume, replicas []longhorn.Replica, bids []longhorn.BackingImageDataSource, cachedVolumes map[string]*longhorn.Volume) map[string]*longhorn.AttachmentTicket {
	attachmentTickets := make(map[string]*longhorn.AttachmentTicket)
	if vol.Spec.NodeID != "" {
		ticketID := longhorn.GetAttachmentTicketID(longhorn.AttacherTypeLonghornUpgrader, vol.Spec.NodeID)
		attachmentTickets[ticketID] = &longhorn.AttachmentTicket{
			ID:     ticketID,
			Type:   longhorn.AttacherTypeLonghornUpgrader,
			NodeID: vol.Spec.NodeID,
			Parameters: map[string]string{
				longhorn.AttachmentParameterDisableFrontend: strconv.FormatBool(vol.Spec.DisableFrontend),
				longhorn.AttachmentParameterLastAttachedBy:  vol.Spec.LastAttachedBy,
			},
		}
	}

	if vol.Spec.MigrationNodeID != "" {
		ticketID := longhorn.GetAttachmentTicketID(longhorn.AttacherTypeLonghornUpgrader, vol.Spec.MigrationNodeID)
		attachmentTickets[ticketID] = &longhorn.AttachmentTicket{
			ID:     ticketID,
			Type:   longhorn.AttacherTypeLonghornUpgrader,
			NodeID: vol.Spec.NodeID,
			Parameters: map[string]string{
				longhorn.AttachmentParameterDisableFrontend: strconv.FormatBool(vol.Spec.DisableFrontend),
				longhorn.AttachmentParameterLastAttachedBy:  vol.Spec.LastAttachedBy,
			},
		}
	}

	if vol.Spec.NodeID != "" || vol.Status.State != longhorn.VolumeStateAttached || vol.Status.CurrentNodeID == "" {
		// Volume is not auto attached
		return attachmentTickets
	}

	// Create tickets for internal auto attachment
	v, ok := cachedVolumes[vol.Name]
	if !ok {
		return attachmentTickets
	}

	v.Spec.NodeID = v.Status.CurrentNodeID

	// DR/Restore
	if vol.Status.RestoreRequired {
		ticketID := longhorn.GetAttachmentTicketID(longhorn.AttacherTypeVolumeRestoreController, vol.Name)
		attachmentTickets[ticketID] = &longhorn.AttachmentTicket{
			ID:     ticketID,
			Type:   longhorn.AttacherTypeVolumeRestoreController,
			NodeID: vol.Status.CurrentNodeID,
			Parameters: map[string]string{
				longhorn.AttachmentParameterDisableFrontend: longhorn.TrueValue,
			},
		}
	}
	// Expansion
	if vol.Status.ExpansionRequired {
		ticketID := longhorn.GetAttachmentTicketID(longhorn.AttacherTypeVolumeExpansionController, vol.Name)
		attachmentTickets[ticketID] = &longhorn.AttachmentTicket{
			ID:     ticketID,
			Type:   longhorn.AttacherTypeVolumeExpansionController,
			NodeID: vol.Status.CurrentNodeID,
			Parameters: map[string]string{
				longhorn.AttachmentParameterDisableFrontend: longhorn.FalseValue,
			},
		}
	}

	// Eviction
	for _, r := range replicas {
		if r.Spec.VolumeName == vol.Name && r.Status.EvictionRequested {
			ticketID := longhorn.GetAttachmentTicketID(longhorn.AttacherTypeVolumeEvictionController, vol.Name)
			attachmentTickets[ticketID] = &longhorn.AttachmentTicket{
				ID:     ticketID,
				Type:   longhorn.AttacherTypeVolumeEvictionController,
				NodeID: vol.Status.CurrentNodeID,
				Parameters: map[string]string{
					longhorn.AttachmentParameterDisableFrontend: longhorn.AnyValue,
				},
			}
		}
	}
	// Cloning
	if isTargetVolumeOfCloning(&vol) {
		ticketID := longhorn.GetAttachmentTicketID(longhorn.AttacherTypeVolumeCloneController, vol.Name)
		attachmentTickets[ticketID] = &longhorn.AttachmentTicket{
			ID:     ticketID,
			Type:   longhorn.AttacherTypeVolumeCloneController,
			NodeID: vol.Status.CurrentNodeID,
			Parameters: map[string]string{
				longhorn.AttachmentParameterDisableFrontend: longhorn.TrueValue,
			},
		}
	}
	for _, v := range volumes {
		if isTargetVolumeOfCloning(&v) && types.GetVolumeName(v.Spec.DataSource) == vol.Name {
			ticketID := longhorn.GetAttachmentTicketID(longhorn.AttacherTypeVolumeCloneController, v.Name)
			attachmentTickets[ticketID] = &longhorn.AttachmentTicket{
				ID:     ticketID,
				Type:   longhorn.AttacherTypeVolumeCloneController,
				NodeID: vol.Status.CurrentNodeID,
				Parameters: map[string]string{
					longhorn.AttachmentParameterDisableFrontend: longhorn.AnyValue,
				},
			}
		}
	}
	// Backing image datasource exporting
	for _, b := range bids {
		if !b.DeletionTimestamp.IsZero() ||
			b.Spec.SourceType != longhorn.BackingImageDataSourceTypeExportFromVolume ||
			b.Spec.FileTransferred ||
			vol.Name != b.Spec.Parameters[longhorn.DataSourceTypeExportFromVolumeParameterVolumeName] ||
			b.Status.CurrentState == longhorn.BackingImageStateFailed ||
			b.Status.CurrentState == longhorn.BackingImageStateUnknown {
			continue
		}
		ticketID := longhorn.GetAttachmentTicketID(longhorn.AttacherTypeBackingImageDataSourceController, b.Name)
		attachmentTickets[ticketID] = &longhorn.AttachmentTicket{
			ID:     ticketID,
			Type:   longhorn.AttacherTypeBackingImageDataSourceController,
			NodeID: vol.Status.CurrentNodeID,
			Parameters: map[string]string{
				longhorn.AttachmentParameterDisableFrontend: longhorn.AnyValue,
			},
		}
	}

	return attachmentTickets
}

// isTargetVolumeOfCloning checks if the input volume is the target volume of an on-going cloning process
// workaround: this is a duplication of the same function in controller package to avoid importing cycle
func isTargetVolumeOfCloning(v *longhorn.Volume) bool {
	isCloningDesired := types.IsDataFromVolume(v.Spec.DataSource)
	isCloningDone := v.Status.CloneStatus.State == longhorn.VolumeCloneStateCompleted ||
		v.Status.CloneStatus.State == longhorn.VolumeCloneStateFailed
	return isCloningDesired && !isCloningDone
}

func upgradeWebhookAndRecoveryService(namespace string, kubeClient *clientset.Clientset) error {
	selectors := []string{"app=longhorn-conversion-webhook", "app=longhorn-admission-webhook", "app=longhorn-recovery-backend"}

	for _, selector := range selectors {
		deployments, err := kubeClient.AppsV1().Deployments("").List(context.TODO(), metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return errors.Wrapf(err, upgradeLogPrefix+"failed to get deployment with label %v during the upgrade", selector)
		}
		for _, deployment := range deployments.Items {
			err := kubeClient.AppsV1().Deployments(deployment.Namespace).Delete(context.TODO(), deployment.Name, metav1.DeleteOptions{})
			if err != nil {
				return errors.Wrapf(err, upgradeLogPrefix+"failed to delete the deployment with label %v during the upgrade", selector)
			}
		}
	}

	return nil
}
