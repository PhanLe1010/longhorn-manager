package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	etypes "github.com/longhorn/longhorn-engine/pkg/types"

	longhornclient "github.com/longhorn/longhorn-manager/client"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	lhclientset "github.com/longhorn/longhorn-manager/k8s/pkg/client/clientset/versioned"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"
)

const (
	FlagSnapshotName = "snapshot-name"
	FlagGroups       = "groups"
	FlagLabels       = "labels"
	FlagRetain       = "retain"
	FlagConcurrent   = "concurrent"
	FlagBackup       = "backup"

	HTTPClientTimout = 1 * time.Minute

	SnapshotPurgeStatusInterval = 5 * time.Second

	WaitInterval              = 5 * time.Second
	DetachingWaitInterval     = 10 * time.Second
	VolumeAttachTimeout       = 300 // 5 minutes
	BackupProcessStartTimeout = 90  // 1.5 minutes

	MaxRecurringJobRetain = 50

	jobTypeSnapshot = string("snapshot")
	jobTypeBackup   = string("backup")
)

type Job struct {
	logger       logrus.FieldLogger
	lhClient     lhclientset.Interface
	namespace    string
	volumeName   string
	snapshotName string
	retain       int
	jobType      string
	labels       map[string]string

	api *longhornclient.RancherClient
}

func RecurringJobCmd() cli.Command {
	return cli.Command{
		Name: "recurring-job",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  FlagManagerURL,
				Usage: "Longhorn manager API URL",
			},
		},
		Action: func(c *cli.Context) {
			if err := recurringJob(c); err != nil {
				logrus.Fatalf("Error taking snapshot: %v", err)
			}
		},
	}
}

func recurringJob(c *cli.Context) error {
	logger := logrus.StandardLogger()
	var err error

	var managerURL string = c.String(FlagManagerURL)
	if managerURL == "" {
		return fmt.Errorf("require %v", FlagManagerURL)
	}

	if c.NArg() != 1 {
		return errors.New("job name is required")
	}
	jobName := c.Args()[0]

	namespace := os.Getenv(types.EnvPodNamespace)
	if namespace == "" {
		return fmt.Errorf("cannot detect pod namespace, environment variable %v is missing", types.EnvPodNamespace)
	}
	lhClient, err := getLonghornClientset()
	if err != nil {
		return errors.Wrap(err, "unable to get clientset")
	}
	recurringJob, err := lhClient.LonghornV1beta2().RecurringJobs(namespace).Get(context.TODO(), jobName, metav1.GetOptions{})
	if err != nil {
		logrus.WithError(err).Errorf("Failed to get recurring job %v.", jobName)
		return nil
	}

	var jobGroups []string = recurringJob.Spec.Groups
	var jobTask string = string(recurringJob.Spec.Task)
	var jobRetain int = recurringJob.Spec.Retain
	var jobConcurrent int = recurringJob.Spec.Concurrency

	jobLabelMap := map[string]string{}
	if recurringJob.Spec.Labels != nil {
		jobLabelMap = recurringJob.Spec.Labels
	}
	jobLabelMap[types.RecurringJobLabel] = recurringJob.Name
	labelJSON, err := json.Marshal(jobLabelMap)
	if err != nil {
		return errors.Wrap(err, "failed to get JSON encoding for labels")
	}

	var doBackup bool = false
	if jobTask == string(longhorn.RecurringJobTypeBackup) {
		doBackup = true
	}

	volumes, err := getVolumesBySelector(types.LonghornLabelRecurringJob, jobName, namespace, lhClient)
	if err != nil {
		return err
	}
	filteredVolumes := []string{}
	filterVolumesForJob(volumes, &filteredVolumes)
	for _, jobGroup := range jobGroups {
		volumes, err := getVolumesBySelector(types.LonghornLabelRecurringJobGroup, jobGroup, namespace, lhClient)
		if err != nil {
			return err
		}
		filterVolumesForJob(volumes, &filteredVolumes)
	}
	logger.Infof("Found %v volumes with recurring job %v", len(filteredVolumes), jobName)

	concurrentLimiter := make(chan struct{}, jobConcurrent)
	var wg sync.WaitGroup
	defer wg.Wait()
	for _, volumeName := range filteredVolumes {
		wg.Add(1)
		go func(volumeName string) {
			concurrentLimiter <- struct{}{}
			defer func() {
				<-concurrentLimiter
				wg.Done()
			}()

			log := logger.WithFields(logrus.Fields{
				"job":        jobName,
				"volume":     volumeName,
				"task":       jobTask,
				"retain":     jobRetain,
				"concurrent": jobConcurrent,
				"groups":     strings.Join(jobGroups, ","),
				"labels":     string(labelJSON),
			})
			log.Info("Creating job")

			snapshotName := sliceStringSafely(types.GetCronJobNameForRecurringJob(jobName), 0, 8) + "-" + util.UUID()
			job, err := NewJob(
				logger,
				managerURL,
				volumeName,
				snapshotName,
				jobLabelMap,
				jobRetain,
				doBackup)
			if err != nil {
				log.WithError(err).Error("failed to create new job for volume")
				return
			}
			err = job.run()
			if err != nil {
				log.WithError(err).Errorf("failed to run job for volume")
				return
			}

			log.Info("Created job")
		}(volumeName)
	}

	return nil
}

