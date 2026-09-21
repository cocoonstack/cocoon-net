package node

import (
	"slices"
	"testing"
)

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
