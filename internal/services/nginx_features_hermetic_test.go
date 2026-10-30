package services

import (
	"context"
	"errors"
)

// Tests never measure the machine's own nginx: a CI runner or developer box
// with nginx installed would otherwise change every rendered HTTPS form.
// Tests that need a version set it on the generator (SetNginxFeatures) or
// replace these seams themselves and restore them.
// Testler makinenin kendi nginx'ini ölçmez; sürüm gerekiyorsa açıkça verilir.
func init() {
	nginxFeaturesRun = func(context.Context, string) ([]byte, error) {
		return nil, errors.New("tests measure no host nginx")
	}
}