func sliceStringSafely(s string, begin, end int) string {
	if begin < 0 {
		begin = 0
	}
	if end > len(s) {
		end = len(s)
	}
	return s[begin:end]
}

func NewJob(logger logrus.FieldLogger, managerURL, volumeName, snapshotName string, labels map[string]string, retain int, backup bool) (*Job, error) {
	namespace := os.Getenv(types.EnvPodNamespace)
	if namespace == "" {
		return nil, fmt.Errorf("cannot detect pod namespace, environment variable %v is missing", types.EnvPodNamespace)
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, errors.Wrap(err, "unable to get client config")
	}
	lhClient, err := lhclientset.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get clientset")
	}

	clientOpts := &longhornclient.ClientOpts{
		Url:     managerURL,
		Timeout: HTTPClientTimout,
	}
	apiClient, err := longhornclient.NewRancherClient(clientOpts)
	if err != nil {
		return nil, errors.Wrapf(err, "could not create longhorn-manager api client")
	}

	// must at least retain 1 of course
	if retain == 0 {
		retain = 1
	}

	jobType := jobTypeSnapshot
	if backup {
		jobType = jobTypeBackup
	}

	logger = logger.WithFields(logrus.Fields{
		"namespace":    namespace,
		"volumeName":   volumeName,
		"snapshotName": snapshotName,
		"labels":       labels,
		"retain":       retain,
		"jobType":      jobType,
	})

	return &Job{
		logger:       logger,
		lhClient:     lhClient,
		namespace:    namespace,
		volumeName:   volumeName,
		snapshotName: snapshotName,
		labels:       labels,
		retain:       retain,
		jobType:      jobType,
		api:          apiClient,
	}, nil
}

func (job *Job) run() (err error) {
	job.logger.Info("job starts running")

	volumeAPI := job.api.Volume
	volumeName := job.volumeName
	jobName, _ := job.labels[types.RecurringJobLabel]
	if jobName == "" {
		return fmt.Errorf("missing RecurringJob label")
	}
	volume, err := volumeAPI.ById(volumeName)
	if err != nil {
		return errors.Wrapf(err, "could not get volume %v", volumeName)
	}

	if len(volume.Controllers) > 1 {
		return fmt.Errorf("cannot run job for volume %v that is using %v engines", volume.Name, len(volume.Controllers))
	}

	if volume.State != string(longhorn.VolumeStateAttached) && volume.State != string(longhorn.VolumeStateDetached) {
		return fmt.Errorf("volume %v is in an invalid state for recurring job: %v. Volume must be in state Attached or Detached", volumeName, volume.State)
	}

	if job.jobType == jobTypeBackup {
		job.logger.Infof("Running recurring backup for volume %v", volumeName)
		return job.doRecurringBackup()
	}
	job.logger.Infof("Running recurring snapshot for volume %v", volumeName)
	return job.doRecurringSnapshot()
}

func (job *Job) doRecurringSnapshot() (err error) {
	defer func() {
		if err == nil {
			job.logger.Info("Finish running recurring snapshot")
		}
	}()

	err = job.doSnapshot()
	if err != nil {
		return err
	}

	return job.doSnapshotCleanup(false)
}

