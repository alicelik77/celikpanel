//go:build !linux

package main

import (
	"context"

	"github.com/alicelik/celikpanel/internal/hostplatform"
)

func pdnsUntestedInstalledVersion(context.Context, hostplatform.Profile, dnsEngineStateReceipt) (string, error) {
	return "", nil
}

func requirePDNSNativeSeriesForManagement(context.Context, hostplatform.Profile, dnsEngineStateReceipt) error {
	return nil
}
