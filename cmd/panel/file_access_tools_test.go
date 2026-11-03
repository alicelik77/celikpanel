package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The Access view carries the missing tools from the Agent's read.
func TestFileAccessGetReportsMissingJailTools(t *testing.T) {
	agent := &fileAccessPanelAgent{capabilities: []string{transport.AgentCapabilityFileAccessV1}, jailMissing: []string{"zip", "unzip"}}
	panel := newFileAccessPanel(t, agent)
	response := fileAccessRequest(panel, http.MethodGet, 7103, false, "")
	var view fileAccessView
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.State == nil || !view.State.Server.JailChecked || !reflect.DeepEqual(view.State.Server.JailMissing, []string{"zip", "unzip"}) {
		t.Fatalf("view %s", response.Body.String())
	}
}

// Turning SSH on while tools are missing is refused by the server itself,
// before the Agent is asked to change anything.
func TestFileAccessSSHRefusedWhileToolsMissing(t *testing.T) {
	agent := &fileAccessPanelAgent{
		capabilities: []string{transport.AgentCapabilityFileAccessV1},
		jailMissing:  []string{"zip", "unzip"},
		answer:       transport.ApplyFileAccessResponse{Applied: true},
	}
	panel := newFileAccessPanel(t, agent)
	response := fileAccessRequest(panel, http.MethodPut, 7103, false, `{"enabled":true,"ssh":true}`)
	body := decodeCode(t, response)
	if response.Code != http.StatusConflict || body.Code != errCodeFileAccessSSHToolsMissing ||
		!reflect.DeepEqual(body.Details, []string{"zip", "unzip"}) || !strings.Contains(body.Error, "FTP and SFTP are not affected") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(agent.applied()) != 0 {
		t.Fatalf("the refused change reached the Agent: %+v", agent.applied())
	}
	// FTP and SFTP alone still open.
	if files := fileAccessRequest(panel, http.MethodPut, 7103, false, `{"enabled":true}`); files.Code != http.StatusOK {
		t.Fatalf("files status=%d body=%s", files.Code, files.Body.String())
	}
}

// An account that already has SSH keeps it and can still be saved.
func TestFileAccessExistingSSHAccountIsNotCutOff(t *testing.T) {
	agent := &fileAccessPanelAgent{
		capabilities: []string{transport.AgentCapabilityFileAccessV1},
		jailMissing:  []string{"zip"}, enabled: true, sshOn: true,
		answer: transport.ApplyFileAccessResponse{Applied: true},
	}
	panel := newFileAccessPanel(t, agent)
	response := fileAccessRequest(panel, http.MethodPut, 7103, false, `{"enabled":true,"ssh":true,"password":"correct horse battery"}`)
	if response.Code != http.StatusOK || len(agent.applied()) != 1 || !agent.applied()[0].SSH {
		t.Fatalf("status=%d body=%s applies=%+v", response.Code, response.Body.String(), agent.applied())
	}
}

func fileAccessToolsRequest(panel *Panel, userID int, role, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, fileAccessToolsPath, strings.NewReader(body))
	request = request.WithContext(context.WithValue(request.Context(), callerKey, &Caller{ID: userID, Role: role, AccountType: core.AccountTypeAccount}))
	response := httptest.NewRecorder()
	panel.handleFileAccessTools(response, request)
	return response
}

func stubJailToolsInstall(t *testing.T, answer transport.InstallJailToolsResponse, err error) *[][]string {
	t.Helper()
	var calls [][]string
	saved := installJailToolsThroughAgent
	t.Cleanup(func() { installJailToolsThroughAgent = saved })
	installJailToolsThroughAgent = func(_ *Panel, _ context.Context, packages []string) (transport.InstallJailToolsResponse, error) {
		calls = append(calls, append([]string(nil), packages...))
		return answer, err
	}
	return &calls
}

// The install is the administrator's, sends exactly the listed packages
// (sorted) and is audited as a package install.
func TestFileAccessToolsInstallSendsOnlyTheListedPackages(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	calls := stubJailToolsInstall(t, transport.InstallJailToolsResponse{Installed: []string{"unzip", "zip"}}, nil)

	for _, role := range []string{roleReseller, roleCustomer} {
		if response := fileAccessToolsRequest(panel, userID, role, `{"packages":["zip","unzip"]}`); response.Code != http.StatusForbidden {
			t.Fatalf("%s: status=%d", role, response.Code)
		}
	}
	for _, body := range []string{`{"packages":[]}`, `{"packages":["zip","zip"]}`, `{"packages":["-o APT::x"]}`, `{}`} {
		if response := fileAccessToolsRequest(panel, userID, roleAdmin, body); response.Code != http.StatusBadRequest {
			t.Fatalf("%s: status=%d", body, response.Code)
		}
	}
	if len(*calls) != 0 {
		t.Fatalf("a refused request reached the Agent: %v", *calls)
	}
	response := fileAccessToolsRequest(panel, userID, roleAdmin, `{"packages":["zip","unzip"]}`)
	if response.Code != http.StatusOK || !reflect.DeepEqual(*calls, [][]string{{"unzip", "zip"}}) {
		t.Fatalf("status=%d body=%s calls=%v", response.Code, response.Body.String(), *calls)
	}
	var action string
	if err := database.GetDB().QueryRow(`SELECT action FROM audit_logs ORDER BY id DESC LIMIT 1`).Scan(&action); err != nil || action != "service.install:unzip,zip" {
		t.Fatalf("audit %q %v", action, err)
	}
}

func TestFileAccessToolsInstallRefusalsAreCoded(t *testing.T) {
	panel, _, _, userID := newServiceStateAtomicPanel(t)
	cases := []struct {
		answer transport.InstallJailToolsResponse
		err    error
		code   string
	}{
		{transport.InstallJailToolsResponse{ErrorCode: transport.JailToolsChanged, Error: "changed"}, nil, errCodeFileAccessToolsChanged},
		{transport.InstallJailToolsResponse{ErrorCode: transport.JailToolsUnsupported, Error: "none"}, nil, errCodeFileAccessToolsUnknown},
		{transport.InstallJailToolsResponse{ErrorCode: transport.JailToolsRefused, Error: "apt said no", StillMissing: []string{"zip"}}, nil, errCodeFileAccessToolsFailed},
		{transport.InstallJailToolsResponse{}, errors.New("rpc: can't find method Agent.InstallJailToolsV1"), errCodeFileAccessAgentUpgrade},
	}
	for _, c := range cases {
		stubJailToolsInstall(t, c.answer, c.err)
		response := fileAccessToolsRequest(panel, userID, roleAdmin, `{"packages":["zip"]}`)
		if response.Code != http.StatusConflict || decodeCode(t, response).Code != c.code {
			t.Fatalf("%s: status=%d body=%s", c.code, response.Code, response.Body.String())
		}
	}
}
