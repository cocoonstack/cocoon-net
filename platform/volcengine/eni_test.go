package volcengine

import (
	"encoding/json"
	"testing"
)

const (
	eniList1Primary7Secondary = `{
  "Result": {
    "NetworkInterfaceSets": [
      {"NetworkInterfaceId": "eni-primary", "Type": "primary", "PrivateIpSets": {"PrivateIpSet": [{"Primary": true, "PrivateIpAddress": "10.0.0.1"}]}},
      {"NetworkInterfaceId": "eni-1", "Type": "secondary", "PrivateIpSets": {"PrivateIpSet": [{"Primary": false, "PrivateIpAddress": "10.0.0.2"}]}},
      {"NetworkInterfaceId": "eni-2", "Type": "secondary", "PrivateIpSets": {"PrivateIpSet": [{"Primary": false, "PrivateIpAddress": "10.0.0.3"}]}},
      {"NetworkInterfaceId": "eni-3", "Type": "secondary", "PrivateIpSets": {"PrivateIpSet": [{"Primary": false, "PrivateIpAddress": "10.0.0.4"}]}},
      {"NetworkInterfaceId": "eni-4", "Type": "secondary", "PrivateIpSets": {"PrivateIpSet": [{"Primary": false, "PrivateIpAddress": "10.0.0.5"}]}},
      {"NetworkInterfaceId": "eni-5", "Type": "secondary", "PrivateIpSets": {"PrivateIpSet": [{"Primary": false, "PrivateIpAddress": "10.0.0.6"}]}},
      {"NetworkInterfaceId": "eni-6", "Type": "secondary", "PrivateIpSets": {"PrivateIpSet": [{"Primary": false, "PrivateIpAddress": "10.0.0.7"}]}},
      {"NetworkInterfaceId": "eni-7", "Type": "secondary", "PrivateIpSets": {"PrivateIpSet": [{"Primary": false, "PrivateIpAddress": "10.0.0.8"}]}}
    ]
  }
}`

	eniList1Primary3Secondary = `{
  "Result": {
    "NetworkInterfaceSets": [
      {"NetworkInterfaceId": "eni-primary", "Type": "primary", "PrivateIpSets": {"PrivateIpSet": [{"Primary": true, "PrivateIpAddress": "10.0.0.1"}]}},
      {"NetworkInterfaceId": "eni-1", "Type": "secondary", "PrivateIpSets": {"PrivateIpSet": [{"Primary": false, "PrivateIpAddress": "10.0.0.2"}]}},
      {"NetworkInterfaceId": "eni-2", "Type": "secondary", "PrivateIpSets": {"PrivateIpSet": [{"Primary": false, "PrivateIpAddress": "10.0.0.3"}]}},
      {"NetworkInterfaceId": "eni-3", "Type": "secondary", "PrivateIpSets": {"PrivateIpSet": [{"Primary": false, "PrivateIpAddress": "10.0.0.4"}]}}
    ]
  }
}`
)

func TestReusableENIs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fixture string
		count   int
		want    int
	}{
		{"exact match runs zero shortfall", eniList1Primary7Secondary, 7, 7},
		{"fewer than count returns all", eniList1Primary3Secondary, 7, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			enis := unmarshalENIList(t, tt.fixture)
			if got := len(reusableENIs(enis, tt.count)); got != tt.want {
				t.Errorf("got %d reusable ENIs, want %d", got, tt.want)
			}
		})
	}
}

func TestIPShortfall_FullENIIsZero(t *testing.T) {
	t.Parallel()

	eni := newENIWithSecondaryIPs("eni-full", ipsPerENI)

	var existing int
	for _, pip := range eni.PrivateIPSets.PrivateIPSet {
		if !pip.Primary {
			existing++
		}
	}
	if shortfall := ipsPerENI - existing; shortfall != 0 {
		t.Errorf("got shortfall %d, want 0", shortfall)
	}
}

func unmarshalENIList(t *testing.T, fixture string) []networkInterface {
	t.Helper()

	var resp struct {
		Result struct {
			NetworkInterfaceSets []networkInterface `json:"NetworkInterfaceSets"`
		} `json:"Result"`
	}
	if err := json.Unmarshal([]byte(fixture), &resp); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	return resp.Result.NetworkInterfaceSets
}

func newENIWithSecondaryIPs(id string, n int) networkInterface {
	eni := networkInterface{NetworkInterfaceID: id, Type: "secondary"}
	for range n {
		eni.PrivateIPSets.PrivateIPSet = append(eni.PrivateIPSets.PrivateIPSet, struct {
			Primary          bool   `json:"Primary"`
			PrivateIPAddress string `json:"PrivateIpAddress"`
		}{PrivateIPAddress: "10.0.1.1"})
	}
	return eni
}
