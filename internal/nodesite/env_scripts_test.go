package nodesite

import (
	"strings"
	"testing"
)

// alpha.128 (PLESK-GAP G19): the environment file line and package scripts.

func TestUnitNamesTheEnvironmentFileOnlyWhenItExists(t *testing.T) {
	caps := systems[3].caps
	plain, err := RenderUnit(params(caps))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain, "EnvironmentFile") {
		t.Fatal("a site without variables names an environment file")
	}
	p := params(caps)
	p.EnvFile = EnvPath("/etc/celikpanel-runtime", p.SiteID)
	withEnv, err := RenderUnit(p)
	if err != nil {
		t.Fatal(err)
	}
	line := "EnvironmentFile=-/etc/celikpanel-runtime/celikapp-12.service.env\n"
	if !strings.Contains(withEnv, "Environment=NODE_ENV=production\n"+line) {
		t.Fatalf("environment file line:\n%s", withEnv)
	}
	if strings.Replace(withEnv, line, "", 1) != plain {
		t.Fatal("the environment file changed more than its own line")
	}
}

func TestPackageScriptsAreTheProjectsPlainNames(t *testing.T) {
	got := PackageScripts([]byte(`{"scripts":{"build":"vite build","db:migrate":"x","bad name":"y","-rf":"z","start":"node ."}}`))
	if strings.Join(got, ",") != "build,db:migrate,start" {
		t.Fatalf("scripts: %v", got)
	}
	if PackageScripts([]byte("not json")) != nil && len(PackageScripts([]byte("not json"))) != 0 {
		t.Fatal("junk lists scripts")
	}
	p := InstallParams{SiteID: 12, Domain: "shop.example.com", User: "u", Group: "u", AppDir: docRoot, NodeVersion: "22.11.0", Build: true, Script: "db:migrate"}
	steps, err := InstallSteps(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || steps[0][len(steps[0])-2] != "run" || steps[0][len(steps[0])-1] != "db:migrate" {
		t.Fatalf("script steps: %v", steps)
	}
	p.Script = "a;rm -rf /"
	if _, err := InstallSteps(p); err == nil {
		t.Fatal("a script name with shell text was accepted")
	}
}
