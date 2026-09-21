package node

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestCNIConflistCarriesThePrimaryNICMTU(t *testing.T) {
	encoded, err := cniConflist(1460)
	if err != nil {
		t.Fatalf("cniConflist: %v", err)
	}
	var conflist struct {
		Plugins []struct {
			Type string `json:"type"`
			MTU  int    `json:"mtu"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(encoded, &conflist); err != nil {
		t.Fatalf("decode conflist: %v", err)
	}
	if len(conflist.Plugins) != 1 || conflist.Plugins[0].Type != "bridge" || conflist.Plugins[0].MTU != 1460 {
		t.Fatalf("plugins = %+v, want one bridge plugin with mtu 1460", conflist.Plugins)
	}
}

func TestUsableSecondaryNICsKeepsThePresentOnes(t *testing.T) {
	got, err := usableSecondaryNICs(t.Context(), []string{"eth1", "eth2", "eth3"}, []string{"eth1", "eth3"})
	if err != nil || !slices.Equal(got, []string{"eth1", "eth3"}) {
		t.Fatalf("usableSecondaryNICs = %v, %v, want the present NICs", got, err)
	}
	if _, err := usableSecondaryNICs(t.Context(), []string{"eth1", "eth2"}, nil); err == nil {
		t.Fatal("no secondary NIC present must still fail")
	}
	if got, err := usableSecondaryNICs(t.Context(), nil, nil); err != nil || len(got) != 0 {
		t.Fatalf("a node without secondary NICs = %v, %v", got, err)
	}
}
