package main

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

type buildGateOperation struct {
	name string
	run  func(expectedBuildCommit string) string
}

func protectedBuildGateOperations() []buildGateOperation {
	agent := &Agent{}
	return []buildGateOperation{
		{name: "StartMailEnrollmentV1", run: func(expected string) string {
			var response transport.MailEnrollmentStartResponse
			err := agent.StartMailEnrollmentV1(&transport.MailEnrollmentStartRequest{MailEnrollmentRequest: transport.MailEnrollmentRequest{RequestID: strings.Repeat("a", 32), OwnerID: strings.Repeat("b", 32), Generation: strings.Repeat("c", 64)}, ExpectedBuildCommit: expected}, &response)
			if err == nil {
				return ""
			}
			return err.Error()
		}}, {name: "ContinueMailEnrollmentV1", run: func(expected string) string {
			var response transport.MailEnrollmentStartResponse
			err := agent.ContinueMailEnrollmentV1(&transport.MailEnrollmentStartRequest{MailEnrollmentRequest: transport.MailEnrollmentRequest{RequestID: strings.Repeat("a", 32), OwnerID: strings.Repeat("b", 32), Generation: strings.Repeat("c", 64)}, ExpectedBuildCommit: expected}, &response)
			if err == nil {
				return ""
			}
			return err.Error()
		}},
		// alpha.127: a Node.js site's install step.
		{name: "NodeSiteInstallV1", run: func(expected string) string {
			var response transport.NodeSiteInstallResponse
			_ = agent.NodeSiteInstallV1(&transport.NodeSiteInstallRequest{ExpectedBuildCommit: expected}, &response)
			return response.Error
		}},
		// alpha.118: WordPress tools and the malware quarantine.
		{name: "WordPressUpdateV1", run: func(expected string) string {
			var response transport.WordPressUpdateResponse
			_ = agent.WordPressUpdateV1(&transport.WordPressUpdateRequest{ExpectedBuildCommit: expected}, &response)
			return response.Error
		}},
		{name: "WordPressCloneV1", run: func(expected string) string {
			var response transport.WordPressCloneResponse
			_ = agent.WordPressCloneV1(&transport.WordPressCloneRequest{ExpectedBuildCommit: expected}, &response)
			return response.Error
		}},
		{name: "MalwareQuarantineV1", run: func(expected string) string {
			var response transport.MalwareQuarantineResponse
			_ = agent.MalwareQuarantineV1(&transport.MalwareQuarantineRequest{ExpectedBuildCommit: expected}, &response)
			return response.Error
		}},
		{name: "MalwareRestoreV1", run: func(expected string) string {
			var response transport.MalwareQuarantineItemResponse
			_ = agent.MalwareRestoreV1(&transport.MalwareQuarantineItemRequest{ExpectedBuildCommit: expected}, &response)
			return response.Error
		}},
		{name: "MalwareDeleteQuarantinedV1", run: func(expected string) string {
			var response transport.MalwareQuarantineItemResponse
			_ = agent.MalwareDeleteQuarantinedV1(&transport.MalwareQuarantineItemRequest{ExpectedBuildCommit: expected}, &response)
			return response.Error
		}},
		// alpha.118: AI and automation service apps (added at integration;
		// each refuses before its lock, its target or any host change).
		{name: "ServiceAppPrepareV1", run: func(expected string) string {
			var response transport.ServiceAppStepResponse
			_ = agent.ServiceAppPrepareV1(&transport.ServiceAppPrepareRequest{ExpectedBuildCommit: expected}, &response)
			return response.Error
		}},
		{name: "ServiceAppActivateV1", run: func(expected string) string {
			var response transport.ServiceAppStepResponse
			_ = agent.ServiceAppActivateV1(&transport.ServiceAppActivateRequest{ExpectedBuildCommit: expected}, &response)
			return response.Error
		}},
		{name: "ServiceAppFinalizeV1", run: func(expected string) string {
			var response transport.ServiceAppStepResponse
			_ = agent.ServiceAppFinalizeV1(&transport.ServiceAppActivateRequest{ExpectedBuildCommit: expected}, &response)
			return response.Error
		}},
		{name: "ServiceAppControlV1", run: func(expected string) string {
			var response transport.ServiceAppStepResponse
			_ = agent.ServiceAppControlV1(&transport.ServiceAppControlRequest{ExpectedBuildCommit: expected}, &response)
			return response.Error
		}},
		{name: "ServiceAppSecretsV1", run: func(expected string) string {
			var response transport.ServiceAppStepResponse
			_ = agent.ServiceAppSecretsV1(&transport.ServiceAppSecretsRequest{ExpectedBuildCommit: expected}, &response)
			return response.Error
		}},
		{name: "ServiceAppRemoveV1", run: func(expected string) string {
			var response transport.ServiceAppRemoveResponse
			_ = agent.ServiceAppRemoveV1(&transport.ServiceAppRemoveRequest{ExpectedBuildCommit: expected}, &response)
			return response.Error
		}},
		{name: "SetServiceAppSuspensionV1", run: func(expected string) string {
			var response transport.SetAppUnitSuspensionResponse
			_ = agent.SetServiceAppSuspensionV1(&transport.SetServiceAppSuspensionRequest{ExpectedBuildCommit: expected}, &response)
			return response.Error
		}},
		// alpha.119: runtime apps (uploaded programs); each refuses before its
		// lock, its target or any host change.
		{name: "RuntimeAppRenderV1", run: func(expected string) string {
			var response transport.RuntimeAppRenderResponse
			_ = agent.RuntimeAppRenderV1(&transport.RuntimeAppRenderRequest{ExpectedBuildCommit: expected}, &response)
			return response.Error
		}},
		{name: "RuntimeAppControlV1", run: func(expected string) string {
			var response transport.RuntimeAppStepResponse
			_ = agent.RuntimeAppControlV1(&transport.RuntimeAppControlRequest{ExpectedBuildCommit: expected}, &response)
			return response.Error
		}},
		{name: "RuntimeAppEnvV1", run: func(expected string) string {
			var response transport.RuntimeAppStepResponse
			_ = agent.RuntimeAppEnvV1(&transport.RuntimeAppEnvRequest{ExpectedBuildCommit: expected}, &response)
			return response.Error
		}},
		{name: "RuntimeAppRemoveV1", run: func(expected string) string {
			var response transport.RuntimeAppRemoveResponse
			_ = agent.RuntimeAppRemoveV1(&transport.RuntimeAppRemoveRequest{ExpectedBuildCommit: expected}, &response)
			return response.Error
		}},
		{name: "RuntimeAppBuildV1", run: func(expected string) string {
			var response transport.RuntimeAppBuildResponse
			_ = agent.RuntimeAppBuildV1(&transport.RuntimeAppBuildRequest{ExpectedBuildCommit: expected}, &response)
			return response.Error
		}},
		{name: "RuntimeAppBuildAdvanceV1", run: func(expected string) string {
			var response transport.RuntimeAppBuildResponse
			_ = agent.RuntimeAppBuildAdvanceV1(&transport.RuntimeAppBuildAdvanceRequest{ExpectedBuildCommit: expected}, &response)
			return response.Error
		}},
		{name: "SetRuntimeAppSuspensionV1", run: func(expected string) string {
			var response transport.SetAppUnitSuspensionResponse
			_ = agent.SetRuntimeAppSuspensionV1(&transport.SetRuntimeAppSuspensionRequest{ExpectedBuildCommit: expected}, &response)
			return response.Error
		}},
		{
			name: "ApplyVhost",
			run: func(expected string) string {
				var response ApplyVhostResponse
				_ = agent.ApplyVhost(&ApplyVhostRequest{ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			// alpha.118 (D-044): a full disk makes FTP and SFTP read-only.
			name: "SetFileAccessReadOnlyV1",
			run: func(expected string) string {
				var response transport.SetFileAccessReadOnlyResponse
				_ = agent.SetFileAccessReadOnlyV1(&transport.SetFileAccessReadOnlyRequest{ExpectedBuildCommit: expected, SubscriptionID: 1}, &response)
				return response.Error
			},
		},
		{
			// alpha.118 (D-044): Dovecot enforces the mailbox limits.
			name: "ReconcileMailQuotaV1",
			run: func(expected string) string {
				var response transport.MailQuotaReconcileResponse
				_ = agent.ReconcileMailQuotaV1(&transport.MailQuotaReconcileRequest{ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			// alpha.117: subscription suspension, one native resource per call.
			name: "SetSiteAccountSuspensionV1",
			run: func(expected string) string {
				var response transport.SetSiteAccountSuspensionResponse
				_ = agent.SetSiteAccountSuspensionV1(&transport.SetSiteAccountSuspensionRequest{ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			name: "SetMailDomainSuspensionV1",
			run: func(expected string) string {
				var response transport.SetMailDomainSuspensionResponse
				_ = agent.SetMailDomainSuspensionV1(&transport.SetMailDomainSuspensionRequest{ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			name: "SetDatabaseUserSuspensionV1",
			run: func(expected string) string {
				var response transport.SetDatabaseUserSuspensionResponse
				_ = agent.SetDatabaseUserSuspensionV1(&transport.SetDatabaseUserSuspensionRequest{ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			// alpha.117: credential rotation after a subscription transfer.
			name: "RotateDatabaseUserPasswordV1",
			run: func(expected string) string {
				var response transport.RotateDatabaseUserPasswordResponse
				_ = agent.RotateDatabaseUserPasswordV1(&transport.RotateDatabaseUserPasswordRequest{ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			name: "EndMailSessionsV1",
			run: func(expected string) string {
				var response transport.EndMailSessionsResponse
				_ = agent.EndMailSessionsV1(&transport.EndMailSessionsRequest{ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			name: "SetAppUnitSuspensionV1",
			run: func(expected string) string {
				var response transport.SetAppUnitSuspensionResponse
				_ = agent.SetAppUnitSuspensionV1(&transport.SetAppUnitSuspensionRequest{ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			// alpha.117 (D-043): a security emergency's unit lock.
			name: "SetEmergencyUnitLockV1",
			run: func(expected string) string {
				var response transport.SetEmergencyUnitLockResponse
				_ = agent.SetEmergencyUnitLockV1(&transport.SetEmergencyUnitLockRequest{ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			// alpha.117 (D-043): a Node.js application's start lock.
			name: "SetEmergencyAppLockV1",
			run: func(expected string) string {
				var response transport.SetEmergencyAppLockResponse
				_ = agent.SetEmergencyAppLockV1(&transport.SetEmergencyAppLockRequest{ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			// alpha.113: the default HTTPS server's "Try again".
			name: "ReconcileDefaultHTTPSServerV1",
			run: func(expected string) string {
				var response transport.ReconcileDefaultHTTPSServerResponse
				_ = agent.ReconcileDefaultHTTPSServerV1(&transport.ReconcileDefaultHTTPSServerRequest{ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			name: "InspectSiteFile",
			run: func(expected string) string {
				var response transport.InspectSiteFileResponse
				_ = agent.InspectSiteFile(&ApplyVhostRequest{ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			name: "InspectSitePool",
			run: func(expected string) string {
				var response transport.InspectSiteFileResponse
				_ = agent.InspectSitePool(&transport.SitePoolRequest{ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			name: "ApplySitePool",
			run: func(expected string) string {
				var response transport.ApplySiteFileResponse
				_ = agent.ApplySitePool(&transport.SitePoolRequest{ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			name: "InspectAppUnit",
			run: func(expected string) string {
				var response transport.InspectSiteFileResponse
				_ = agent.InspectAppUnit(&AppApplyRequest{ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			name: "ApplyAppUnit",
			run: func(expected string) string {
				var response AppApplyResponse
				_ = agent.ApplyAppUnit(&AppApplyRequest{ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			// alpha.114: a mailbox's out-of-office reply (Sieve vacation).
			name: "SetMailAutoresponder",
			run: func(expected string) string {
				var response transport.MailAutoresponderResponse
				if err := agent.SetMailAutoresponder(&transport.SetMailAutoresponderRequest{ExpectedBuildCommit: expected}, &response); err != nil {
					return err.Error()
				}
				return response.Detail
			},
		},
		{
			// alpha.117: FTP, SFTP and SSH access of a hosting account.
			name: "ApplyFileAccessV1",
			run: func(expected string) string {
				var response transport.ApplyFileAccessResponse
				if err := agent.ApplyFileAccessV1(&transport.ApplyFileAccessRequest{ExpectedBuildCommit: expected}, &response); err != nil {
					return err.Error()
				}
				return response.Detail
			},
		},
		{
			name: "ApplyVhosts",
			run: func(expected string) string {
				var response ApplyVhostsResponse
				_ = agent.ApplyVhosts(&ApplyVhostsRequest{ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			name: "CreateSite",
			run: func(expected string) string {
				var response transport.CreateSiteResponse
				_ = agent.CreateSite(transport.CreateSiteRequest{ExpectedBuildCommit: expected}, &response)
				return response.ErrorMessage
			},
		},
		{
			name: "DeleteCertLineage",
			run: func(expected string) string {
				var response DeleteCertLineageResponse
				_ = agent.DeleteCertLineage(&DeleteCertLineageRequest{ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			name: "DeleteMailDomain",
			run: func(expected string) string {
				var response transport.DeleteMailDomainResponse
				err := agent.DeleteMailDomain(&transport.DeleteMailDomainRequest{
					ExpectedBuildCommit: expected,
					DomainID:            41,
					Domain:              "example.com",
				}, &response)
				if err == nil {
					return ""
				}
				return err.Error()
			},
		},
		{
			// 30 Oct 2026: a deleted domain's DKIM key.
			name: "RemoveDKIMKey",
			run: func(expected string) string {
				var response transport.DKIMRemoveResponse
				err := agent.RemoveDKIMKey(&transport.DKIMRemoveRequest{
					ExpectedBuildCommit: expected,
					Domain:              "example.com",
					Selector:            "celik",
				}, &response)
				if err == nil {
					return response.Error
				}
				return err.Error()
			},
		},
		{
			name: "UpdateMailPassword",
			run: func(expected string) string {
				var response transport.MailMutationResponse
				err := agent.UpdateMailPassword(&transport.UpdateMailPasswordRequest{
					ExpectedBuildCommit: expected,
					Email:               "user@example.com",
					NewPassword:         "not-a-real-password",
				}, &response)
				if err == nil {
					return ""
				}
				return err.Error()
			},
		},
		{
			name: "DeleteSite",
			run: func(expected string) string {
				var response DeleteSiteResponse
				_ = agent.DeleteSite(&DeleteSiteRequest{ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			name: "ExtractCpmoveFiles",
			run: func(expected string) string {
				var response CpmoveExtractResponse
				err := agent.ExtractCpmoveFiles(
					&CpmoveExtractRequest{ExpectedBuildCommit: expected},
					&response,
				)
				if err != nil {
					return err.Error()
				}
				return response.Error
			},
		},
		{
			name: "ImportCpmoveDatabase",
			run: func(expected string) string {
				var response CpmoveImportDBResponse
				err := agent.ImportCpmoveDatabase(
					&CpmoveImportDBRequest{ExpectedBuildCommit: expected},
					&response,
				)
				if err != nil {
					return err.Error()
				}
				return response.Error
			},
		},
		{
			name: "InspectCpmove",
			run: func(expected string) string {
				var response CpmoveInspectResponse
				err := agent.InspectCpmove(
					&CpmoveInspectRequest{ExpectedBuildCommit: expected},
					&response,
				)
				if err != nil {
					return err.Error()
				}
				return response.Error
			},
		},
		{
			name: "InstallCustomCertificate",
			run: func(expected string) string {
				var response InstallCertResponse
				_ = agent.InstallCustomCertificate(InstallCertRequest{ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			// alpha.118: the application catalog's three privileged steps.
			name: "InstallCatalogAppV1",
			run: func(expected string) string {
				var response transport.InstallCatalogAppResponse
				_ = agent.InstallCatalogAppV1(&transport.InstallCatalogAppRequest{ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			name: "SetupCatalogAppV1",
			run: func(expected string) string {
				var response transport.SetupCatalogAppResponse
				_ = agent.SetupCatalogAppV1(&transport.SetupCatalogAppRequest{ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			name: "RemoveCatalogAppV1",
			run: func(expected string) string {
				var response transport.RemoveCatalogAppResponse
				_ = agent.RemoveCatalogAppV1(&transport.RemoveCatalogAppRequest{ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			name: "InstallWordPress",
			run: func(expected string) string {
				var response InstallWordPressResponse
				_ = agent.InstallWordPress(&InstallWordPressRequest{ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			name: "IssueLetsEncryptCertificate",
			run: func(expected string) string {
				var response IssueLetsEncryptResponse
				_ = agent.IssueLetsEncryptCertificate(IssueLetsEncryptRequest{ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			name: "IssueMailHostCertificateV1",
			run: func(expected string) string {
				var response transport.IssueMailHostCertificateResponse
				_ = agent.IssueMailHostCertificateV1(&transport.IssueMailHostCertificateRequest{Domain: "mail.example.test", Email: "admin@example.test", ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			name: "IssuePanelCertificateV2",
			run: func(expected string) string {
				var response IssuePanelCertV2Response
				_ = agent.IssuePanelCertificateV2(&IssuePanelCertV2Request{
					Domain:              "panel.example.test",
					Email:               "admin@example.test",
					TLSDir:              managedPanelTLSDir,
					ExpectedBuildCommit: expected,
				}, &response)
				return response.Error
			},
		},
		{
			name: "ReconcileSiteCertLineages",
			run: func(expected string) string {
				var response ReconcileSiteCertLineagesResponse
				_ = agent.ReconcileSiteCertLineages(
					&ReconcileSiteCertLineagesRequest{ExpectedBuildCommit: expected},
					&response,
				)
				return response.Error
			},
		},
		{
			name: "RenewLetsEncryptCertificate",
			run: func(expected string) string {
				var response RenewCertResponse
				_ = agent.RenewLetsEncryptCertificate(RenewCertRequest{ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			name: "SetSiteCertificateAutoRenewV1",
			run: func(expected string) string {
				var response transport.SetSiteCertificateAutoRenewResponse
				_ = agent.SetSiteCertificateAutoRenewV1(&transport.SetSiteCertificateAutoRenewRequest{ExpectedBuildCommit: expected}, &response)
				return response.Error
			},
		},
		{
			name: "RestartPanelSoon",
			run: func(expected string) string {
				var response bool
				err := agent.RestartPanelSoon(
					&RestartPanelSoonRequest{ExpectedBuildCommit: expected},
					&response,
				)
				if err == nil {
					return ""
				}
				return err.Error()
			},
		},
		{
			name: "SyncMailTLSV2",
			run: func(expected string) string {
				var response SecureMailTLSResponse
				_ = agent.SyncMailTLSV2(&SyncMailTLSV2Request{
					ExpectedBuildCommit: expected,
					Myhostname:          "mail.example.test",
				}, &response)
				return response.Error
			},
		},
		// alpha.128 ops: restarting the server and certificate requests.
		{name: "ServerRebootV1", run: func(expected string) string {
			var response transport.ServerRebootResponse
			_ = agent.ServerRebootV1(&transport.ServerRebootRequest{ExpectedBuildCommit: expected}, &response)
			return response.Error
		}},
		{name: "CertificateRequestCreateV1", run: func(expected string) string {
			var response transport.CertificateRequestCreateResponse
			_ = agent.CertificateRequestCreateV1(&transport.CertificateRequestCreateRequest{ExpectedBuildCommit: expected}, &response)
			return response.Error
		}},
		{name: "CertificateRequestDiscardV1", run: func(expected string) string {
			var response transport.CertificateRequestDiscardResponse
			_ = agent.CertificateRequestDiscardV1(&transport.CertificateRequestDiscardRequest{ExpectedBuildCommit: expected}, &response)
			return response.Error
		}},
	}
}

func agentRPCMethodsWithBuildCommitField(t *testing.T) []string {
	t.Helper()

	agentType := reflect.TypeOf(&Agent{})
	methods := make([]string, 0)
	for index := 0; index < agentType.NumMethod(); index++ {
		method := agentType.Method(index)
		// V1 is a stable zero-touch mixed-version stub. It deliberately keeps
		// the old wire request shape but never reaches the build gate or host.
		// Mail enrollment source and preview are read-only inspections with
		// separate build-identity tests, not protected host mutations.
		if method.Name == "IssuePanelCertificate" ||
			method.Name == "ReconcileMailTLSMutation" ||
			method.Name == "SecureMailTLS" ||
			method.Name == "MailEnrollmentSourceV1" ||
			method.Name == "MailEnrollmentPreviewV1" {
			continue
		}
		if method.Type.NumIn() != 3 {
			continue
		}
		requestType := method.Type.In(1)
		if requestType.Kind() == reflect.Pointer {
			requestType = requestType.Elem()
		}
		if requestType.Kind() != reflect.Struct {
			continue
		}
		field, ok := requestType.FieldByName("ExpectedBuildCommit")
		if !ok {
			continue
		}
		if field.Type.Kind() != reflect.String {
			t.Fatalf("%s ExpectedBuildCommit type = %s, want string", method.Name, field.Type)
		}
		methods = append(methods, method.Name)
	}
	sort.Strings(methods)
	return methods
}

func TestProtectedBuildCommitOperationMatrixIsComplete(t *testing.T) {
	operations := protectedBuildGateOperations()
	want := make([]string, 0, len(operations))
	seen := make(map[string]struct{}, len(operations))
	for _, operation := range operations {
		if _, exists := seen[operation.name]; exists {
			t.Fatalf("duplicate protected operation %q", operation.name)
		}
		seen[operation.name] = struct{}{}
		want = append(want, operation.name)
	}
	sort.Strings(want)

	got := agentRPCMethodsWithBuildCommitField(t)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf(
			"build-commit RPC coverage changed\ncovered: %v\ndeclared: %v\n"+
				"add every declared operation to the behavioral gate matrix",
			want,
			got,
		)
	}
}

func TestProtectedBuildCommitOperationsFailClosedInProduction(t *testing.T) {
	previousCommit := buildCommit
	buildCommit = "agent-release-commit"
	t.Cleanup(func() { buildCommit = previousCommit })

	for _, operation := range protectedBuildGateOperations() {
		for _, scenario := range []struct {
			name     string
			expected string
			want     string
		}{
			{name: "missing panel identity", want: "expected panel build commit"},
			{name: "mismatched panel identity", expected: "different-panel-release", want: "panel/agent build mismatch"},
		} {
			t.Run(operation.name+"/"+scenario.name, func(t *testing.T) {
				got := operation.run(scenario.expected)
				if !strings.Contains(got, scenario.want) {
					t.Fatalf("gate error = %q, want substring %q", got, scenario.want)
				}
			})
		}
	}
}
