//go:build !linux

package main

import (
	"errors"

	"github.com/alicelik/celikpanel/internal/transport"
)

type wordpressConfigFile struct{}

func (f *wordpressConfigFile) close() {}

func (f *wordpressConfigFile) writePassword(string) error {
	return errors.New("wp-config.php is updated only on Linux")
}

func openWordPressConfigForRotation(transport.DatabaseUserApp, string) (*wordpressConfigFile, string) {
	return nil, "wp-config.php is updated only on Linux"
}
