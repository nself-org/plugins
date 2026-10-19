package signing

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/secrets"
)

// LoadKeyIgnoringStoredID returns the id of the stored key without the stored-id check.
func LoadKeyIgnoringStoredID(ctx context.Context, res secrets.ValueResolver) (string, error) {
	k, err := loadKey(ctx, res, "t", false)
	if err != nil {
		return "", err
	}
	defer k.Zero()
	return k.ID, nil
}
