package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.134 fix round, on the domain page's create (POST
// /api/v1/domains/{id}/databases) with an Agent that records what it is
// asked: names the engine cannot hold are refused before the Agent is asked
// (defect 1), the list says how a new name begins (defect 2), and a create
// whose clean-up did not finish names what is left (defect 3).

const labTooLongName = "a_b_c_d_e_f_g_h_i_j_k_l_m_n_o_p_q_r_s_t_u_v_w_x"

type domainCreateAgent struct {
	created     []transport.CreateDatabaseRequest
	deleted     []transport.DeleteDatabaseRequest
	create      transport.CreateDatabaseResponse
	deleteFails bool
}

func (a *domainCreateAgent) GetServices(_ *transport.Empty, reply *[]core.Service) error {
	*reply = []core.Service{{Name: "mariadb.service", Version: "11.8", Status: "running"}}
	return nil
}

func (a *domainCreateAgent) CreateDatabase(req transport.CreateDatabaseRequest, resp *transport.CreateDatabaseResponse) error {
	a.created = append(a.created, req)
	*resp = a.create
	return nil
}

func (a *domainCreateAgent) DeleteDatabase(req transport.DeleteDatabaseRequest, resp *transport.DeleteDatabaseResponse) error {
	a.deleted = append(a.deleted, req)
	if a.deleteFails {
		resp.Error = "failed to drop database user"
		return nil
	}
	resp.Success = true
	return nil
}

// newDomainCreateFixture is the administrator's subscription with one domain
// and the recording Agent in place of the real one.
func newDomainCreateFixture(t *testing.T, domain string, agent *domainCreateAgent) (*Panel, int) {
	t.Helper()
	panel, database := newDatabaseAdminAccountFixture(t, &databaseAdminAccountAgent{services: adminAccountServices()})
	server := rpc.NewServer()
	if err := server.RegisterName("Agent", agent); err != nil {
		t.Fatal(err)
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
		t.Fatal(err)
	}
	panel.agentClient = transport.NewReconnectingClientWithContextConnector(client, connector)
	t.Cleanup(func() { _ = client.Close() })
	result, err := database.GetDB().Exec(`INSERT INTO domains (subscription_id, name, status) VALUES (?, ?, 'active')`, adminAccountSubscriptionID, domain)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := result.LastInsertId()
	return panel, int(id)
}

func postDomainDatabase(panel *Panel, domainID int, body string) (*httptest.ResponseRecorder, apiErrorBody) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/domains/1/databases", strings.NewReader(body))
	request = request.WithContext(context.WithValue(request.Context(), callerKey, &Caller{ID: adminAccountUserID, Role: roleAdmin}))
	recorder := httptest.NewRecorder()
	panel.handleCreateDatabase(recorder, request, domainID)
	var refusal apiErrorBody
	_ = json.Unmarshal(recorder.Body.Bytes(), &refusal)
	return recorder, refusal
}

func TestDomainCreateRefusesNamesTheEngineCannotHoldBeforeTheAgent(t *testing.T) {
	agent := &domainCreateAgent{}
	panel, domainID := newDomainCreateFixture(t, "k-1.dr.test", agent)
	full := "k_1_dr_test_" + labTooLongName

	// The lab's case on PostgreSQL: the generated user is 64 characters.
	answer, refusal := postDomainDatabase(panel, domainID, `{"name":"`+labTooLongName+`","type":"postgresql","password":"Sup3r-secret-pass!"}`)
	if answer.Code != http.StatusConflict || refusal.Code != errCodeDatabaseNameTooLong || refusal.Reason != databaseNameTooLongPlain ||
		refusal.Vars["over"] != "1" || refusal.Vars["name"] != full || strings.Contains(refusal.Error, "underscore") {
		t.Fatalf("PostgreSQL: %d %s", answer.Code, answer.Body.String())
	}
	// The same name on MariaDB: its exact form is 22 over; fewer underscores help.
	answer, refusal = postDomainDatabase(panel, domainID, `{"name":"`+labTooLongName+`","type":"mysql","password":"Sup3r-secret-pass!"}`)
	if answer.Code != http.StatusConflict || refusal.Code != errCodeDatabaseNameTooLong || refusal.Reason != "" || refusal.Vars["over"] != "22" ||
		!strings.Contains(refusal.Error, "underscore") {
		t.Fatalf("MariaDB: %d %s", answer.Code, answer.Body.String())
	}
	// MariaDB, few underscores: only the user (64) is too long.
	answer, refusal = postDomainDatabase(panel, domainID, `{"name":"`+strings.Repeat("a", 47)+`","type":"mysql","password":"Sup3r-secret-pass!"}`)
	if answer.Code != http.StatusConflict || refusal.Reason != databaseNameTooLongPlain || refusal.Vars["over"] != "1" {
		t.Fatalf("MariaDB user name: %d %s", answer.Code, answer.Body.String())
	}
	// A character the engine's names do not hold.
	answer, _ = postDomainDatabase(panel, domainID, `{"name":"my-app","type":"mysql","password":"Sup3r-secret-pass!"}`)
	if answer.Code != http.StatusBadRequest {
		t.Fatalf("hyphen: %d %s", answer.Code, answer.Body.String())
	}
	if len(agent.created) != 0 {
		t.Fatalf("the Agent was asked: %+v", agent.created)
	}
}

