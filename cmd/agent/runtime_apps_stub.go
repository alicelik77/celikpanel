//go:build !linux

package main

import (
	"errors"

	"github.com/alicelik/celikpanel/internal/siteruntime"
)

func readRuntimeAppEntryFacts(string, string) (siteruntime.FileFacts, error) {
	return siteruntime.FileFacts{}, errors.New("runtime apps run only on Linux")
}

func runRuntimeAppAsUser(serviceAppAccount, string, ...string) ([]byte, error) {
	return nil, errors.New("runtime apps run only on Linux")
}
