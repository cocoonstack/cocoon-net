//go:build linux

package node

import (
	"errors"
	"net"
	"testing"

	"github.com/vishvananda/netlink"
)

func TestSetupSecondaryNICsOnlyRaisesDownLinks(t *testing.T) {
	links := map[string]netlink.Link{
		"eth1": &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: "eth1", Flags: net.FlagUp}},
		"eth2": &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: "eth2"}},
	}
	var raised []string
	err := setupSecondaryNICsWith([]string{"eth1", "eth2"}, func(name string) (netlink.Link, error) {
		return links[name], nil
	}, func(link netlink.Link) error {
		raised = append(raised, link.Attrs().Name)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(raised) != 1 || raised[0] != "eth2" {
		t.Fatalf("raised links = %v, want [eth2]", raised)
	}
}

func TestSetupSecondaryNICsFailsOnMissingLink(t *testing.T) {
	want := errors.New("not found")
	err := setupSecondaryNICsWith([]string{"eth1"}, func(string) (netlink.Link, error) {
		return nil, want
	}, func(netlink.Link) error {
		t.Fatal("setUp must not run after lookup failure")
		return nil
	})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want wrapped %v", err, want)
	}
}

func TestSetupSecondaryNICsStopsOnSetUpFailure(t *testing.T) {
	want := errors.New("permission denied")
	err := setupSecondaryNICsWith([]string{"eth1"}, func(name string) (netlink.Link, error) {
		return &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: name}}, nil
	}, func(netlink.Link) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want wrapped %v", err, want)
	}
}
