package main

import (
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.134 fix round: the Agent refuses a database or user name the engine
// cannot hold with the panel's own rule (services.DatabaseNamesOver), before
// any statement runs, as NameTooLong - never a bare "invalid username" (the
// lab's PostgreSQL case: the user came to 64 characters).
func TestCreateDatabaseRefusesNamesTooLongBeforeAnythingRuns(t *testing.T) {
	full := "k_1_dr_test_a_b_c_d_e_f_g_h_i_j_k_l_m_n_o_p_q_r_s_t_u_v_w_x"
	for _, engine := range []string{"postgresql", "mysql"} {
		var resp transport.CreateDatabaseResponse
		if err := (&Agent{}).CreateDatabase(transport.CreateDatabaseRequest{Type: engine, Name: full, User: full + "_user", Password: "Sup3r-secret-pass!"}, &resp); err != nil {
			t.Fatal(err)
		}
		if resp.Success || !resp.NameTooLong || resp.CleanupIncomplete {
			t.Fatalf("%s: %+v", engine, resp)
		}
	}
}
