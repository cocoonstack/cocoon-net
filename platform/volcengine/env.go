package volcengine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/projecteru2/core/log"
)

// envConfig carries the Volcengine credentials and region resolved at
// construction time. It is exported as a field of Volcengine rather than
// stashed in package-global env vars so state is explicit.
type envConfig struct {
	AccessKeyID     string
	SecretAccessKey string
	Region          string
}

// loadEnv resolves credentials from env vars first, then from
// ~/.volcengine/config.json. A missing home dir or config file is NOT an
// error — the downstream `ve` CLI has its own fallbacks. The resolved
// values are also exported back into the process environment so child
// subprocesses (the `ve` binary) inherit them.
func loadEnv(ctx context.Context) (*envConfig, error) {
	logger := log.WithFunc("volcengine.loadEnv")

	cfg := &envConfig{
		AccessKeyID:     os.Getenv("VOLCENGINE_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("VOLCENGINE_SECRET_ACCESS_KEY"),
		Region:          os.Getenv("VOLCENGINE_REGION"),
	}
	if cfg.AccessKeyID != "" {
		logger.Debug(ctx, "credentials loaded from environment")
		return cfg, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		logger.Debugf(ctx, "no home dir for config file: %v", err)
		return cfg, nil
	}
	cfgPath := filepath.Join(home, ".volcengine", "config.json")
	data, err := os.ReadFile(cfgPath) //nolint:gosec // standard config file path
	if err != nil {
		if os.IsNotExist(err) {
			logger.Debugf(ctx, "no volcengine config file at %s", cfgPath)
			return cfg, nil
		}
		return nil, fmt.Errorf("read %s: %w", cfgPath, err)
	}

	var file struct {
		AccessKeyID     string `json:"access_key_id"`
		SecretAccessKey string `json:"secret_access_key"`
		Region          string `json:"region"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse %s: %w", cfgPath, err)
	}
	if file.AccessKeyID != "" {
		cfg.AccessKeyID = file.AccessKeyID
		cfg.SecretAccessKey = file.SecretAccessKey
		_ = os.Setenv("VOLCENGINE_ACCESS_KEY_ID", file.AccessKeyID)
		_ = os.Setenv("VOLCENGINE_SECRET_ACCESS_KEY", file.SecretAccessKey)
	}
	if file.Region != "" {
		cfg.Region = file.Region
		_ = os.Setenv("VOLCENGINE_REGION", file.Region)
	}
	logger.Debugf(ctx, "credentials loaded from %s", cfgPath)
	return cfg, nil
}
