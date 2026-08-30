package volcengine

import (
	"context"
	"errors"
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

	var errs error
	for _, eni := range enis {
		if eni.Type == eniTypePrimary {
			continue
		}

		_, detachErr := runVe(
			ctx, "vpc", "DetachNetworkInterface",
			"--NetworkInterfaceId", eni.NetworkInterfaceID,
			"--InstanceId", instanceID,
		)
		if detachErr != nil {
			errs = errors.Join(errs, fmt.Errorf("detach ENI %s: %w", eni.NetworkInterfaceID, detachErr))
			continue
		}

		if err := sleepCtx(ctx, attachPropagationDelay); err != nil {
			return err
		}

		_, delErr := runVe(
			ctx, "vpc", "DeleteNetworkInterface",
			"--NetworkInterfaceId", eni.NetworkInterfaceID,
		)
		if delErr != nil {
			errs = errors.Join(errs, fmt.Errorf("delete ENI %s: %w", eni.NetworkInterfaceID, delErr))
			continue
		}
		logger.Infof(ctx, "deleted ENI %s", eni.NetworkInterfaceID)
	}
	return errs
}
