package csi

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"time"

	longhornclient "github.com/longhorn/longhorn-manager/client"
)

type Manager struct {
	ids *IdentityServer
	ns  *NodeServer
	cs  *ControllerServer
}

func init() {}

func GetCSIManager() *Manager {
	return &Manager{}
}

func (m *Manager) Run(driverName, nodeID, endpoint, identityVersion, managerURL string) error {
	logrus.Infof("CSI Driver: %v version: %v, manager URL %v", driverName, identityVersion, managerURL)

	// Longhorn API Client
	clientOpts := &longhornclient.ClientOpts{Url: managerURL}
	apiClient, err := initRancherClient(clientOpts)
	if err != nil {
		return err
	}

	// Create GRPC servers
	m.ids = NewIdentityServer(driverName, identityVersion)
	m.ns = NewNodeServer(apiClient, nodeID)
	m.cs = NewControllerServer(apiClient, nodeID)
	s := NewNonBlockingGRPCServer()
	s.Start(endpoint, m.ids, m.cs, m.ns)
	s.Wait()

	return nil
}

func initRancherClient(clientOpts *longhornclient.ClientOpts) (*longhornclient.RancherClient, error) {
	maxRetry := 20
	var lastErr error

	for i := 0; i < maxRetry; i++ {
		apiClient, err := longhornclient.NewRancherClient(clientOpts)
		if err == nil {
			return apiClient, nil
		}
		logrus.Warnf("Failed to initialize Longhorn API client %v. Retrying", err)
		lastErr = err
		time.Sleep(time.Second)
	}

	return nil, errors.Wrap(lastErr, "Failed to initialize Longhorn API client")
}
