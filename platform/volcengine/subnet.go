package volcengine

import (
	"context"
	"encoding/json"
	"fmt"
)

func getSecurityGroupID(ctx context.Context, vpcID string) (string, error) {
	out, err := veRun(ctx, "vpc", "DescribeSecurityGroups",
		"--VpcId", vpcID, "--PageSize", "1",
	)
	if err != nil {
		return "", fmt.Errorf("describe security groups: %w", err)
	}
	var resp struct {
		Result struct {
			SecurityGroups []struct {
				SecurityGroupID string `json:"SecurityGroupId"`
			} `json:"SecurityGroups"`
		} `json:"Result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("parse security groups: %w", err)
	}
	if len(resp.Result.SecurityGroups) == 0 {
		return "", fmt.Errorf("no security groups found in VPC %s", vpcID)
	}
	return resp.Result.SecurityGroups[0].SecurityGroupID, nil
}

func ensureSubnet(ctx context.Context, vpcID, cidr, name string) (string, error) {
	out, err := veRun(ctx, "vpc", "DescribeSubnets",
		"--VpcId", vpcID, "--PageSize", "100",
	)
	if err != nil {
		return "", fmt.Errorf("describe subnets: %w", err)
	}
	var listResp struct {
		Result struct {
			Subnets []struct {
				SubnetID  string `json:"SubnetId"`
				CIDRBlock string `json:"CidrBlock"`
			} `json:"Subnets"`
		} `json:"Result"`
	}
	if unmarshalErr := json.Unmarshal(out, &listResp); unmarshalErr != nil {
		return "", fmt.Errorf("parse describe subnets: %w", unmarshalErr)
	}
	for _, s := range listResp.Result.Subnets {
		if s.CIDRBlock == cidr {
			return s.SubnetID, nil
		}
	}

	zone, err := fetchMeta(ctx, "/placement/availability-zone")
	if err != nil {
		return "", fmt.Errorf("fetch zone: %w", err)
	}

	createOut, err := veRun(ctx, "vpc", "CreateSubnet",
		"--VpcId", vpcID,
		"--CidrBlock", cidr,
		"--ZoneId", zone,
		"--SubnetName", name+"-vms",
	)
	if err != nil {
		return "", fmt.Errorf("create subnet: %w", err)
	}
	var resp struct {
		Result struct {
			SubnetID string `json:"SubnetId"`
		} `json:"Result"`
	}
	if err := json.Unmarshal(createOut, &resp); err != nil {
		return "", fmt.Errorf("parse create subnet response: %w", err)
	}
	return resp.Result.SubnetID, nil
}
