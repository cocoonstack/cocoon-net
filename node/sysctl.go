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

// setupSysctl applies cocoon-net sysctl tuning. Global keys are required;
// per-interface keys are best-effort (an ENI can be detached out-of-band).
func setupSysctl(ctx context.Context, primaryNIC string, secondaryNICs []string) error {
	logger := log.WithFunc("node.setupSysctl")

	required := map[string]string{
		"net.ipv4.ip_forward":             "1",
		"net.ipv4.conf.all.rp_filter":     "0",
		"net.ipv4.conf.default.rp_filter": "0",
		"net.ipv4.conf.cni0.rp_filter":    "0",
	}
	for key, val := range required {
		if err := writeSysctl(key, val); err != nil {
			return fmt.Errorf("write sysctl %s=%s: %w", key, val, err)
		}
	}

	perIface := make([]string, 0, 1+len(secondaryNICs))
	if primaryNIC != "" {
		perIface = append(perIface, primaryNIC)
	}
	perIface = append(perIface, secondaryNICs...)
	for _, iface := range perIface {
		key := "net.ipv4.conf." + iface + ".rp_filter"
		if err := writeSysctl(key, "0"); err != nil {
			logger.Warnf(ctx, "write sysctl %s=0: %v (continuing)", key, err)
		}
	}

	logger.Info(ctx, "sysctl parameters applied")
	return nil
}

func writeSysctl(key, val string) error {
	return os.WriteFile(sysctlPath(key), []byte(val), filePerm) //nolint:gosec // sysctl tuning
}

// sysctlPath maps a dotted sysctl key to its /proc/sys file path.
func sysctlPath(key string) string {
	return filepath.Join(procSysBase, strings.ReplaceAll(key, ".", "/"))
}