func (job *Job) doSnapshot() (err error) {
	volumeAPI := job.api.Volume
	volumeName := job.volumeName
	volume, err := volumeAPI.ById(volumeName)
	if err != nil {
		return errors.Wrapf(err, "could not get volume %v", volumeName)
	}

	// get all snapshot CR belong to this recurring job
	snapshotCRList, err := job.api.Volume.ActionSnapshotCRList(volume)
	if err != nil {
		return err
	}

	snapshotCRs := filterSnapshotCRsWithLabel(snapshotCRList.Data, types.RecurringJobLabel, job.labels[types.RecurringJobLabel])

	if len(snapshotCRs) > MaxRecurringJobRetain {
		job.logger.Errorf("Trying to cleanup snapshots. There are currently too many snapshots created by this recurring job: %v snapshots. The maximum number of snapshots per recurring job is %v", len(snapshotCRs), MaxRecurringJobRetain)
		if err := job.doSnapshotCleanup(false); err != nil {
			return err
		}
		return fmt.Errorf("there are currently too many snapshots created by this recurring job: %v snapshots. The maximum number of snapshots per recurring job is %v", len(snapshotCRs), MaxRecurringJobRetain)
	}

	// Check if the latest snapshot of this recurring job is pending creation
	// If yes, update the snapshot name to be that snapshot
	// If no, create a new one with new snapshot job name
	var latestSnapshotCR longhornclient.SnapshotCR
	var latestSnapshotCRCreationTime time.Time
	for _, snapshotCR := range snapshotCRs {
		requestCreateNewSnapshot := snapshotCR.CreateSnapshot
		if !requestCreateNewSnapshot {
			// This snapshot was created by old snapshot API (AKA old recurring job).
			// So we skip considering it
			continue
		}
		t, err := time.Parse(time.RFC3339, snapshotCR.CrCreationTime)
		if err != nil {
			logrus.Errorf("Failed to parse datetime %v for snapshot %v",
				snapshotCR.CrCreationTime, snapshotCR.Name)
			continue
		}
		if t.After(latestSnapshotCRCreationTime) {
			latestSnapshotCRCreationTime = t
			latestSnapshotCR = snapshotCR
		}
	}

	alreadyCreatedBefore := latestSnapshotCR.CreationTime != ""
	if latestSnapshotCR.Name != "" && !alreadyCreatedBefore {
		job.snapshotName = latestSnapshotCR.Name
	} else {
		_, err = job.api.Volume.ActionSnapshotCRCreate(volume, &longhornclient.SnapshotCRInput{
			Labels: job.labels,
			Name:   job.snapshotName,
		})
		if err != nil {
			return err
		}
	}

	// wait for the snapshot to be completed
	for {
		existSnapshotCR, err := job.api.Volume.ActionSnapshotCRGet(volume, &longhornclient.SnapshotCRInput{
			Name: job.snapshotName,
		})
		if err != nil {
			return fmt.Errorf("error while waiting for snapshot %v to be ready: %v", job.snapshotName, err)
		}
		if existSnapshotCR.ReadyToUse {
			break
		}
		time.Sleep(WaitInterval)
	}

	job.logger.Infof("Complete creating the snapshot %v", job.snapshotName)

	return nil
}

func (job *Job) doSnapshotCleanup(backupDone bool) (err error) {
	volumeName := job.volumeName
	volume, err := job.api.Volume.ById(volumeName)
	if err != nil {
		return errors.Wrapf(err, "could not get volume %v", volumeName)
	}

	collection, err := job.api.Volume.ActionSnapshotCRList(volume)
	if err != nil {
		return err
	}

	cleanupSnapshotNames := job.listSnapshotNamesForCleanup(collection.Data, backupDone)
	for _, snapshotName := range cleanupSnapshotNames {
		if _, err := job.api.Volume.ActionSnapshotCRDelete(volume, &longhornclient.SnapshotCRInput{
			Name: snapshotName,
		}); err != nil {
			return err
		}
		job.logger.Debugf("Cleaned up snapshot CR %v for %v", snapshotName, volumeName)
	}

	return nil
}

type NameWithTimestamp struct {
	Name      string
	Timestamp time.Time
}

// getLastSnapshot return the last snapshot of the volume exclude the volume-head
// return nil, nil if volume doesn't have any snapshot other than the volume-head
func (job *Job) getLastSnapshot(volume *longhornclient.Volume) (*longhornclient.Snapshot, error) {
	volumeHead, err := job.api.Volume.ActionSnapshotGet(volume, &longhornclient.SnapshotInput{
		Name: etypes.VolumeHeadName,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "could not get volume-head for volume %v", job.volumeName)
	}

	if volumeHead.Parent == "" {
		return nil, nil
	}

	lastSnapshot, err := job.api.Volume.ActionSnapshotGet(volume, &longhornclient.SnapshotInput{
		Name: volumeHead.Parent,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "could not get parent of volume-head for volume %v", job.volumeName)
	}

	return lastSnapshot, nil
}

