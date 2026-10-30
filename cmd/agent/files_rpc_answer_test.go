package main

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The file-manager RPCs answer "not there" and "already there" with the exact
// texts the Panel classifies (final alpha.113 acceptance run 2: a missing
// path reached the browser as a bare 500).
func TestSiteFileAnswerClassifiesMissingAndExisting(t *testing.T) {
	other := errors.New("input/output error")
	for _, tc := range []struct {
		name string
		in   error
		want error
	}{
		{"nil", nil, nil},
		{"missing entry", syscall.ENOENT, errors.New(transport.SiteFilePathMissing)},
		{"missing via os", os.ErrNotExist, errors.New(transport.SiteFilePathMissing)},
		{"component is a file", syscall.ENOTDIR, errors.New(transport.SiteFilePathMissing)},
		{"web folder gone", fmt.Errorf("open immutable site document root: %w", syscall.ENOENT), errors.New(transport.SiteFileRootMissing)},
		{"name taken", syscall.EEXIST, errors.New(transport.SiteFilePathExists)},
		{"other", other, other},
		{"permission", os.ErrPermission, os.ErrPermission},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := siteFileAnswer(tc.in)
			if (got == nil) != (tc.want == nil) || (got != nil && got.Error() != tc.want.Error()) {
				t.Fatalf("siteFileAnswer(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
