package volcengine

import (
	"context"

	"github.com/cocoonstack/cocoon-net/platform"
)

// Adopt is a no-op: the VPC routes the VM subnet to the secondary ENIs, so no host route hijacks it.
func (v *Volcengine) Adopt(context.Context, *platform.Config) error { return nil }
