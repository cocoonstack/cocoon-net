//go:build linux

package node

import (
	"errors"
	"syscall"
	"testing"

	"github.com/vishvananda/netlink"
)

func TestLinkMTUReadsTheLoopback(t *testing.T) {
	mtu, err := linkMTU("lo")
	if err != nil || mtu <= 0 {
		t.Fatalf("linkMTU(lo) = %d, %v, want a positive MTU", mtu, err)
	}
	if _, err := linkMTU("no-such-nic"); err == nil {
		t.Fatal("linkMTU(no-such-nic) = nil error, want the lookup failure")
	}
}

func TestSetupBridgeFollowsTheMTU(t *testing.T) {
	if err := netlink.LinkAdd(&netlink.Bridge{Name: BridgeName}); err != nil {
		if errors.Is(err, syscall.EPERM) {
			t.Skipf("no CAP_NET_ADMIN: %v", err)
		}
		t.Fatalf("pre-create %s: %v", BridgeName, err)
	}
	t.Cleanup(func() {
		if link, err := netlink.LinkByName(BridgeName); err == nil {
			_ = netlink.LinkDel(link)
		}
	})
	for _, mtu := range []int{1460, 1500} {
		if err := setupBridge(t.Context(), "10.99.0.1", "10.99.0.0/24", mtu); err != nil {
			t.Fatalf("setupBridge(mtu %d): %v", mtu, err)
		}
		link, err := netlink.LinkByName(BridgeName)
		if err != nil {
			t.Fatalf("lookup %s: %v", BridgeName, err)
		}
		if got := link.Attrs().MTU; got != mtu {
			t.Fatalf("%s mtu = %d after setupBridge(%d)", BridgeName, got, mtu)
		}
	}
}