// getVolumeHeadSize return the size of volume-head snapshot
func (job *Job) getVolumeHeadSize(volume *longhornclient.Volume) (int64, error) {
	volumeHead, err := job.api.Volume.ActionSnapshotGet(volume, &longhornclient.SnapshotInput{
		Name: etypes.VolumeHeadName,
	})
	if err != nil {
		return 0, errors.Wrapf(err, "could not get volume-head for volume %v", job.volumeName)
	}
	volumeHeadSize, err := strconv.ParseInt(volumeHead.Size, 10, 64)
	if err != nil {
		return 0, err
	}

	return volumeHeadSize, nil
}

func (job *Job) listSnapshotNamesForCleanup(snapshotCRs []longhornclient.SnapshotCR, backupDone bool) []string {
	jobLabel, found := job.labels[types.RecurringJobLabel]
	if !found {
		return []string{}
	}

	// Only consider deleting the snapshots that were created by our current job
	snapshotCRs = filterSnapshotCRsWithLabel(snapshotCRs, types.RecurringJobLabel, jobLabel)

	if job.jobType == jobTypeSnapshot {
		return filterExpiredItems(snapshotCRsToNameWithTimestamps(snapshotCRs), job.retain)
	}

	// For the recurring backup job, only keep the snapshot of the last backup and the current snapshot
	retainingSnapshotCRs := map[string]struct{}{job.snapshotName: struct{}{}}
	if !backupDone {
		lastBackup, err := job.getLastBackup()
		if err == nil && lastBackup != nil {
			retainingSnapshotCRs[lastBackup.SnapshotName] = struct{}{}
		}
	}
	return snapshotCRsToNames(filterSnapshotCRsNotInTargets(snapshotCRs, retainingSnapshotCRs))
}

func (job *Job) doRecurringBackup() (err error) {
	defer func() {
		if err == nil {
			job.logger.Info("Finish running recurring backup")
		}
	}()

	defer func() {
		err = errors.Wrapf(err, "failed to complete backupAndCleanup for %v", job.volumeName)
	}()

	volume, err := job.api.Volume.ById(job.volumeName)
	if err != nil {
		return errors.Wrapf(err, "could not get volume %v", job.volumeName)
	}

	if err := job.doSnapshot(); err != nil {
		return err
	}

	if err := job.doSnapshotCleanup(false); err != nil {
		return err
	}

	if _, err := job.api.Volume.ActionSnapshotBackup(volume, &longhornclient.SnapshotInput{
		Labels: job.labels,
		Name:   job.snapshotName,
	}); err != nil {
		return err
	}

	if err := job.waitForBackupProcessStart(BackupProcessStartTimeout); err != nil {
		return err
	}

	// Wait for backup creation complete
	for {
		volume, err := job.api.Volume.ById(job.volumeName)
		if err != nil {
			return err
		}

		var info *longhornclient.BackupStatus
		for _, status := range volume.BackupStatus {
			if status.Snapshot == job.snapshotName {
				info = &status
				break
			}
		}

		if info == nil {
			return fmt.Errorf("cannot find the status of the backup for snapshot %v. It might because the engine has restarted", job.snapshotName)
		}

		complete := false

		switch info.State {
		case string(longhorn.BackupStateCompleted):
			complete = true
			job.logger.Debugf("Complete creating backup %v", info.Id)
		case string(longhorn.BackupStateNew), string(longhorn.BackupStateInProgress):
			job.logger.Debugf("Creating backup %v, current progress %v", info.Id, info.Progress)
		case string(longhorn.BackupStateError), string(longhorn.BackupStateUnknown):
			return fmt.Errorf("failed to create backup %v: %v", info.Id, info.Error)
		default:
			return fmt.Errorf("invalid state %v for backup %v", info.State, info.Id)
		}

		if complete {
			break
		}
		time.Sleep(WaitInterval)
	}

	defer func() {
		if err != nil {
			job.logger.Warnf("created backup successfully but errored on cleanup for %v: %v", job.volumeName, err)
			err = nil
		}
	}()

	backupVolume, err := job.api.BackupVolume.ById(job.volumeName)
	if err != nil {
		return err
	}
	backups, err := job.api.BackupVolume.ActionBackupList(backupVolume)
	if err != nil {
		return err
	}
	cleanupBackups := job.listBackupsForCleanup(backups.Data)
	for _, backup := range cleanupBackups {
		if _, err := job.api.BackupVolume.ActionBackupDelete(backupVolume, &longhornclient.BackupInput{
			Name: backup,
		}); err != nil {
			return fmt.Errorf("cleaned up backup %v failed for %v: %v", backup, job.volumeName, err)
		}
		job.logger.Debugf("Cleaned up backup %v for %v", backup, job.volumeName)
	}

	if err := job.doSnapshotCleanup(true); err != nil {
		return err
	}
	return nil
}

