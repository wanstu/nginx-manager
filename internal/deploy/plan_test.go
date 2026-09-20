package deploy

import (
	"strings"
	"testing"
)

func TestBuildPlan(t *testing.T) {
	plan, err := BuildPlan(PlanInput{
		Executable:                   "/usr/local/bin/nginx-manager",
		ServiceUser:                  "nginx-manager",
		CurrentUser:                  "nginx-manager",
		ManagementPasswordConfigured: true,
		PrivilegeReady:               true,
		PathsConfigured:              true,
		NginxConfigOK:                true,
		SystemdAvailable:             true,
		ManagerInstalled:             true,
		ManagerEnabled:               true,
		ManagerActive:                true,
		CertbotAvailable:             true,
		RenewalInstalled:             true,
		RenewalEnabled:               true,
		RenewalActive:                true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Ready || plan.RequiredReady != plan.RequiredTotal {
		t.Fatalf("plan not ready: %+v", plan)
	}
	if plan.Completed != plan.Total {
		t.Fatalf("completed=%d total=%d", plan.Completed, plan.Total)
	}
	if !strings.Contains(plan.VerifyCommand, "doctor") {
		t.Fatalf("verify command = %q", plan.VerifyCommand)
	}
}

func TestBuildPlanRequiresServiceUserOwnedConfig(t *testing.T) {
	plan, err := BuildPlan(PlanInput{
		Executable:                   "/usr/local/bin/nginx-manager",
		ServiceUser:                  "nginx-manager",
		CurrentUser:                  "admin",
		ManagementPasswordConfigured: true,
		PrivilegeReady:               true,
		PathsConfigured:              true,
		NginxConfigOK:                true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Ready {
		t.Fatal("expected plan to remain blocked when CLI runs as another user")
	}
	if plan.RequiredReady != 2 || plan.RequiredTotal != 5 {
		t.Fatalf("required progress = %d/%d", plan.RequiredReady, plan.RequiredTotal)
	}
	steps := map[string]PlanStep{}
	for _, step := range plan.Steps {
		steps[step.ID] = step
	}
	if steps["service_user"].Complete || steps["management_password"].Complete || steps["privilege"].Complete {
		t.Fatalf("service-user dependent steps unexpectedly complete: %+v", steps)
	}
	if len(steps["management_password"].Commands) == 0 || !strings.Contains(steps["management_password"].Commands[0], "auth set-password") {
		t.Fatalf("missing password migration command: %+v", steps["management_password"])
	}
}

func TestBuildPlanOmitsSystemdCommandsWhenUnavailable(t *testing.T) {
	plan, err := BuildPlan(PlanInput{
		Executable:                   "/usr/local/bin/nginx-manager",
		ServiceUser:                  "nginx-manager",
		CurrentUser:                  "nginx-manager",
		ManagementPasswordConfigured: true,
		PrivilegeReady:               true,
		PathsConfigured:              true,
		NginxConfigOK:                true,
		CertbotAvailable:             true,
	})
	if err != nil {
		t.Fatal(err)
	}
	steps := map[string]PlanStep{}
	for _, step := range plan.Steps {
		steps[step.ID] = step
	}
	if len(steps["service"].Commands) != 0 || len(steps["renewal"].Commands) != 0 {
		t.Fatalf("systemd commands should be absent: service=%v renewal=%v", steps["service"].Commands, steps["renewal"].Commands)
	}
}

func TestAccountName(t *testing.T) {
	for input, want := range map[string]string{
		"nginx-manager":         "nginx-manager",
		"DOMAIN\\nginx-manager": "nginx-manager",
		"/users/nginx-manager":  "nginx-manager",
	} {
		if got := accountName(input); got != want {
			t.Fatalf("accountName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestBuildPlanRejectsRelativeExecutable(t *testing.T) {
	if _, err := BuildPlan(PlanInput{Executable: "nginx-manager"}); err == nil {
		t.Fatal("expected error")
	}
}
