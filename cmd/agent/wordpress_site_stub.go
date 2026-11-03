//go:build !linux

package main

import (
	"context"
	"errors"

	"github.com/alicelik/celikpanel/internal/transport"
)

var wordpressRunAsSite = func(context.Context, transport.WordPressSite, string, []byte, string, ...string) ([]byte, error) {
	return nil, errors.New("WordPress tools are supported only on Linux")
}

func rewriteStagedWordPressConfig(int, *transport.WordPressCloneRequest) error {
	return errors.New("WordPress tools are supported only on Linux")
}
