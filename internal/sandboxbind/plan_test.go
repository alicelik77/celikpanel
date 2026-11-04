package sandboxbind

import (
	"strings"
	"testing"
)

func TestPlanForShapes(t *testing.T) {
	plan, err := PlanFor("/var/www/celikpanel/subscriptions/3/sites/7/app", 1005)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Start != "/var/www/celikpanel/subscriptions" || plan.Home != "/var/www/celikpanel/subscriptions/3/sites/7" {
		t.Fatalf("plan = %+v", plan)
	}
	want := []Step{
		{Name: "3", UID: 0, Private: true},
		{Name: "sites", UID: 0, Private: true},
		{Name: "7", UID: 1005},
		{Name: "app", UID: 1005, SameMount: true},
	}
	if len(plan.Steps) != len(want) {
		t.Fatalf("steps = %+v", plan.Steps)
	}
	for i := range want {
		if plan.Steps[i] != want[i] {
			t.Fatalf("step %d = %+v, want %+v", i, plan.Steps[i], want[i])
		}
	}
	svc, err := PlanFor("/var/www/celikpanel/subscriptions/3/sites/7/apps/open-webui", 1005)
	if err != nil || len(svc.Steps) != 5 || svc.Steps[3].Name != "apps" || !svc.Steps[4].SameMount || svc.Steps[4].UID != 1005 {
		t.Fatalf("service app plan = %+v, %v", svc, err)
	}
	for _, bad := range []string{
		"/var/www/celikpanel/subscriptions/3/sites/7",
		"/var/www/celikpanel/subscriptions/3/sites/7/app/bin",
		"/var/www/celikpanel/subscriptions/3/sites/7/apps",
		"/var/www/celikpanel/subscriptions/3/sites/7/apps/../app",
		"/var/www/celikpanel/subscriptions/3/sites/7/apps/N8N",
		"/var/www/celikpanel/subscriptions/03/sites/7/app",
		"/var/www/celikpanel/subscriptions/3/web/7/app",
		"/var/www/celikpanel/subscriptions/3/sites/7/app/",
		"/etc/app",
		"var/www/celikpanel/subscriptions/3/sites/7/app",
	} {
		if _, err := PlanFor(bad, 1005); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
	if _, err := PlanFor("/var/www/celikpanel/subscriptions/3/sites/7/app", 33); err == nil {
		t.Error("a system account accepted")
	}
}

func TestRefusalWording(t *testing.T) {
	r := refused(CodeNotReal, "/x/app", "is a link, not a real folder")
	if r.Error() != "/x/app is a link, not a real folder" {
		t.Fatal(r.Error())
	}
	got, ok := AsRefusal(r)
	if !ok || got.Code != CodeNotReal || !strings.Contains(got.Error(), "link") {
		t.Fatal("refusal not found")
	}
}
