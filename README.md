# cpin-operator

cloud-pi-native console and socle, reimplemented as KRO ResourceGraphDefinitions
backed by Pulumi programs — no controllers, no database.

```
Project CR ──► KRO (RGD) ──► pulumi.com/v1 Stack ×N ──► PKO workspace pods ──► pulumi up
     ▲                                                              │
     └──────────── status ◄── CEL ◄── Stack.status.outputs ◄────────┘
```

## Installation

Dependencies, in order. Versions below are the proven-on cluster pair; newer
may work.

1. **kro** (ResourceGraphDefinition controller):

   ```sh
   kubectl apply -f https://github.com/kubernetes-sigs/kro/releases/latest/download/kro-with-crds.yaml
   kubectl -n kro-system wait deploy/kro --for=condition=Available --timeout=300s
   ```

2. **pulumi-kubernetes-operator** (PKO v2, `Stack` + `Program` + `Workspace`):

   ```sh
   kubectl apply -f https://github.com/pulumi/pulumi-kubernetes-operator/releases/latest/download/pulumi-kubernetes-operator-crds.yaml
   helm repo add pulumi https://pulumi.github.io/pulumi-kubernetes-operator
   helm install -n pulumi-operator-system --create-namespace pko pulumi/pulumi-kubernetes-operator
   ```

3. **Pulumi state backend + passphrase** (dev: file backend on a PVC or
   hostPath; prod: point each Stack's `backend` at a real service):

   ```sh
   kubectl create secret generic pulumi-passphrase --from-literal=PULUMI_CONFIG_PASSPHRASE=replace-me
   ```

4. **RBAC for the chain** (kro creates/owns Stacks; PKO workspaces need
   TokenReview): included in `manifests/base/rbac.yaml`.

5. **This repo**:

   ```sh
   kubectl apply -k manifests/overlays/default
   kubectl wait --for=condition=Established crd/projects.cloud-pi-native.fr
   ```

Verify: `kubectl get rgd` — all nine `cpin-*` Active; then `kubectl apply -f
e2e/project.yaml && watch kubectl get stacks`.

## Layout

```
manifests/base/resourcegraphdefinition.yaml
                                    multi-doc RGDs: Project (seeds the five
                                    standard ProjectRoles), Zone/Cluster/Stage,
                                    project-scoped Role/Member/Service/Secret/
                                    Environment
manifests/base/rbac.yaml            fine-grained + standard-role ClusterRoles,
                                    backend chain RBAC (single rbac.yaml per
                                    studio convention)
manifests/components/<svc>/           pulumi.com/v1 Program CRs, one kustomize
                                    Component per service (<svc>/program.yaml);
                                    overlays compose them per cluster
manifests/overlays/<cluster>/       cluster entrypoint (base + selected service
                                    components); default overlay: default/
e2e/                                Go e2e suite mirroring console
                                    apps/server-nestjs/test coverage
```

Deploy path: Flux `clusters/<c>/components/cpin/ks.yaml` in
shikanime-labs/manifests points at `./manifests/overlays/<c>`.

## CRD model (console entities → KRO)

| Console entity | CRD | Scope | Provisions via |
| --- | --- | --- | --- |
| Project | `Project` | Namespaced | one Stack per `spec.services` (forEach) |
| ProjectRole | `ProjectRole` | Namespaced | aggregate in Project Stack config |
| ProjectMember | `ProjectMember` | Namespaced | aggregate in Project Stack config |
| ProjectService | `ProjectService` | Namespaced | aggregate in Project Stack config |
| ProjectSecret | `ProjectSecret` | Namespaced | aggregate in Project Stack config |
| Environment | `Environment` | Namespaced | aggregate in Project Stack config |
| Zone | `Zone` | Cluster | declarative topology (vault mount via socle) |
| Cluster | `Cluster` | Cluster | kubeconfig secret ref |
| Stage | `Stage` | Cluster | name-only dimension |

Project is the Gateway-API-style "hat". Users live in the IdP (Keycloak):
no ProjectMember CR — the K8s plane binds OIDC groups (`kind: Group`
subjects, `/slug/console/<role>`) to the seeded native Roles, and each
service resolves groups at login from its `rbac.<svc>.cloud-pi-native.fr`
Role contract CRs (Harbor OIDC group→roleId, GitLab group→accessLevel).
Environments attach by label (`cloud-pi-native.fr/project: <name>`); the
Project RGD reads them through externalRef collections with reactive
re-sync — same role the HTTPRoute-to-Gateway reference plays. — each maps to a console
module and a dedicated ClusterRole (`cpin:project-roles-admin`,
`cpin:project-members-admin`, `cpin:project-services-admin`,
`cpin:project-secrets-admin`, `cpin:environment-admin`). Permissions stay
integer bitmasks (decode at API layer).

## RBAC

Fine-grained roles in `manifests/base/project-scoped-rbac.yaml` mirror the
console module split, so a platform team can own member management without
project mutation rights:

```sh
kubectl auth reconcile -f manifests/base/project-scoped-rbac.yaml
```

## E2E

```sh
go test -tags e2e -v -timeout 30m        # against the current kubectl context
CPIN_OVERLAY=<cluster> go test ...       # non-default overlay
```

Coverage mirrors console `apps/server-nestjs/test`: project lifecycle +
KRO→PKO chain, roles, members, services, secrets, environments, RBAC roles.

## Conventions

- `<resource-name>.yaml` with multi-doc files per resource; RBAC exempt as a
  single `rbac.yaml`, sorted `resources:` lists
- Programs are CRDs, not generated artifacts — edit
  `manifests/components/<svc>/program.yaml` directly
- Render gate: `kubectl kustomize manifests/overlays/<cluster>`
- Beyond POC: replace `backend: file:///share` per Stack with a real backend

## Commit Style

- Plain-text capitalized title, no conventional-commit prefix
- Trailers: `Signed-off-by: Shikanime Deva <william.phetsinorath@shikanime.studio>`
  and `Co-authored-by: Automata <automata@shikanime.studio>`