// waitForBackupProcessStart timeout in second
// Return nil if the backup progress has started; error if error or timeout
func (job *Job) waitForBackupProcessStart(timeout int) error {
	volumeAPI := job.api.Volume
	volumeName := job.volumeName
	snapshot := job.snapshotName

	for i := 0; i < timeout; i++ {
		volume, err := volumeAPI.ById(volumeName)
		if err != nil {
			return err
		}

		for _, status := range volume.BackupStatus {
			if status.Snapshot == snapshot {
				return nil
			}
		}
		time.Sleep(WaitInterval)
	}
	return fmt.Errorf("timeout waiting for the backup of the snapshot %v of volume %v to start", snapshot, volumeName)
}

// getLastBackup return the last backup of the volume
// return nil, nil if volume doesn't have any backup
func (job *Job) getLastBackup() (*longhornclient.Backup, error) {
	var err error
	defer func() {
		err = errors.Wrapf(err, "failed to get last backup for %v", job.volumeName)
	}()

	volume, err := job.api.Volume.ById(job.volumeName)
	if err != nil {
		return nil, err
	}
	if volume.LastBackup == "" {
		return nil, nil
	}
	backupVolume, err := job.api.BackupVolume.ById(job.volumeName)
	if err != nil {
		return nil, err
	}
	return job.api.BackupVolume.ActionBackupGet(backupVolume, &longhornclient.BackupInput{
		Name: volume.LastBackup,
	})
}

func (job *Job) listBackupsForCleanup(backups []longhornclient.Backup) []string {
	sts := []NameWithTimestamp{}

	// only remove backups that where created by our current job
	jobLabel, found := job.labels[types.RecurringJobLabel]
	if !found {
		return []string{}
	}
	for _, backup := range backups {
		backupLabel, found := backup.Labels[types.RecurringJobLabel]
		if found && jobLabel == backupLabel {
			t, err := time.Parse(time.RFC3339, backup.Created)
			if err != nil {
				job.logger.Errorf("Failed to parse datetime %v for backup %v",
					backup.Created, backup)
				continue
			}
			sts = append(sts, NameWithTimestamp{
				Name:      backup.Name,
				Timestamp: t,
			})
		}
	}
	return filterExpiredItems(sts, job.retain)
}

