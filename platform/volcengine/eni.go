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

func ensureENIs(ctx context.Context, subnetID, sgID, instanceID, prefix string, count int) ([]networkInterface, error) {
	logger := log.WithFunc("volcengine.ensureENIs")

	existing, err := listENIs(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("list existing ENIs: %w", err)
	}
	result := reusableENIs(existing, count)
	if len(result) > 0 {
		logger.Infof(ctx, "reusing %d existing ENI(s)", len(result))
	}

	for i := len(result) + 1; i <= count; i++ {
		out, err := veRun(
			ctx, "vpc", "CreateNetworkInterface",
			"--SubnetId", subnetID,
			"--SecurityGroupIds.1", sgID,
			"--NetworkInterfaceName", fmt.Sprintf("%s-eni-%d", prefix, i),
		)
		if err != nil {
			return result, fmt.Errorf("create ENI %d: %w", i, err)
		}
		var resp struct {
			Result struct {
				NetworkInterfaceID string `json:"NetworkInterfaceId"`
			} `json:"Result"`
		}
		if unmarshalErr := json.Unmarshal(out, &resp); unmarshalErr != nil {
			return result, fmt.Errorf("parse create ENI %d response: %w", i, unmarshalErr)
		}
		eniID := resp.Result.NetworkInterfaceID

		if err := sleepCtx(ctx, createPropagationDelay); err != nil {
			if _, delErr := veRun(context.WithoutCancel(ctx), "vpc", "DeleteNetworkInterface", "--NetworkInterfaceId", eniID); delErr != nil {
				logger.Warnf(ctx, "delete orphan ENI %s: %v", eniID, delErr)
			}
			return result, err
		}

		_, attachErr := veRun(
			ctx, "vpc", "AttachNetworkInterface",
			"--NetworkInterfaceId", eniID,
			"--InstanceId", instanceID,
		)
		if attachErr != nil {
			// Best-effort delete to avoid leaking ENI quota; degraded not fatal, keep building the pool.
			logger.Warnf(ctx, "attach ENI %s: %v", eniID, attachErr)
			if _, delErr := veRun(
				ctx, "vpc", "DeleteNetworkInterface",
				"--NetworkInterfaceId", eniID,
			); delErr != nil {
				logger.Warnf(ctx, "delete orphan ENI %s: %v", eniID, delErr)
			}
			continue
		}

		if err := sleepCtx(ctx, attachPropagationDelay); err != nil {
			return result, err
		}

		result = append(result, networkInterface{NetworkInterfaceID: eniID})
		logger.Infof(ctx, "created and attached ENI %s (%d/%d)", eniID, i, count)
	}
	return result, nil
}

func reusableENIs(enis []networkInterface, count int) []networkInterface {
	var reusable []networkInterface
	for _, e := range enis {
		if e.Type != eniTypePrimary {
			reusable = append(reusable, e)
		}
	}
	if len(reusable) > count {
		reusable = reusable[:count]
	}
	return reusable
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
