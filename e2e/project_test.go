//go:build e2e

// e2e suite mirroring console apps/server-nestjs/test coverage, driven by
// client-go (dynamic client + discovery mapper) — no kubectl binary, only a
// kubeconfig. Run: go test -tags e2e -v -timeout 30m
package e2e

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/kustomize/api/filesys"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/yaml"
)

const ns = "default"

var (
	cfg  *rest.Config
	dyn  dynamic.Interface
	disc discovery.DiscoveryInterface
	rmap meta.RESTMapper
)

var (
	projectGVK = schema.GroupVersionKind{Group: "cloud-pi-native.fr", Version: "v1alpha1", Kind: "Project"}
	rbacRoleGVK    = schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "Role"}
	rbacBindingGVK = schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "RoleBinding"}
	svcGVK     = schema.GroupVersionKind{Group: "cloud-pi-native.fr", Version: "v1alpha1", Kind: "Integration"}
	secretGVK  = schema.GroupVersionKind{Group: "cloud-pi-native.fr", Version: "v1alpha1", Kind: "ProjectSecret"}
	envGVK     = schema.GroupVersionKind{Group: "cloud-pi-native.fr", Version: "v1alpha1", Kind: "Environment"}
	stackGVK   = schema.GroupVersionKind{Group: "pulumi.com", Version: "v1", Kind: "Stack"}
	rgdGVK     = schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "ResourceGraphDefinition"}
	crdGVK     = schema.GroupVersionKind{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"}
	croleGVK   = schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole"}
)

func setup(t *testing.T) {
	t.Helper()
	if dyn != nil {
		return
	}
	var err error
	cfg, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(), nil).ClientConfig()
	if err != nil {
		t.Fatalf("kubeconfig: %v", err)
	}
	cfg.Timeout = 60 * time.Second
	if dyn, err = dynamic.NewForConfig(cfg); err != nil {
		t.Fatalf("dynamic client: %v", err)
	}
	if disc, err = discovery.NewDiscoveryClientForConfig(cfg); err != nil {
		t.Fatalf("discovery client: %v", err)
	}
	refreshMapper(t)
}

func refreshMapper(t *testing.T) {
	t.Helper()
	gr, err := restmapper.GetAPIGroupResources(disc)
	if err != nil {
		t.Fatalf("discovery: %v", err)
	}
	rmap = restmapper.NewDiscoveryRESTMapper(gr)
}

func mapping(t *testing.T, gvk schema.GroupVersionKind) *meta.RESTMapping {
	t.Helper()
	setup(t)
	m, err := rmap.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		refreshMapper(t)
		if m, err = rmap.RESTMapping(gvk.GroupKind(), gvk.Version); err != nil {
			t.Fatalf("no RESTMapping for %s: %v", gvk, err)
		}
	}
	return m
}

func resource(t *testing.T, gvk schema.GroupVersionKind, namespace string) dynamic.ResourceInterface {
	t.Helper()
	m := mapping(t, gvk)
	if m.Scope.Name() == meta.RESTScopeNameNamespace {
		return dyn.Resource(m.Resource).Namespace(namespace)
	}
	return dyn.Resource(m.Resource)
}

func applyObject(t *testing.T, obj *unstructured.Unstructured) {
	t.Helper()
	if obj.GetNamespace() == "" &&
		mapping(t, obj.GroupVersionKind()).Scope.Name() == meta.RESTScopeNameNamespace {
		obj.SetNamespace(ns)
	}
	ri := resource(t, obj.GroupVersionKind(), obj.GetNamespace())
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cur, err := ri.Get(ctx, obj.GetName(), metav1.GetOptions{})
	switch {
	case errors.IsNotFound(err):
		if _, err := ri.Create(ctx, obj, metav1.CreateOptions{}); err != nil {
			t.Fatalf("create %s %s: %v", obj.GetKind(), obj.GetName(), err)
		}
	case err != nil:
		t.Fatalf("get %s %s: %v", obj.GetKind(), obj.GetName(), err)
	default:
		obj.SetResourceVersion(cur.GetResourceVersion())
		if _, err := ri.Update(ctx, obj, metav1.UpdateOptions{}); err != nil {
			t.Fatalf("update %s %s: %v", obj.GetKind(), obj.GetName(), err)
		}
	}
}

