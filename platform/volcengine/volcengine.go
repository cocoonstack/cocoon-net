// Package volcengine implements the CloudPlatform interface for Volcengine.
package volcengine

import (
	"context"
	"fmt"

	"github.com/cocoonstack/cocoon-net/platform"
)

const (
	metadataBase = "http://100.96.0.96/latest/meta-data"
	enisPerNode  = 7
	ipsPerENI    = 20

	eniTypePrimary = "primary"
)

var _ platform.CloudPlatform = (*Volcengine)(nil)

// Volcengine implements CloudPlatform; the struct is empty because credentials live in the env of the `ve` child binary.
type Volcengine struct{}

// New loads credentials from env or ~/.volcengine/config.json.
func New(ctx context.Context) (*Volcengine, error) {
	if err := loadEnv(ctx); err != nil {
		return nil, fmt.Errorf("load volcengine env: %w", err)
	}
	return &Volcengine{}, nil
}

func (v *Volcengine) Name() string { return platform.PlatformVolcengine }
