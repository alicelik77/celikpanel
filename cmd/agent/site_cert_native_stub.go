//go:build !linux

package main

import "errors"

func publishNativeSiteDeployHook() error {
	return errors.New("the certbot deploy hook can only be written on Linux")
}
