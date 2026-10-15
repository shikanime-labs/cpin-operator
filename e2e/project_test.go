//go:build e2e

// e2e: applies manifests/overlays/<cluster> (RGDs + Program CRs) to the
// current kubectl context, creates a Project, and asserts the KRO→PKO chain:
// per-service Stacks spawn, go Ready, and their outputs project back into
// Project.status via CEL. Requires kro + pulumi-kubernetes-operator installed.
// Run: go test -tags e2e -v -timeout 20m
package e2e

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func kubectl(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "kubectl", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("kubectl %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(bytes.TrimSpace(out))
}

func jsonpath(t *testing.T, resource, expr string) string {
	t.Helper()
	return kubectl(t, "get", resource, "-o", "jsonpath="+expr)
}

func waitReady(t *testing.T, resource, name, condExpr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		v := jsonpath(t, resource+" "+name, condExpr)
		if v == "True" {
			return
		}
		time.Sleep(10 * time.Second)
	}
	t.Fatalf("%s %s not ready after %s: %s", resource, name, timeout,
		jsonpath(t, resource+" "+name, "{.status.conditions}"))
}

func TestProjectChain(t *testing.T) {
	overlay := os.Getenv("CPIN_OVERLAY")
	if overlay == "" {
		overlay = "nishir"
	}

	t.Run("apply", func(t *testing.T) {
		kubectl(t, "apply", "-k", "manifests/overlays/"+overlay)
	})

	t.Run("crds-established", func(t *testing.T) {
		deadline := time.Now().Add(2 * time.Minute)
		for time.Now().Before(deadline) {
			out := kubectl(t, "get", "crd", "projects.cpin.shikanime.studio", "--ignore-not-found")
			if out != "" {
				return
			}
			time.Sleep(5 * time.Second)
		}
		t.Fatal("projects.cpin.shikanime.studio CRD not established")
	})

	t.Run("project-applied", func(t *testing.T) {
		kubectl(t, "apply", "-f", "e2e/project.yaml")
	})

	t.Run("project-ready", func(t *testing.T) {
		waitReady(t, "project", "detonation",
			"{.status.conditions[?(@.type==\"Ready\")].status}", 10*time.Minute)
	})

	t.Run("stacks-ready", func(t *testing.T) {
		for _, svc := range []string{"keycloak", "gitlab", "harbor"} {
			waitReady(t, "stack", "detonation-"+svc,
				"{.status.conditions[?(@.type==\"Ready\")].status}", 15*time.Minute)
		}
	})

	t.Run("outputs-projected", func(t *testing.T) {
		svc := jsonpath(t, "project detonation", "{.status.services}")
		want := "[keycloak gitlab harbor]"
		if !strings.Contains(svc, "keycloak") {
			t.Fatalf("services not projected: got %q want containing keycloak (from %q)", svc, want)
		}
	})

	t.Run("cleanup", func(t *testing.T) {
		kubectl(t, "delete", "project", "detonation", "--ignore-not-found", "--wait=false")
	})
}
