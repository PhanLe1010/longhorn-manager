package app

import (
	"context"
	"fmt"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	lhclientset "github.com/longhorn/longhorn-manager/k8s/pkg/client/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"net/http"
	"os"
	"sort"
	"time"

	_ "net/http/pprof" // for runtime profiling

	"github.com/gorilla/handlers"
	"github.com/pkg/errors"
	"github.com/rancher/wrangler/pkg/signals"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	"github.com/longhorn/go-iscsi-helper/iscsi"

	iscsiutil "github.com/longhorn/go-iscsi-helper/util"

	"github.com/longhorn/longhorn-manager/api"
	"github.com/longhorn/longhorn-manager/controller"
	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/manager"
	"github.com/longhorn/longhorn-manager/meta"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/upgrade"
	"github.com/longhorn/longhorn-manager/util"

	metricscollector "github.com/longhorn/longhorn-manager/metrics_collector"
)

const (
	FlagEngineImage               = "engine-image"
	FlagInstanceManagerImage      = "instance-manager-image"
	FlagShareManagerImage         = "share-manager-image"
	FlagBackingImageManagerImage  = "backing-image-manager-image"
	FlagManagerImage              = "manager-image"
	FlagSupportBundleManagerImage = "support-bundle-manager-image"
	FlagServiceAccount            = "service-account"
	FlagKubeConfig                = "kube-config"
)

func DaemonCmd() cli.Command {
	return cli.Command{
		Name: "daemon",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  FlagEngineImage,
				Usage: "Specify Longhorn engine image",
			},
			cli.StringFlag{
				Name:  FlagInstanceManagerImage,
				Usage: "Specify Longhorn instance manager image",
			},
			cli.StringFlag{
				Name:  FlagShareManagerImage,
				Usage: "Specify Longhorn share manager image",
			},
			cli.StringFlag{
				Name:  FlagBackingImageManagerImage,
				Usage: "Specify Longhorn backing image manager image",
			},
			cli.StringFlag{
				Name:  FlagSupportBundleManagerImage,
				Usage: "Specify Longhorn support bundle manager image",
			},
			cli.StringFlag{
				Name:  FlagManagerImage,
				Usage: "Specify Longhorn manager image",
			},
			cli.StringFlag{
				Name:  FlagServiceAccount,
				Usage: "Specify service account for manager",
			},
			cli.StringFlag{
				Name:  FlagKubeConfig,
				Usage: "Specify path to kube config (optional)",
			},
		},
		Action: func(c *cli.Context) {
			if err := startManager(c); err != nil {
				logrus.Fatalf("Error starting manager: %v", err)
			}
		},
	}
}

