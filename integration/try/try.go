package try

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/containous/i3o/internal/k8s"
	"github.com/containous/traefik/pkg/safe"
	log "github.com/sirupsen/logrus"
)

const (
	// CITimeoutMultiplier is the multiplier for all timeout in the CI
	CITimeoutMultiplier = 3
)

type Try struct {
	client *k8s.ClientWrapper
}

func NewTry(client *k8s.ClientWrapper) *Try {
	return &Try{client: client}
}

// WaitReadyDeployment wait until the deployment is ready.
func (t *Try) WaitReadyDeployment(name string, namespace string, timeout time.Duration) error {
	ebo := backoff.NewExponentialBackOff()
	ebo.MaxElapsedTime = applyCIMultiplier(timeout)

	if err := backoff.Retry(safe.OperationWithRecover(func() error {
		d, exists, err := t.client.GetDeployment(namespace, name)
		if err != nil {
			return fmt.Errorf("unable get the deployment %q in namespace %q: %v", name, namespace, err)
		}
		if !exists {
			return fmt.Errorf("deployment %q has not been yet created", name)
		}
		if d.Status.Replicas == 0 {
			return fmt.Errorf("deployment %q has no replicas", name)
		}

		if d.Status.ReadyReplicas == d.Status.Replicas {
			return nil
		}
		return errors.New("deployment not ready")
	}), ebo); err != nil {
		return fmt.Errorf("unable get the deployment %q in namespace %q: %v", name, namespace, err)
	}

	return nil
}

// WaitClientCreated wait until the file is created.
func (t *Try) WaitClientCreated(url string, kubeConfigPath string, timeout time.Duration) (*k8s.ClientWrapper, error) {
	ebo := backoff.NewExponentialBackOff()
	ebo.MaxElapsedTime = applyCIMultiplier(timeout)

	var clients *k8s.ClientWrapper
	var err error
	if err = backoff.Retry(safe.OperationWithRecover(func() error {
		clients, err = k8s.NewClientWrapper(url, kubeConfigPath)
		if err != nil {
			return fmt.Errorf("unable to create clients: %v", err)
		}

		if _, err = clients.KubeClient.ServerVersion(); err != nil {
			return fmt.Errorf("unable to get server version: %v", err)
		}

		return nil
	}), ebo); err != nil {
		return nil, fmt.Errorf("unable to create clients: %v", err)
	}

	return clients, nil
}

func applyCIMultiplier(timeout time.Duration) time.Duration {
	if os.Getenv("CI") == "" {
		return timeout
	}

	ciTimeoutMultiplier := getCITimeoutMultiplier()
	log.Debug("Apply CI multiplier:", ciTimeoutMultiplier)
	return time.Duration(float64(timeout) * ciTimeoutMultiplier)

}

func getCITimeoutMultiplier() float64 {
	ciTimeoutMultiplier := os.Getenv("CI_TIMEOUT_MULTIPLIER")
	if ciTimeoutMultiplier == "" {
		return CITimeoutMultiplier
	}

	multiplier, err := strconv.ParseFloat(ciTimeoutMultiplier, 64)
	if err != nil {
		return CITimeoutMultiplier
	}

	return multiplier
}