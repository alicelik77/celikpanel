package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func mailProfileUIContractSource(t *testing.T) string {
	t.Helper()
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal(`resolve mail-profile frontend contract test path`)
	}
	path := filepath.Clean(filepath.Join(
		filepath.Dir(testFile), `..`, `..`, `web`, `tests`, `mail-profile-ui-contract.test.mjs`,
	))
	content, err := os.ReadFile(path)
	if err != nil {
		path = filepath.Clean(filepath.Join(`..`, `..`, `web`, `tests`, `mail-profile-ui-contract.test.mjs`))
		content, err = os.ReadFile(path)
		if err != nil {
			t.Fatalf(`read mail-profile UI contract: %v`, err)
		}
	}
	return string(content)
}

func TestMailProfileFrontendSnapshotFailsClosed(t *testing.T) {
	source := componentOperationSource(t)
	decoder := sourceSection(
		t,
		source,
		`function decodeManagedMailProfiles(`,
		`// A terminal operation may unlock the page only after`,
	)
	for _, required := range []string{
		`value.length !== MAIL_PROFILE_IDS.length`,
		`profileIDs.has(id)`,
		`profile.services.length === 0`,
		`serviceIDs.has(serviceID)`,
		`new Set(profile.services).size !== profile.services.length`,
		`profile.available !== (`,
		`status === 'blocked'`,
		`profile.blocked_reason`,
		`profile.warning`,
	} {
		if !strings.Contains(decoder, required) {
			t.Fatalf(`strict mail-profile decoder is missing %q`, required)
		}
	}
	for _, profileID := range []string{`core-mail`, `webmail`, `protected-mail`} {
		if !strings.Contains(source, profileID) {
			t.Fatalf(`closed mail-profile id set is missing %q`, profileID)
		}
	}
}

func TestMailProfileFrontendRequiresFreshFullTerminalProof(t *testing.T) {
	source := componentOperationSource(t)
	proof := sourceSection(
		t,
		source,
		`function decodeVerifiedMailProfileResult(`,
		`function readSessionValue(`,
	)
	for _, required := range []string{
		`operation.kind !== 'mail_profile_install'`,
		`profile.status !== 'complete'`,
		`result.success !== true`,
		`result.profile_id !== operation.service_id`,
		`stringArrayMatchesSet(result.services, profile.services)`,
		`stringArrayMatchesSet(result.completed_services, profile.services)`,
		`tls.configured !== true`,
		`tls.fallback_only !== (tls.sni_count === 0)`,
		`result.submission_configured !== true`,
		`tls.fallback_only && warnings.length === 0`,
	} {
		if !strings.Contains(proof, required) {
			t.Fatalf(`mail-profile terminal proof is missing %q`, required)
		}
	}
	if !strings.Contains(source, `scannedAt < finishedAt`) {
		t.Fatal(`mail-profile success must retain the common fresh-scan time proof`)
	}
}

func TestMailProfileFrontendRecoveryAndFallbackStayKindBound(t *testing.T) {
	source := componentOperationSource(t)
	for _, required := range []string{
		`const OPERATION_RECOVERY_VERSION = 3;`,
		`function isMailProfileID(value: unknown)`,
		`value.version !== 2`,
		`operation_kind: operationKind`,
		`operation.kind === marker.operation_kind`,
		`operation.kind !== 'mail_profile_install'`,
		`!isMailProfileID(serviceID)`,
		`!isMailProfileID(value.service_id)`,
		`!isMailProfileID(operation.service_id)`,
		`'/api/v1/service/profile/install'`,
		`profile_id: request.serviceId`,
		`verifiedProfileResult?.fallbackOnly`,
		`showToast('warning', t('services.mailProfiles.fallbackWarning'`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf(`mail-profile recovery contract is missing %q`, required)
		}
	}
	decoder := sourceSection(
		t,
		source,
		`export function decodeOperationRecoveryMarker(`,
		`function readStoredRecoveryMarker(`,
	)
	guardIndex := strings.Index(decoder, `!isMailProfileID(value.service_id)`)
	createIndex := strings.Index(decoder, `return createOperationRecoveryMarker({`)
	if guardIndex < 0 || createIndex < 0 || guardIndex > createIndex {
		t.Fatal(`unknown V3 mail-profile marker must fail closed before marker reconstruction`)
	}
	nodeContract := mailProfileUIContractSource(t)
	for _, required := range []string{
		`const unknownCall = {`,
		`serviceId: 'unknown-mail'`,
		`operationKind: 'mail_profile_install'`,
		`createOperationRecoveryMarker(unknownCall, 1, requestID)`,
		`const unknownV3Marker = {`,
		`service_id: 'unknown-mail'`,
		`decodeOperationRecoveryMarker(JSON.stringify(unknownV3Marker))`,
	} {
		if !strings.Contains(nodeContract, required) {
			t.Fatalf(`runtime unknown V3 marker contract is missing %q`, required)
		}
	}
}