func applyKustomize(t *testing.T, dir string) {
	t.Helper()
	setup(t)
	rm, err := krusty.MakeKustomizer(krusty.MakeDefaultOptions()).Run(filesys.MakeFsOnDisk(), dir)
	if err != nil {
		t.Fatalf("kustomize build %s: %v", dir, err)
	}
	for _, r := range rm.Resources() {
		m, err := r.Map()
		if err != nil {
			t.Fatalf("kustomize resource %s: %v", r.GetName(), err)
		}
		applyObject(t, &unstructured.Unstructured{Object: m})
	}
}

func fromYAML(t *testing.T, manifest string) *unstructured.Unstructured {
	t.Helper()
	obj := &unstructured.Unstructured{}
	if err := yaml.Unmarshal([]byte(manifest), &obj.Object); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	return obj
}

func get(t *testing.T, gvk schema.GroupVersionKind, name string) *unstructured.Unstructured {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	u, err := resource(t, gvk, ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get %s %s: %v", gvk.Kind, name, err)
	}
	return u
}

func del(t *testing.T, gvk schema.GroupVersionKind, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := resource(t, gvk, ns).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
		t.Fatalf("delete %s %s: %v", gvk.Kind, name, err)
	}
}

func patch(t *testing.T, gvk schema.GroupVersionKind, name, patchJSON string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := resource(t, gvk, ns).Patch(ctx, name, types.MergePatchType,
		[]byte(patchJSON), metav1.PatchOptions{}); err != nil {
		t.Fatalf("patch %s %s: %v", gvk.Kind, name, err)
	}
}

func field(t *testing.T, u *unstructured.Unstructured, fields ...string) string {
	t.Helper()
	v, ok, err := unstructured.NestedFieldNoCopy(u.Object, fields...)
	if err != nil || !ok {
		t.Fatalf("field %v missing on %s %s", fields, u.GetKind(), u.GetName())
	}
	return fmt.Sprintf("%v", v)
}

func condTrue(u *unstructured.Unstructured, condType string) bool {
	conds, _, _ := unstructured.NestedSlice(u.Object, "status", "conditions")
	for _, c := range conds {
		cm, _ := c.(map[string]interface{})
		if cm["type"] == condType && cm["status"] == "True" {
			return true
		}
	}
	return false
}

func waitTrue(t *testing.T, gvk schema.GroupVersionKind, name, condType string, timeout time.Duration) {
	t.Helper()
	setup(t)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		u, err := resource(t, gvk, ns).Get(ctx, name, metav1.GetOptions{})
		cancel()
		if err == nil && condTrue(u, condType) {
			return
		}
		time.Sleep(10 * time.Second)
	}
	t.Fatalf("%s %s %s not True after %s", gvk.Kind, name, condType, timeout)
}

func TestInstall(t *testing.T) {
	for _, crdName := range []string{
		"resourcegraphdefinitions.kro.run",
		"stacks.pulumi.com",
		"programs.pulumi.com",
		"workspaces.auto.pulumi.com",
	} {
		get(t, crdGVK, crdName)
	}
	get(t, rgdGVK, "cpin-project")
}

