package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// fileAnswerTestAgent answers every file-manager RPC with one error, as the
// Agent does for a path that does not exist (siteFileAnswer).
type fileAnswerTestAgent struct {
	fileMutationTestAgent
	answer string
}

func (a *fileAnswerTestAgent) ListFiles(*transport.ListFilesRequest, *transport.ListFilesResponse) error {
	return errors.New(a.answer)
}

func (a *fileAnswerTestAgent) ReadFile(*transport.ReadFileRequest, *transport.ReadFileResponse) error {
	return errors.New(a.answer)
}

func (a *fileAnswerTestAgent) RenameFile(*transport.RenameFileRequest, *bool) error {
	return errors.New(a.answer)
}

func (a *fileAnswerTestAgent) CreateFileOrDir(*transport.CreateFileRequest, *bool) error {
	return errors.New(a.answer)
}

func newFileAnswerTestPanel(t *testing.T, agent *fileAnswerTestAgent) *Panel {
	t.Helper()
	server := rpc.NewServer()
	if err := server.RegisterName("Agent", agent); err != nil {
		t.Fatalf("register file answer agent: %v", err)
	}
	connector := func(ctx context.Context) (*rpc.Client, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		serverConn, clientConn := net.Pipe()
		go server.ServeConn(serverConn)
		return rpc.NewClient(clientConn), nil
	}
	client, err := connector(context.Background())
	if err != nil {
		t.Fatalf("connect file answer agent: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return &Panel{
		pkgFamilyVal: "apt",
		agentClient:  transport.NewReconnectingClientWithContextConnector(client, connector),
	}
}

// Final alpha.113 acceptance run 2: the file manager answered a bare 500
// INTERNAL for a path that does not exist. The known conditions are coded
// client errors with guidance; anything else stays a server error.
func TestFileManagerMissingPathAnswersGuidance(t *testing.T) {
	scope := domainFileScope{SubscriptionID: 7, DomainID: 1, Root: "/site"}
	for _, tc := range []struct {
		name, answer, body string
		list               bool
		status             int
		code, contains     string
	}{
		{name: "list missing folder", answer: transport.SiteFilePathMissing, list: true, status: http.StatusNotFound, code: errCodeFileNotFound, contains: "top level (/) is the site's web folder"},
		{name: "read missing file", answer: transport.SiteFilePathMissing, body: `{"action":"read"}`, status: http.StatusNotFound, code: errCodeFileNotFound, contains: "does not exist"},
		{name: "rename missing source", answer: transport.SiteFilePathMissing, body: `{"action":"rename","new_path":"b.txt"}`, status: http.StatusNotFound, code: errCodeFileNotFound, contains: "does not exist"},
		{name: "web folder gone", answer: transport.SiteFileRootMissing, list: true, status: http.StatusNotFound, code: errCodeFileRootMissing, contains: "server owner restores"},
		{name: "name taken", answer: transport.SiteFilePathExists, body: `{"action":"create"}`, status: http.StatusConflict, code: errCodeFileExists, contains: "already exists"},
		{name: "unknown failure", answer: "input/output error", body: `{"action":"read"}`, status: http.StatusInternalServerError, code: "INTERNAL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			agentPanel := newFileAnswerTestPanel(t, &fileAnswerTestAgent{answer: tc.answer})
			recorder := httptest.NewRecorder()
			if tc.list {
				request := httptest.NewRequest(http.MethodGet, "/api/v1/domains/1/files?path=/gone", nil)
				agentPanel.handleListFiles(recorder, request, "gone", scope)
			} else {
				request := httptest.NewRequest(http.MethodPost, "/api/v1/domains/1/files", strings.NewReader(tc.body))
				agentPanel.handleFileAction(recorder, request, "gone/a.txt", scope)
			}
			if recorder.Code != tc.status {
				t.Fatalf("status = %d, want %d; body=%q", recorder.Code, tc.status, recorder.Body.String())
			}
			var body apiErrorBody
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode %q: %v", recorder.Body.String(), err)
			}
			if body.Code != tc.code || !strings.Contains(body.Error, tc.contains) {
				t.Fatalf("answer = %+v, want code %s mentioning %q", body, tc.code, tc.contains)
			}
			if strings.Contains(body.Error, "no such file") {
				t.Fatalf("raw system error reached the browser: %q", body.Error)
			}
		})
	}
}
