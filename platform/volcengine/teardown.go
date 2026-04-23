package volcengine

import (
	"context"
	"fmt"
	"time"

	"github.com/projecteru2/core/log"
)

const attachPropagationDelay = 4 * time.Second

// Teardown detaches and deletes all secondary ENIs for this instance.
func (v *Volcengine) Teardown(ctx context.Context) error {
	logger := log.WithFunc("volcengine.Teardown")

	instanceID, err := fetchMeta(ctx, "/instance-id")
	if err != nil {
		return fmt.Errorf("get instance id: %w", err)
	}

	enis, err := listENIs(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("list ENIs: %w", err)
	}

	for _, eni := range enis {
		if eni.Type == eniTypePrimary {
			continue
		}

		_, detachErr := veRun(ctx, "vpc", "DetachNetworkInterface",
			"--NetworkInterfaceId", eni.NetworkInterfaceID,
			"--InstanceId", instanceID,
		)
		if detachErr != nil {
			logger.Warnf(ctx, "detach ENI %s: %v (skipping delete)", eni.NetworkInterfaceID, detachErr)
			continue
		}

		// Wait for detach to propagate before deleting.
		time.Sleep(attachPropagationDelay)

		_, delErr := veRun(ctx, "vpc", "DeleteNetworkInterface",
			"--NetworkInterfaceId", eni.NetworkInterfaceID,
		)
		if delErr != nil {
			logger.Warnf(ctx, "delete ENI %s: %v", eni.NetworkInterfaceID, delErr)
		} else {
			logger.Infof(ctx, "deleted ENI %s", eni.NetworkInterfaceID)
		}
	}
	return nil
}