func startManager(c *cli.Context) error {
	var (
		err error
	)

	engineImage := c.String(FlagEngineImage)
	if engineImage == "" {
		return fmt.Errorf("require %v", FlagEngineImage)
	}
	instanceManagerImage := c.String(FlagInstanceManagerImage)
	if instanceManagerImage == "" {
		return fmt.Errorf("require %v", FlagInstanceManagerImage)
	}
	shareManagerImage := c.String(FlagShareManagerImage)
	if shareManagerImage == "" {
		return fmt.Errorf("require %v", FlagShareManagerImage)
	}
	backingImageManagerImage := c.String(FlagBackingImageManagerImage)
	if backingImageManagerImage == "" {
		return fmt.Errorf("require %v", FlagBackingImageManagerImage)
	}
	supportBundleManagerImage := c.String(FlagSupportBundleManagerImage)
	if supportBundleManagerImage == "" {
		return fmt.Errorf("require %v", FlagSupportBundleManagerImage)
	}
	managerImage := c.String(FlagManagerImage)
	if managerImage == "" {
		return fmt.Errorf("require %v", FlagManagerImage)
	}
	serviceAccount := c.String(FlagServiceAccount)
	if serviceAccount == "" {
		return fmt.Errorf("require %v", FlagServiceAccount)
	}
	kubeconfigPath := c.String(FlagKubeConfig)

	if err := environmentCheck(); err != nil {
		return errors.Wrap(err, "Failed environment check, please make sure you have iscsiadm/open-iscsi installed on the host")
	}

	currentNodeID, err := util.GetRequiredEnv(types.EnvNodeName)
	if err != nil {
		return fmt.Errorf("failed to detect the node name")
	}

	currentIP, err := util.GetRequiredEnv(types.EnvPodIP)
	if err != nil {
		return fmt.Errorf("failed to detect the node IP")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return errors.Wrap(err, "unable to get client config")
	}

	config.Burst = 100
	config.QPS = 50

	lhClient, err := lhclientset.NewForConfig(config)
	if err != nil {
		return errors.Wrap(err, "unable to get clientset")
	}

	namespace := os.Getenv(types.EnvPodNamespace)
	if namespace == "" {
		logrus.Warnf("Cannot detect pod namespace, environment variable %v is missing, "+
			"using default namespace", types.EnvPodNamespace)
		namespace = corev1.NamespaceDefault
	}

	var nodes []longhorn.Node
	for i := 0; i < 15; i++ {
		nodeList, err := lhClient.LonghornV1beta2().Nodes(namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return err
		}
		nodes = nodeList.Items
		if len(nodes) == 3 {
			break
		}
		time.Sleep(1 * time.Second)
	}

	if len(nodes) != 3 {
		return fmt.Errorf("there must be 3 nodes instead of %v", len(nodes))
	}

	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Name < nodes[j].Name
	})

	index := 0
	for i, node := range nodes {
		if currentNodeID == node.Name {
			index = i
			break
		}
	}

	logrus.Infof("The current node is %v, with index is %v", currentNodeID, index)

	ctx := signals.SetupSignalContext(index)

	logger := logrus.StandardLogger().WithField("node", currentNodeID)

	webhookTypes := []string{types.WebhookTypeConversion, types.WebhookTypeAdmission}
	for _, webhookType := range webhookTypes {
		if err := startWebhook(ctx, serviceAccount, kubeconfigPath, webhookType); err != nil {
			return err
		}
	}

	if err := startRecoveryBackend(ctx, serviceAccount, kubeconfigPath); err != nil {
		return err
	}

	if err := upgrade.Upgrade(kubeconfigPath, currentNodeID, managerImage); err != nil {
		return err
	}

	proxyConnCounter := util.NewAtomicCounter()

	ds, wsc, err := controller.StartControllers(logger, ctx.Done(),
		currentNodeID, serviceAccount, managerImage, backingImageManagerImage, shareManagerImage,
		kubeconfigPath, meta.Version, proxyConnCounter)
	if err != nil {
		return err
	}

	m := manager.NewVolumeManager(currentNodeID, ds, proxyConnCounter)

	metricscollector.InitMetricsCollectorSystem(logger, currentNodeID, ds, kubeconfigPath, proxyConnCounter)

	defaultImageSettings := map[types.SettingName]string{
		types.SettingNameDefaultEngineImage:              engineImage,
		types.SettingNameDefaultInstanceManagerImage:     instanceManagerImage,
		types.SettingNameDefaultBackingImageManagerImage: backingImageManagerImage,
		types.SettingNameSupportBundleManagerImage:       supportBundleManagerImage,
	}
	if err := ds.UpdateCustomizedSettings(defaultImageSettings); err != nil {
		return err
	}

	if err := updateRegistrySecretName(m); err != nil {
		return err
	}

	if err := initDaemonNode(ds); err != nil {
		return err
	}

	if err := m.DeployEngineImage(engineImage); err != nil {
		return err
	}

	server := api.NewServer(m, wsc)
	router := http.Handler(api.NewRouter(server))
	router = util.FilteredLoggingHandler(map[string]struct{}{
		"/v1/apiversions":  {},
		"/v1/schemas":      {},
		"/v1/settings":     {},
		"/v1/volumes":      {},
		"/v1/nodes":        {},
		"/v1/engineimages": {},
		"/v1/events":       {},
	}, os.Stdout, router)
	router = handlers.ProxyHeaders(router)

	listen := types.GetAPIServerAddressFromIP(currentIP)
	logger.Infof("Listening on %s", listen)

	go func() {
		if err := http.ListenAndServe(listen, router); err != nil {
			logrus.Fatalf("Error longhorn backend server failed: %v", err)
		}
	}()

	go func() {
		debugAddress := "127.0.0.1:6060"
		debugHandler := http.DefaultServeMux
		logger.Infof("Debug Server listening on %s", debugAddress)
		if err := http.ListenAndServe(debugAddress, debugHandler); err != nil && err != http.ErrServerClosed {
			logger.Errorf(fmt.Sprintf("ListenAndServe: %s", err))
		}
	}()

	<-ctx.Done()
	return nil
}

func environmentCheck() error {
	initiatorNSPath := iscsiutil.GetHostNamespacePath(util.HostProcPath)
	namespace, err := iscsiutil.NewNamespaceExecutor(initiatorNSPath)
	if err != nil {
		return err
	}
	if err := iscsi.CheckForInitiatorExistence(namespace); err != nil {
		return err
	}
	return nil
}

func updateRegistrySecretName(m *manager.VolumeManager) error {
	settingRegistrySecret, err := m.GetSetting(types.SettingNameRegistrySecret)
	if err != nil {
		return err
	}
	managerDaemonSet, err := m.GetDaemonSetRO(types.LonghornManagerDaemonSetName)
	if err != nil {
		return err
	}
	registrySecretName := ""
	if len(managerDaemonSet.Spec.Template.Spec.ImagePullSecrets) > 0 {
		registrySecretName = managerDaemonSet.Spec.Template.Spec.ImagePullSecrets[0].Name
	}
	if settingRegistrySecret.Value != registrySecretName {
		settingRegistrySecret.Value = registrySecretName
		if _, err := m.CreateOrUpdateSetting(settingRegistrySecret); err != nil {
			return err
		}
	}
	return nil
}

func initDaemonNode(ds *datastore.DataStore) error {
	nodeName := os.Getenv("NODE_NAME")
	if _, err := ds.GetNode(nodeName); err != nil {
		// init default disk on node when starting longhorn-manager
		if datastore.ErrorIsNotFound(err) {
			if _, err = ds.CreateDefaultNode(nodeName); err != nil {
				return err
			}
		}
		return err
	}
	return nil
}
