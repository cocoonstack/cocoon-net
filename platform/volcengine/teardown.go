package volcengine

import (
	"context"
	"errors"
	"fmt"

	"github.com/projecteru2/core/log"

	"github.com/cocoonstack/cocoon-net/platform"
)

func (v *Volcengine) Teardown(ctx context.Context, cfg *platform.TeardownConfig) error {
	logger := log.WithFunc("volcengine.Teardown")

	enis, err := listTeardownENIs(ctx, cfg.ENIIDs)
	if err != nil {
		return fmt.Errorf("list ENIs: %w", err)
	}

	var errs error
	for _, eni := range enis {
		if eni.Type == eniTypePrimary {
			continue
		}

		if eni.DeviceID != "" {
			_, detachErr := runVe(
				ctx, "vpc", "DetachNetworkInterface",
				"--NetworkInterfaceId", eni.NetworkInterfaceID,
				"--InstanceId", eni.DeviceID,
			)
			if detachErr != nil {
				errs = errors.Join(errs, fmt.Errorf("detach ENI %s: %w", eni.NetworkInterfaceID, detachErr))
				continue
			}

			if err := sleepCtx(ctx, attachPropagationDelay); err != nil {
				return err
			}
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

func listTeardownENIs(ctx context.Context, eniIDs []string) ([]networkInterface, error) {
	if len(eniIDs) > 0 {
		return listENIsByIDs(ctx, eniIDs)
	}
	instanceID, err := fetchMeta(ctx, "/instance-id")
	if err != nil {
		return nil, fmt.Errorf("get instance id: %w", err)
	}
	return listENIs(ctx, instanceID)
}
