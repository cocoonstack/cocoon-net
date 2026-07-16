package volcengine

import (
	"context"
	"fmt"

	"github.com/projecteru2/core/log"

	"github.com/cocoonstack/cocoon-net/platform"
)

// cfg is ignored: teardown walks the ENIs currently attached rather than persisted state.
func (v *Volcengine) Teardown(ctx context.Context, _ *platform.TeardownConfig) error {
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

		_, detachErr := veRun(
			ctx, "vpc", "DetachNetworkInterface",
			"--NetworkInterfaceId", eni.NetworkInterfaceID,
			"--InstanceId", instanceID,
		)
		if detachErr != nil {
			logger.Errorf(ctx, detachErr, "detach ENI %s (skipping delete)", eni.NetworkInterfaceID)
			continue
		}

		if err := sleepCtx(ctx, attachPropagationDelay); err != nil {
			return err
		}

		_, delErr := veRun(
			ctx, "vpc", "DeleteNetworkInterface",
			"--NetworkInterfaceId", eni.NetworkInterfaceID,
		)
		if delErr != nil {
			logger.Errorf(ctx, delErr, "delete ENI %s", eni.NetworkInterfaceID)
		} else {
			logger.Infof(ctx, "deleted ENI %s", eni.NetworkInterfaceID)
		}
	}
	return nil
}
