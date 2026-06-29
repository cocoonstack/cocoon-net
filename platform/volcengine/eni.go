package volcengine

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/projecteru2/core/log"
)

const (
	createPropagationDelay = 2 * time.Second
	attachPropagationDelay = 4 * time.Second
)

// networkInterface is the JSON shape of a Volcengine ENI.
type networkInterface struct {
	NetworkInterfaceID string `json:"NetworkInterfaceId"`
	Type               string `json:"Type"`
	PrivateIPSets      struct {
		PrivateIPSet []struct {
			Primary          bool   `json:"Primary"`
			PrivateIPAddress string `json:"PrivateIpAddress"`
		} `json:"PrivateIpSet"`
	} `json:"PrivateIpSets"`
}

func createAndAttachENIs(ctx context.Context, subnetID, sgID, instanceID, prefix string, count int) ([]string, error) {
	logger := log.WithFunc("volcengine.createAndAttachENIs")
	eniIDs := make([]string, 0, count)

	for i := 1; i <= count; i++ {
		out, err := veRun(
			ctx, "vpc", "CreateNetworkInterface",
			"--SubnetId", subnetID,
			"--SecurityGroupIds.1", sgID,
			"--NetworkInterfaceName", fmt.Sprintf("%s-eni-%d", prefix, i),
		)
		if err != nil {
			return nil, fmt.Errorf("create ENI %d: %w", i, err)
		}
		var resp struct {
			Result struct {
				NetworkInterfaceID string `json:"NetworkInterfaceId"`
			} `json:"Result"`
		}
		if unmarshalErr := json.Unmarshal(out, &resp); unmarshalErr != nil {
			return nil, fmt.Errorf("parse create ENI %d response: %w", i, unmarshalErr)
		}
		eniID := resp.Result.NetworkInterfaceID

		time.Sleep(createPropagationDelay)

		_, attachErr := veRun(
			ctx, "vpc", "AttachNetworkInterface",
			"--NetworkInterfaceId", eniID,
			"--InstanceId", instanceID,
		)
		if attachErr != nil {
			// Attach failed: this ENI is unusable. Best-effort delete to
			// avoid leaking quota (log-only — the attach failure is the
			// primary signal). Degraded, not fatal: remaining ENIs can
			// still build a usable pool.
			logger.Warnf(ctx, "attach ENI %s: %v", eniID, attachErr)
			if _, delErr := veRun(
				ctx, "vpc", "DeleteNetworkInterface",
				"--NetworkInterfaceId", eniID,
			); delErr != nil {
				logger.Warnf(ctx, "delete orphan ENI %s: %v", eniID, delErr)
			}
			continue
		}

		time.Sleep(attachPropagationDelay)

		eniIDs = append(eniIDs, eniID)
		logger.Infof(ctx, "created and attached ENI %s (%d/%d)", eniID, i, count)
	}
	return eniIDs, nil
}

func assignSecondaryIPs(ctx context.Context, eniID string, count int) ([]string, error) {
	out, err := veRun(
		ctx, "vpc", "AssignPrivateIpAddresses",
		"--NetworkInterfaceId", eniID,
		"--SecondaryPrivateIpAddressCount", strconv.Itoa(count),
	)
	if err != nil {
		return nil, fmt.Errorf("assign secondary IPs to %s: %w", eniID, err)
	}

	var resp struct {
		Result struct {
			PrivateIPSet []string `json:"PrivateIpSet"`
		} `json:"Result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse assign IPs response: %w", err)
	}
	return resp.Result.PrivateIPSet, nil
}

func listENIs(ctx context.Context, instanceID string) ([]networkInterface, error) {
	out, err := veRun(
		ctx, "vpc", "DescribeNetworkInterfaces",
		"--InstanceId", instanceID,
		"--PageSize", "100",
	)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Result struct {
			NetworkInterfaceSets []networkInterface `json:"NetworkInterfaceSets"`
		} `json:"Result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse ENI list: %w", err)
	}
	return resp.Result.NetworkInterfaceSets, nil
}
