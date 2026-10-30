package mailrenewalkit

import (
	"strings"
	"testing"
)

// TestEmbeddedFilesHaveNoCarriageReturns is the sibling of the nginx template
// and migration checks: the deploy hooks are shell scripts installed into
// /etc/letsencrypt/renewal-hooks/deploy, and a Windows checkout used to embed
// them with CRLF (`#!/bin/sh\r` is a missing interpreter). .gitattributes now
// pins them to LF; this fails a build that embedded them otherwise.
//
// Gömülü kanca ve birimler CRLF taşırsa kanca çalışmaz; test bunu yakalar.
func TestEmbeddedFilesHaveNoCarriageReturns(t *testing.T) {
	for name, body := range map[string]string{
		"internal/mailrenewalkit/deploy-hook":        hookTemplate,
		"internal/mailrenewalkit/legacy-deploy-hook": string(legacyHook),
		"internal/mailrenewalkit/renewal.service":    serviceTemplate,
		"internal/mailrenewalkit/renewal.timer":      timerTemplate,
	} {
		if body == "" {
			t.Errorf("%s is not embedded", name)
			continue
		}
		if offset := strings.IndexByte(body, '\r'); offset >= 0 {
			t.Errorf("%s has a CR byte at line %d: the checkout converted it to CRLF. "+
				"Keep it LF (.gitattributes `text eol=lf`), then re-checkout the file and rebuild.",
				name, 1+strings.Count(body[:offset], "\n"))
		}
	}
}