func TestDomainDatabaseListSaysHowANewNameBegins(t *testing.T) {
	for domain, prefix := range map[string]string{"k-1.dr.test": "k_1_dr_test_", "1and1.com": "_1and1_com_", "xn--rksel-kva.example": "xn__rksel_kva_example_"} {
		panel, domainID := newDomainCreateFixture(t, domain, &domainCreateAgent{})
		request := httptest.NewRequest(http.MethodGet, "/api/v1/domains/1/databases", nil)
		request = request.WithContext(context.WithValue(request.Context(), callerKey, &Caller{ID: adminAccountUserID, Role: roleAdmin}))
		recorder := httptest.NewRecorder()
		panel.handleGetDomainDatabases(recorder, request, domainID)
		var list struct {
			NamePrefix string `json:"name_prefix"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &list); err != nil || list.NamePrefix != prefix {
			t.Fatalf("%s: %d %s", domain, recorder.Code, recorder.Body.String())
		}
		if got := sanitizeName(domain) + "_myapp"; got != prefix+"myapp" {
			t.Fatalf("%s: the create would make %s", domain, got)
		}
	}
}

func TestDomainCreateTakesBackWhatTheAgentMadeWhenItsRecordsFail(t *testing.T) {
	for _, deleteFails := range []bool{false, true} {
		agent := &domainCreateAgent{create: transport.CreateDatabaseResponse{Success: true}, deleteFails: deleteFails}
		panel, domainID := newDomainCreateFixture(t, "shop.example", agent)
		if _, err := panel.db.GetDB().Exec(`CREATE TRIGGER a134_fail_records BEFORE INSERT ON databases_v2 BEGIN SELECT RAISE(ABORT, 'records refused'); END;`); err != nil {
			t.Fatal(err)
		}
		answer, refusal := postDomainDatabase(panel, domainID, `{"name":"app","type":"mysql","password":"Sup3r-secret-pass!"}`)
		if len(agent.deleted) != 1 || agent.deleted[0].Name != "shop_example_app" || agent.deleted[0].User != "shop_example_app_user" || !agent.deleted[0].RequireUserCleanup {
			t.Fatalf("take-back asked %+v", agent.deleted)
		}
		if !deleteFails {
			if answer.Code != http.StatusInternalServerError || refusal.Code == errCodeDatabaseCreateLeftOver {
				t.Fatalf("taken back: %d %s", answer.Code, answer.Body.String())
			}
			continue
		}
		if answer.Code != http.StatusConflict || refusal.Code != errCodeDatabaseCreateLeftOver || refusal.Reason != "administrator" ||
			refusal.Vars["left"] != "shop_example_app, shop_example_app_user" {
			t.Fatalf("not taken back: %d %s", answer.Code, answer.Body.String())
		}
	}
}

func TestDomainCreateNamesWhatTheAgentCouldNotTakeBack(t *testing.T) {
	agent := &domainCreateAgent{create: transport.CreateDatabaseResponse{CleanupIncomplete: true, Left: []string{"shop_example_app_user"}, Error: "failed to grant database privileges"}}
	panel, domainID := newDomainCreateFixture(t, "shop.example", agent)
	answer, refusal := postDomainDatabase(panel, domainID, `{"name":"app","type":"mysql","password":"Sup3r-secret-pass!"}`)
	if answer.Code != http.StatusConflict || refusal.Code != errCodeDatabaseCreateLeftOver || refusal.Vars["left"] != "shop_example_app_user" ||
		strings.Contains(refusal.Error, "grant database privileges") {
		t.Fatalf("%d %s", answer.Code, answer.Body.String())
	}
	var count int
	if err := panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM databases_v2`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("recorded %d %v", count, err)
	}
}
