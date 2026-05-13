package volcengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/projecteru2/core/log"
)

// loadEnv resolves Volcengine credentials from env vars or
// ~/.volcengine/config.json and exports them so the `ve` child binary
// inherits them. Missing home dir or config file is not an error —
// `ve` has its own fallbacks.
func loadEnv(ctx context.Context) error {
	logger := log.WithFunc("volcengine.loadEnv")

	if os.Getenv("VOLCENGINE_ACCESS_KEY_ID") != "" {
		logger.Debug(ctx, "credentials loaded from environment")
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		logger.Debugf(ctx, "no home dir for config file: %v", err)
		return nil
	}
	cfgPath := filepath.Join(home, ".volcengine", "config.json")
	data, err := os.ReadFile(cfgPath) //nolint:gosec // standard config file path
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			logger.Debugf(ctx, "no volcengine config file at %s", cfgPath)
			return nil
		}
		return fmt.Errorf("read %s: %w", cfgPath, err)
	}

	var file struct {
		AccessKeyID     string `json:"access_key_id"`
		SecretAccessKey string `json:"secret_access_key"`
		Region          string `json:"region"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("parse %s: %w", cfgPath, err)
	}
	if file.AccessKeyID != "" {
		_ = os.Setenv("VOLCENGINE_ACCESS_KEY_ID", file.AccessKeyID)
		_ = os.Setenv("VOLCENGINE_SECRET_ACCESS_KEY", file.SecretAccessKey)
	}
	if file.Region != "" {
		_ = os.Setenv("VOLCENGINE_REGION", file.Region)
	}
	logger.Debugf(ctx, "credentials loaded from %s", cfgPath)
	return nil
}