// webSource reads one file of the web interface for a source contract.
func webSource(t *testing.T, parts ...string) string {
	t.Helper()
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal(`resolve web source path`)
	}
	path := filepath.Clean(filepath.Join(append([]string{filepath.Dir(testFile), `..`, `..`, `web`, `src`}, parts...)...))
	content, err := os.ReadFile(path)
	if err != nil {
		content, err = os.ReadFile(filepath.Clean(filepath.Join(append([]string{`..`, `..`, `web`, `src`}, parts...)...)))
		if err != nil {
			t.Fatalf(`read %v: %v`, parts, err)
		}
	}
	return string(content)
}

// alpha.94 (D-034): the mail stack cards became the email strip's ready
// bundles. The intent stays: the server names each bundle's components and
// its one action, the page decodes a closed set of states and actions, a mail
// bundle opens the profile's own install dialogue and starts the kind-bound
// operation, and nothing hard-codes which components a bundle has. alpha.95:
// the strip is a fixed chip list; Verify of a component bundle lives in the
// installed bundle's dialogue.
func TestReadyBundleStripUsesServerMembershipAndClosedActions(t *testing.T) {
	decoder := webSource(t, `lib`, `readyBundles.ts`)
	for _, required := range []string{
		`const components = stringList(bundle.components, serviceIDs);`,
		`!missing.every((id) => components.includes(id))`,
		`installed_verified: ['repair', 'none'],`,
		`blocked: ['none'],`,
		`!MAIL_PROFILE_IDS.has(bundle.profile_id)`,
		`bundle.profile_id !== undefined || action === 'repair'`,
	} {
		if !strings.Contains(decoder, required) {
			t.Fatalf(`ready bundle decoder is missing %q`, required)
		}
	}
	strip := webSource(t, `components`, `ReadyBundleStrip.tsx`)
	for _, required := range []string{
		`if (bundle.action_route === 'mail_profile') onMail(bundle);`,
		`else onComponents(bundle);`,
		// alpha.101: Verify is withdrawn; Repair (mail) is the dialogue's one action.
		`onRepair={() => { setDetails(null); onMail(open.bundle); }}`,
		// One step at a time, only for this step's own operation.
		`if (operation.service_id !== step || operation.kind !== 'service_install') stop(run, 'foreign');`,
		`if (tracker.locked) return;`,
		`if (tracker.failure) {`,
		`if (tracker.catalogSnapshot === baselineRef.current) return;`,
		`if (sentRef.current === key) return;`,
		`const closed = await gate();`,
	} {
		if !strings.Contains(strip, required) {
			t.Fatalf(`ready bundle strip is missing %q`, required)
		}
	}
	for _, forbidden := range []string{`['postfix'`, `['dovecot'`, `['nginx'`, `onVerify`, `verifyHeld`} {
		if strings.Contains(strip, forbidden) {
			t.Fatalf(`ready bundle strip must not hard-code %q`, forbidden)
		}
	}
	source := serviceListSource(t)
	for _, required := range []string{
		`openProfilePlan(profile);`,
		`operationKind: 'mail_profile_install'`,
		`navigate('/dns-infrastructure')`,
		`bundleSequence.begin(bundle, choices);`,
		// The client gate of the mail actions before the DNS identity is
		// ready, beside the Panel's own refusal (the cards' former
		// "disabled={disabled || !dnsIdentityReady || !actionable}").
		`if (!dnsIdentityReady) return;`,
		`disabled={mutationControlsDisabled || componentOperation.locked || (cat === 'email' && !dnsIdentityReady)}`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf(`Components page is missing %q`, required)
		}
	}
	if strings.Contains(source, `function MailProfileCards(`) {
		t.Fatal(`the mail stack cards must be gone from the Components page`)
	}
}