// TestProjectChain: apply overlay, project goes Ready, delete cascades.
func TestProjectChain(t *testing.T) {
	overlay := os.Getenv("CPIN_OVERLAY")
	if overlay == "" {
		overlay = "default"
	}
	name := "detonation"
	t.Cleanup(func() { del(t, projectGVK, name) })

	t.Run("apply", func(t *testing.T) {
		applyKustomize(t, "../manifests/overlays/"+overlay)
	})
	t.Run("full-example-applied", func(t *testing.T) {
		b, err := os.ReadFile("full-example.yaml")
		if err != nil {
			t.Fatal(err)
		}
		for _, doc := range strings.Split(string(b), "\n---\n") {
			applyObject(t, fromYAML(t, doc))
		}
	})
	t.Run("project-ready", func(t *testing.T) {
		waitTrue(t, projectGVK, name, "Ready", 10*time.Minute)
	})
	t.Run("group-bindings-seeded", func(t *testing.T) {
		waitTrue(t, projectGVK, name, "Ready", 5*time.Minute)
		for _, suffix := range []string{"admin", "devops", "developer", "readonly", "security"} {
			b := get(t, rbacBindingGVK, name+"-"+suffix)
			if out := field(t, b, "subjects", "0", "kind"); out != "Group" {
				t.Fatalf("%s binding subject kind: %q", suffix, out)
			}
		}
	})
	t.Run("standard-roles-seeded", func(t *testing.T) {
		for _, suffix := range []string{"admin", "devops", "developer", "readonly", "security"} {
			r := get(t, rbacRoleGVK, name+"-"+suffix)
			if out := field(t, r, "metadata", "labels", "cloud-pi-native.fr/project"); out != name {
				t.Fatalf("%s attachment label: %q", suffix, out)
			}
		}
	})
	t.Run("stacks-ready", func(t *testing.T) {
		for _, svc := range []string{"keycloak", "gitlab", "harbor"} {
			waitTrue(t, stackGVK, name+"-"+svc, "Ready", 15*time.Minute)
		}
	})
	t.Run("outputs-projected", func(t *testing.T) {
		if svc := field(t, get(t, projectGVK, name), "status", "services"); !strings.Contains(svc, "keycloak") {
			t.Fatalf("services not projected: %q", svc)
		}
	})
	t.Run("delete-cascades", func(t *testing.T) {
		del(t, projectGVK, name)
		deadline := time.Now().Add(10 * time.Minute)
		for time.Now().Before(deadline) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			_, err := resource(t, projectGVK, ns).Get(ctx, name, metav1.GetOptions{})
			cancel()
			if errors.IsNotFound(err) {
				return
			}
			time.Sleep(10 * time.Second)
		}
		t.Fatalf("project %s and its stacks not gone after 10m", name)
	})
}

// TestIntegrations.
func TestIntegrations(t *testing.T) {
	t.Cleanup(func() { del(t, svcGVK, "detonation-keycloak") })
	applyObject(t, fromYAML(t, `apiVersion: cloud-pi-native.fr/v1alpha1
kind: Integration
metadata:
  name: detonation-keycloak
spec:
  project: detonation
  service: keycloak
  enabled: true
`))
	if out := field(t, get(t, svcGVK, "detonation-keycloak"), "spec", "service"); out != "keycloak" {
		t.Fatalf("service: %q", out)
	}
	del(t, svcGVK, "detonation-keycloak")
}

// TestProjectSecrets.
func TestProjectSecrets(t *testing.T) {
	t.Cleanup(func() { del(t, secretGVK, "detonation-sonar-token") })
	applyObject(t, fromYAML(t, `apiVersion: cloud-pi-native.fr/v1alpha1
kind: ProjectSecret
metadata:
  name: detonation-sonar-token
spec:
  project: detonation
  key: sonarqube.token
  value: sq-token
`))
	if out := field(t, get(t, secretGVK, "detonation-sonar-token"), "spec", "key"); out != "sonarqube.token" {
		t.Fatalf("key: %q", out)
	}
	del(t, secretGVK, "detonation-sonar-token")
}

// TestEnvironments: clusterRef + quota fields.
func TestEnvironments(t *testing.T) {
	t.Cleanup(func() { del(t, envGVK, "detonation-dev") })
	applyObject(t, fromYAML(t, `apiVersion: cloud-pi-native.fr/v1alpha1
kind: Environment
metadata:
  name: detonation-dev
spec:
  projectRef: {group: cloud-pi-native.fr, kind: Project, name: detonation}
  name: dev
  clusterRef: {group: cloud-pi-native.fr, kind: Cluster, name: nishir}
  stageRef: {group: cloud-pi-native.fr, kind: Stage, name: dev}
  resources:
    requests:
      cpu: "2"
      memory: 4Gi
      gpu: 0
`))
	if out := field(t, get(t, envGVK, "detonation-dev"), "spec", "clusterRef", "name"); out != "nishir" {
		t.Fatalf("cluster: %q", out)
	}
	del(t, envGVK, "detonation-dev")
}

// TestRBAC asserts the fine-grained cluster roles exist with rules.
func TestRBAC(t *testing.T) {
	for _, role := range []string{
		"cpin:project-read", "cpin:project-write", "cpin:project-roles-admin",
		"cpin:project-members-admin", "cpin:project-integrations-admin",
		"cpin:project-secrets-admin", "cpin:environment-admin",
		"cpin-admin:operator", "cpin-admin:reader", "kro:cpin-orchestrator",
	} {
		rules, ok, _ := unstructured.NestedSlice(get(t, croleGVK, role).Object, "rules")
		if !ok || len(rules) == 0 {
			t.Fatalf("clusterrole %s missing or empty", role)
		}
	}
}
