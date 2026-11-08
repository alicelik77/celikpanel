package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

type serverSettingsFixture struct {
	p        *Panel
	sub      []transport.MailSubaddressingRequest
	lists    []transport.MailSenderListRequest
	answer   transport.MailSettingApplyResponse
	callFail bool
}

func newServerSettingsFixture(t *testing.T) *serverSettingsFixture {
	t.Helper()
	database, err := openTemplatePanelDatabase(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(database.Close)
	f := &serverSettingsFixture{p: &Panel{db: database}, answer: transport.MailSettingApplyResponse{Applied: true, Changed: true}}
	savedSub, savedList, savedStatus := mailSubaddressingAgentApply, mailSenderAgentApply, mailSettingsAgentStatus
	t.Cleanup(func() {
		mailSubaddressingAgentApply, mailSenderAgentApply, mailSettingsAgentStatus = savedSub, savedList, savedStatus
	})
	mailSubaddressingAgentApply = func(_ *Panel, _ context.Context, req *transport.MailSubaddressingRequest) (transport.MailSettingApplyResponse, error) {
		f.sub = append(f.sub, *req)
		return f.answer, nil
	}
	mailSenderAgentApply = func(_ *Panel, _ context.Context, req *transport.MailSenderListRequest) (transport.MailSettingApplyResponse, error) {
		f.lists = append(f.lists, *req)
		return f.answer, nil
	}
	mailSettingsAgentStatus = func(*Panel, context.Context) (transport.MailServerSettingsStatus, error) {
		return transport.MailServerSettingsStatus{Postfix: true, Subaddressing: transport.MailSettingOff, SenderList: transport.MailSettingOff}, nil
	}
	for _, stmt := range []string{
		`INSERT INTO users (id, username, password_hash, email, role) VALUES (9602, 'cust1', 'x', 'cust1@example.net', 'customer')`,
		`INSERT INTO subscriptions (id, owner_id, name) VALUES (9701, 9602, 'cust1-sub')`,
		`INSERT INTO domains (id, subscription_id, name) VALUES (9801, 9701, 'hosted.example')`,
	} {
		if _, err := database.GetDB().Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	return f
}

func (f *serverSettingsFixture) post(t *testing.T, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	recorder := httptest.NewRecorder()
	f.p.handleMailServerSettings(recorder, httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw)))
	return recorder
}

func (f *serverSettingsFixture) view(t *testing.T) mailServerSettingsView {
	t.Helper()
	recorder := httptest.NewRecorder()
	f.p.handleMailServerSettings(recorder, httptest.NewRequest(http.MethodGet, mailServerSettingsPath, nil))
	var view mailServerSettingsView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatalf("%v: %s", err, recorder.Body)
	}
	return view
}

func TestSubaddressingSaveAppliesAndRecordsTheServersAnswer(t *testing.T) {
	f := newServerSettingsFixture(t)
	if rec := f.post(t, mailSubaddressingPath, map[string]any{"enabled": true}); rec.Code != http.StatusOK {
		t.Fatalf("save = %d %s", rec.Code, rec.Body)
	}
	if len(f.sub) != 1 || !f.sub[0].Enabled {
		t.Fatalf("agent = %+v", f.sub)
	}
	if view := f.view(t); !view.Subaddressing.Enabled || view.Subaddressing.Apply.State != "applied" || view.Server == nil {
		t.Fatalf("view = %+v", view)
	}
	f.answer = transport.MailSettingApplyResponse{Code: transport.MailSettingOwnerConfig, Error: "main.cf already has the server owner's recipient_delimiter = -"}
	f.post(t, mailSubaddressingPath, map[string]any{"enabled": true})
	if view := f.view(t); view.Subaddressing.Apply.State != "refused" || view.Subaddressing.Apply.Code != transport.MailSettingOwnerConfig {
		t.Fatalf("refused view = %+v", view.Subaddressing)
	}
}

func TestSenderListAddsValidatesAndAppliesTheWholeList(t *testing.T) {
	f := newServerSettingsFixture(t)
	for _, body := range []map[string]any{
		{"op": "add", "value": "Spam.Example", "action": "block"},
		{"op": "add", "value": "Boss@Spam.Example", "action": "allow"},
	} {
		if rec := f.post(t, mailSenderRulesPath, body); rec.Code != http.StatusOK {
			t.Fatalf("%v = %d %s", body, rec.Code, rec.Body)
		}
	}
	last := f.lists[len(f.lists)-1]
	if len(last.Rules) != 2 {
		t.Fatalf("applied = %+v", last)
	}
	for _, bad := range []struct {
		body map[string]any
		code string
	}{
		{map[string]any{"op": "add", "value": "spam.example", "action": "allow"}, errCodeMailSenderExists},
		{map[string]any{"op": "add", "value": "x@hosted.example", "action": "block"}, errCodeMailSenderOwnDomain},
		{map[string]any{"op": "add", "value": "example", "action": "block"}, errCodeMailSenderInvalid},
		{map[string]any{"op": "add", "value": "a.example", "action": "permit"}, errCodeMailSenderInvalid},
		{map[string]any{"op": "add", "value": "a.example OK", "action": "block"}, errCodeMailSenderInvalid},
	} {
		rec := f.post(t, mailSenderRulesPath, bad.body)
		if rec.Code < 400 || !strings.Contains(rec.Body.String(), bad.code) {
			t.Fatalf("%v = %d %s", bad.body, rec.Code, rec.Body)
		}
	}
	// A parent of a hosted domain would block its senders too.
	if rec := f.post(t, mailSenderRulesPath, map[string]any{"op": "add", "value": "example", "action": "block"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("bare label = %d", rec.Code)
	}
	if len(f.lists) != 2 {
		t.Fatalf("refused requests reached the server: %d", len(f.lists))
	}
	view := f.view(t)
	if len(view.Sender.Rules) != 2 || view.Sender.Apply.State != "applied" {
		t.Fatalf("view = %+v", view.Sender)
	}
	// Removing the last entries applies an empty list (the server removes its map).
	for _, rule := range view.Sender.Rules {
		if rec := f.post(t, mailSenderRulesPath, map[string]any{"op": "remove", "id": rule.ID}); rec.Code != http.StatusOK {
			t.Fatalf("remove = %d %s", rec.Code, rec.Body)
		}
	}
	if last := f.lists[len(f.lists)-1]; len(last.Rules) != 0 {
		t.Fatalf("last list = %+v", last)
	}
	if rec := f.post(t, mailSenderRulesPath, map[string]any{"op": "remove", "id": 999}); rec.Code != http.StatusNotFound {
		t.Fatalf("missing = %d", rec.Code)
	}
}
