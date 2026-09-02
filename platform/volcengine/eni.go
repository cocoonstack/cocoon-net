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
	orphanDeleteTimeout    = 15 * time.Second
)

type networkInterface struct {
	NetworkInterfaceID string `json:"NetworkInterfaceId"`
	DeviceID           string `json:"DeviceId"`
	MacAddress         string `json:"MacAddress"`
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
	result := selectReusableENIs(existing, count)
	if len(result) > 0 {
		logger.Infof(ctx, "reusing %d existing ENI(s)", len(result))
	}

	for i := len(result) + 1; i <= count; i++ {
		out, err := runVe(
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
			delCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), orphanDeleteTimeout)
			if _, delErr := runVe(delCtx, "vpc", "DeleteNetworkInterface", "--NetworkInterfaceId", eniID); delErr != nil {
				logger.Warnf(ctx, "delete orphan ENI %s: %v", eniID, delErr)
			}
			cancel()
			return result, err
		}

		_, attachErr := runVe(
			ctx, "vpc", "AttachNetworkInterface",
			"--NetworkInterfaceId", eniID,
			"--InstanceId", instanceID,
		)
		if attachErr != nil {
			// best-effort delete so one attach failure neither leaks quota nor stalls the pool build
			logger.Warnf(ctx, "attach ENI %s: %v", eniID, attachErr)
			if _, delErr := runVe(
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

func selectReusableENIs(enis []networkInterface, count int) []networkInterface {
	var reusable []networkInterface
	for _, e := range enis {
		if e.Type != eniTypePrimary {
			reusable = append(reusable, e)
		}
	}
	return reusable[:min(len(reusable), count)]
}

func assignSecondaryIPs(ctx context.Context, eniID string, count int) ([]string, error) {
	out, err := runVe(
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
	return describeENIs(ctx, "--InstanceId", instanceID)
}

func listENIsByIDs(ctx context.Context, eniIDs []string) ([]networkInterface, error) {
	args := make([]string, 0, len(eniIDs)*2)
	for i, id := range eniIDs {
		args = append(args, fmt.Sprintf("--NetworkInterfaceIds.%d", i+1), id)
	}
	return describeENIs(ctx, args...)
}

func describeENIs(ctx context.Context, filters ...string) ([]networkInterface, error) {
	args := append([]string{"vpc", "DescribeNetworkInterfaces"}, filters...)
	args = append(args, "--PageSize", "100")
	out, err := runVe(ctx, args...)
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
