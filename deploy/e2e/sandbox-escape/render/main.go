// Command render prints the site sandbox's native files for the escape-test
// kit (deploy/e2e/sandbox-escape/README.md): the egress table, its guard and
// a sandboxed unit rendered exactly as CelikPanel renders one, for the
// capabilities this host reports. It changes nothing; run.sh installs what
// it prints, and the helper (bin/sandbox-helper, the Agent built by
// "run.sh build") the units run before every start.
//
//	render -what caps
//	render -what egress|guard-script|guard-service|guard-timer|bind-template
//	render -what unit -unit celikrt-990001.service -kind program -user u -appdir D -port 30990 -exec "D/bin/probe"
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/alicelik/celikpanel/internal/sitesandbox"
)

func main() {
	what := flag.String("what", "", "caps, egress, guard-script, guard-service, guard-timer, bind-template or unit")
	unit := flag.String("unit", "", "unit name (celikrt-<n>.service)")
	kind := flag.String("kind", "program", "service_app, python or program")
	user := flag.String("user", "", "the site account")
	appDir := flag.String("appdir", "", "the application folder (inside a site home)")
	port := flag.Int("port", 0, "the loopback port")
	exec := flag.String("exec", "", "ExecStart (space-separated, no quotes)")
	env := flag.String("env", "", "Environment= values, comma-separated KEY=VALUE")
	memory := flag.Int("memory", 256, "MemoryMax in MB")
	tasks := flag.Int("tasks", 64, "TasksMax")
	cpu := flag.Int("cpu", 50, "CPUQuota percent")
	flag.Parse()

	caps := sitesandbox.Probe(context.Background(), sitesandbox.HostRunner{})
	switch *what {
	case "caps":
		out, _ := json.MarshalIndent(map[string]any{"caps": caps, "assessment": sitesandbox.Assess(caps)}, "", "  ")
		fmt.Println(string(out))
	case "egress":
		fmt.Print(sitesandbox.EgressRuleset())
	case "guard-script":
		fmt.Print(sitesandbox.EgressGuardScriptText())
	case "guard-service":
		fmt.Print(sitesandbox.EgressGuardServiceText())
	case "guard-timer":
		fmt.Print(sitesandbox.EgressGuardTimerText())
	case "bind-template":
		fmt.Print(sitesandbox.BindTemplateText())
	case "unit":
		text, err := renderUnit(caps, *unit, sitesandbox.Profile{
			Kind: sitesandbox.Kind(*kind), Unit: *unit, User: *user, Group: *user, AppDir: *appDir, Port: *port,
			Limits: sitesandbox.Limits{MemoryMB: *memory, CPUPercent: *cpu, Tasks: *tasks},
			Level:  sitesandbox.Assess(caps).Level,
		}, *exec, *env)
		if err != nil {
			fmt.Fprintln(os.Stderr, "render:", err)
			os.Exit(1)
		}
		fmt.Print(text)
	default:
		flag.Usage()
		os.Exit(2)
	}
}

func renderUnit(caps sitesandbox.Caps, unit string, p sitesandbox.Profile, exec, env string) (string, error) {
	if !sitesandbox.ValidUnitName(unit) {
		return "", fmt.Errorf("%q is not a sandboxed unit name", unit)
	}
	directives, err := sitesandbox.Directives(p, caps)
	if err != nil {
		return "", err
	}
	args := strings.Fields(exec)
	if len(args) == 0 {
		return "", fmt.Errorf("no ExecStart")
	}
	for _, a := range args {
		if err := sitesandbox.CheckValue(a); err != nil {
			return "", err
		}
	}
	// The bind unit binds the app folder before every start, and the helper
	// loads the SMTP block where the host holds it, as in CelikPanel's units.
	unitLines, pre, post, err := sitesandbox.HelperLines(p, caps)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s%d\n", sitesandbox.GenerationMarker, sitesandbox.Generation)
	b.WriteString("# Site sandbox escape-test unit (deploy/e2e/sandbox-escape). Remove after the run.\n[Unit]\nDescription=CelikPanel sandbox escape test " + unit + "\n")
	for _, line := range append(sitesandbox.UnitDirectives(), unitLines...) {
		b.WriteString(line + "\n")
	}
	b.WriteString("\n[Service]\nType=simple\n")
	fmt.Fprintf(&b, "User=%s\nGroup=%s\nWorkingDirectory=%s\n", p.User, p.Group, p.AppDir)
	for _, kv := range strings.Split(env, ",") {
		if kv == "" {
			continue
		}
		if err := sitesandbox.CheckValue(kv); err != nil {
			return "", err
		}
		b.WriteString("Environment=" + kv + "\n")
	}
	b.WriteString("ExecStart=" + strings.Join(args, " ") + "\n")
	directives = append(append(pre, directives...), post...)
	for _, d := range directives {
		b.WriteString(d + "\n")
	}
	return b.String(), nil
}
