package serviceapps

import (
	"strings"
	"testing"
)

// systemd reads EnvironmentFile= as root and follows links: the unit must
// name root's file, never one inside the site's folder (alpha.118 fix).
func TestUnitEnvironmentFileIsOutsideTheSite(t *testing.T) {
	text, err := RenderUnit(n8nParams(t))
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, line := range strings.Split(text, "\n") {
		if value, ok := strings.CutPrefix(line, "EnvironmentFile="); ok {
			found++
			if value != EnvDir+"/celiksvc-12.env" || EnvDir != "/etc/celikpanel-svcapps" {
				t.Fatalf("EnvironmentFile=%s", value)
			}
		}
	}
	if found != 1 {
		t.Fatalf("EnvironmentFile= lines: %d", found)
	}
	p := n8nParams(t)
	env, err := RenderEnv(EnvParams{App: p.App, Domain: "flows.example.com", Layout: p.Layout, Port: 21010, AuxPort: 21011,
		Generated: map[string]string{"N8N_ENCRYPTION_KEY": strings.Repeat("ab", 32)}})
	if err != nil || UnitTextState(env) != "ours" {
		t.Fatalf("environment file header: %v", err)
	}
	if UnitTextState(env+"OWNER=\"x\"\n") != "edited" {
		t.Fatal("an owner's edit of the environment file must be detected")
	}
}

func TestMigrateUnitEnvLineRewritesOnlyThatLine(t *testing.T) {
	text, err := RenderUnit(n8nParams(t))
	if err != nil {
		t.Fatal(err)
	}
	legacyPath := "/var/www/celikpanel/subscriptions/3/sites/7/apps/n8n/config/app.env"
	_, body, _ := strings.Cut(text, "\n")
	legacy := Seal(strings.Replace(body, "EnvironmentFile="+EnvFile(12), "EnvironmentFile="+legacyPath, 1))
	migrated, named, ok := MigrateUnitEnvLine(legacy, 12)
	if !ok || named != legacyPath || migrated != text {
		t.Fatalf("migration: ok=%v named=%q equal=%v", ok, named, migrated == text)
	}
	if _, _, ok := MigrateUnitEnvLine(text, 12); ok {
		t.Fatal("a current unit needs no migration")
	}
	if _, _, ok := MigrateUnitEnvLine(legacy+"# owner\n", 12); ok {
		t.Fatal("an owner-edited unit is never rewritten")
	}
	if sub, dom, app, ok := LegacyEnvTarget(legacyPath); !ok || sub != 3 || dom != 7 || app != "n8n" {
		t.Fatalf("target: %d %d %s %v", sub, dom, app, ok)
	}
	for _, bad := range []string{"/etc/shadow", "/var/www/celikpanel/subscriptions/3/sites/7/apps/../../../../etc/x/config/app.env",
		"/var/www/celikpanel/subscriptions/3/sites/7/apps/nope/config/app.env", "/var/www/celikpanel/subscriptions/3/sites/7/apps/n8n/config/other"} {
		if _, _, _, ok := LegacyEnvTarget(bad); ok {
			t.Errorf("accepted %s", bad)
		}
	}
	if UnitUser(text) != "flows_example_com" {
		t.Fatalf("user: %q", UnitUser(text))
	}
}

func TestRenderLegacyEnvKeepsOnlyValidLines(t *testing.T) {
	text, err := RenderLegacyEnv(n8nParams(t).App, map[string]string{"N8N_PORT": "21010", "bad key": "x", "OK_EMPTY": ""})
	if err != nil || UnitTextState(text) != "ours" {
		t.Fatal(err)
	}
	env := ParseEnv(text)
	if env["N8N_PORT"] != "21010" || len(env) != 2 {
		t.Fatalf("env: %v", env)
	}
}
