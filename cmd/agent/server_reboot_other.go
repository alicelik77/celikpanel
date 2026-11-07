//go:build !linux

package main

import (
	"errors"

	"github.com/alicelik/celikpanel/internal/transport"
)

type platformServerReboot struct{}

func (platformServerReboot) acquireIdle() (func(), string, string) {
	return nil, transport.ServerRebootUnavailable, "restarting the server is supported only on Linux"
}

func (platformServerReboot) reboot() error {
	return errors.New("restarting the server is supported only on Linux")
}
