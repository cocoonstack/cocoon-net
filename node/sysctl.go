package node

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/projecteru2/core/log"
)

const procSysBase = "/proc/sys"

func setupSysctl(ctx context.Context, primaryNIC string, secondaryNICs []string) error {
	logger := log.WithFunc("node.setupSysctl")

	settings := map[string]string{
		"net.ipv4.ip_forward":             "1",
		"net.ipv4.conf.all.rp_filter":     "0",
		"net.ipv4.conf.cni0.rp_filter":    "0",
		"net.ipv4.conf.default.rp_filter": "0",
	}
	if primaryNIC != "" {
		settings["net.ipv4.conf."+primaryNIC+".rp_filter"] = "0"
	}
	for _, iface := range secondaryNICs {
		settings["net.ipv4.conf."+iface+".rp_filter"] = "0"
	}

	for key, val := range settings {
		path := filepath.Join(procSysBase, strings.ReplaceAll(key, ".", "/"))
		if err := os.WriteFile(path, []byte(val), filePerm); err != nil { //nolint:gosec // sysctl tuning
			return fmt.Errorf("write sysctl %s=%s: %w", key, val, err)
		}
	}
	logger.Info(ctx, "sysctl parameters applied")
	return nil
}