func (job *Job) GetVolume(name string) (*longhorn.Volume, error) {
	return job.lhClient.LonghornV1beta2().Volumes(job.namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (job *Job) GetEngineImage(name string) (*longhorn.EngineImage, error) {
	return job.lhClient.LonghornV1beta2().EngineImages(job.namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (job *Job) UpdateVolumeStatus(v *longhorn.Volume) (*longhorn.Volume, error) {
	return job.lhClient.LonghornV1beta2().Volumes(job.namespace).UpdateStatus(context.TODO(), v, metav1.UpdateOptions{})
}

// waitForVolumeState timeout in second
func (job *Job) waitForVolumeState(state string, timeout int) (*longhornclient.Volume, error) {
	volumeAPI := job.api.Volume
	volumeName := job.volumeName

	for i := 0; i < timeout; i++ {
		volume, err := volumeAPI.ById(volumeName)
		if err == nil {
			if volume.State == state {
				return volume, nil
			}
		}
		time.Sleep(1 * time.Second)
	}

	return nil, fmt.Errorf("timeout waiting for volume %v to be in state %v", volumeName, state)
}

func filterSnapshotCRs(snapshotCRs []longhornclient.SnapshotCR, predicate func(snapshot longhornclient.SnapshotCR) bool) []longhornclient.SnapshotCR {
	filtered := []longhornclient.SnapshotCR{}
	for _, snapshotCR := range snapshotCRs {
		if predicate(snapshotCR) {
			filtered = append(filtered, snapshotCR)
		}
	}
	return filtered
}

// filterSnapshotCRsWithLabel return snapshotCRs that have LabelKey and LabelValue
func filterSnapshotCRsWithLabel(snapshotCRs []longhornclient.SnapshotCR, labelKey, labelValue string) []longhornclient.SnapshotCR {
	return filterSnapshotCRs(snapshotCRs, func(snapshotCR longhornclient.SnapshotCR) bool {
		snapshotLabelValue, found := snapshotCR.Labels[labelKey]
		return found && labelValue == snapshotLabelValue
	})
}

// filterSnapshotCRsNotInTargets returns snapshots that are not in the Targets
func filterSnapshotCRsNotInTargets(snapshotCRs []longhornclient.SnapshotCR, targets map[string]struct{}) []longhornclient.SnapshotCR {
	return filterSnapshotCRs(snapshotCRs, func(snapshotCR longhornclient.SnapshotCR) bool {
		if _, ok := targets[snapshotCR.Name]; !ok {
			return true
		}
		return false
	})
}

// filterExpiredItems returns a list of names from the input sts excluding the latest retainCount names
func filterExpiredItems(nts []NameWithTimestamp, retainCount int) []string {
	sort.Slice(nts, func(i, j int) bool {
		return nts[i].Timestamp.Before(nts[j].Timestamp)
	})

	ret := []string{}
	for i := 0; i < len(nts)-retainCount; i++ {
		ret = append(ret, nts[i].Name)
	}
	return ret
}

func snapshotCRsToNameWithTimestamps(snapshotCRs []longhornclient.SnapshotCR) []NameWithTimestamp {
	result := []NameWithTimestamp{}
	for _, snapshotCR := range snapshotCRs {
		t, err := time.Parse(time.RFC3339, snapshotCR.CrCreationTime)
		if err != nil {
			logrus.Errorf("Failed to parse datetime %v for snapshot CR %v",
				snapshotCR.CrCreationTime, snapshotCR.Name)
			continue
		}
		result = append(result, NameWithTimestamp{
			Name:      snapshotCR.Name,
			Timestamp: t,
		})
	}
	return result
}

func snapshotCRsToNames(snapshotCRs []longhornclient.SnapshotCR) []string {
	result := []string{}
	for _, snapshotCR := range snapshotCRs {
		result = append(result, snapshotCR.Name)
	}
	return result
}

func filterVolumesForJob(volumes []longhorn.Volume, filterNames *[]string) {
	logger := logrus.StandardLogger()
	for _, volume := range volumes {
		// skip duplicates
		if util.Contains(*filterNames, volume.Name) {
			continue
		}

		if volume.Status.RestoreRequired {
			logger.Debugf("Bypassed to create job for %v volume during restoring from the backup", volume.Name)
			continue
		}

		if volume.Status.Robustness != longhorn.VolumeRobustnessFaulted {
			*filterNames = append(*filterNames, volume.Name)
			continue
		}
		logger.Debugf("Cannot create job for %v volume in state %v", volume.Name, volume.Status.State)
	}
}

func getVolumesBySelector(recurringJobType, recurringJobName, namespace string, client *lhclientset.Clientset) ([]longhorn.Volume, error) {
	logger := logrus.StandardLogger()

	label := fmt.Sprintf("%s=%s",
		types.GetRecurringJobLabelKey(recurringJobType, recurringJobName), types.LonghornLabelValueEnabled)
	logger.Debugf("Get volumes from label %v", label)

	volumes, err := client.LonghornV1beta2().Volumes(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: label,
	})
	if err != nil {
		return nil, err
	}
	return volumes.Items, nil
}

func getSettingAsBoolean(name types.SettingName, namespace string, client *lhclientset.Clientset) (bool, error) {
	obj, err := client.LonghornV1beta2().Settings(namespace).Get(context.TODO(), string(name), metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	value, err := strconv.ParseBool(obj.Value)
	if err != nil {
		return false, err
	}
	return value, nil
}

func getLonghornClientset() (*lhclientset.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, errors.Wrap(err, "unable to get client config")
	}
	return lhclientset.NewForConfig(config)
}
