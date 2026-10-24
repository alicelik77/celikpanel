//go:build !linux

package main

import (
	"context"

	"github.com/alicelik/celikpanel/internal/transport"
)

type roundcubeUpdateOps struct{}

func hostRoundcubeUpdateOps() roundcubeUpdateOps { return roundcubeUpdateOps{} }

func updateRoundcubeTo(_ context.Context, version string, _ roundcubeUpdateOps) transport.RoundcubeUpdateResponse {
	return transport.RoundcubeUpdateResponse{To: version, ErrorCode: "UNSUPPORTED", Error: errRoundcubeLifecycleUnsupported.Error()}
}
