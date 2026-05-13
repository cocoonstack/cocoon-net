package node

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/projecteru2/core/log"
)

const procSysBase = "/proc/sys"

// setupSysctl applies the cocoon-net sysctl tuning. Failures on
// individual keys are logged and skipped — per-interface keys can
// vanish out-of-band when an ENI is detached, and aborting startup
// on the first stale key would leave the daemon unrunnable. The
// iptables/route layer will surface a real connectivity problem if
// one matters.
func setupSysctl(ctx context.Context, primaryNIC string, secondaryNICs []string) {
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
			logger.Warnf(ctx, "write sysctl %s=%s: %v (continuing)", key, val, err)
			continue
		}
	}
	logger.Info(ctx, "sysctl parameters applied")
}
