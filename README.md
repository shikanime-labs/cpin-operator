# cpin-operator

cloud-pi-native console, reimplemented as a controller-free stack:
**KRO** gives us the `Project` CRD, **Pulumi Kubernetes Operator** gives us the reconcile loop,
**Pulumi programs** hold the provisioning logic that was 12k lines of NestJS.

```
Project CR ──► KRO ResourceGraphDefinition ──► pulumi.com/v1 Stack (one per project)
                                                    │
                                                PKO controller ──► workspace pod ──► pulumi up
                                                    │
                        Project.status ◄── CEL ◄── Stack.status.outputs
```

Deletion: Project delete → KRO deletes the Stack → PKO finalizer runs `pulumi destroy`
(`destroyOnFinalize: true`). No finalizer code on our side.

## Layout

```
manifests/resourcegroup.yaml  the Project CRD-as-code (KRO RGD, v0.9.4 shape)
manifests/program.yaml        inline Pulumi YAML Program (stand-in resources; replace per service)
manifests/rbac.yaml           REQUIRED: kro cannot watch the CRDs it orchestrates without it
manifests/auth-delegator.yaml REQUIRED: PKO workspace agent grpc auth (TokenReview)
example/project.yaml          sample Project instance
```

## Install

```sh
kubectl apply -f https://github.com/kubernetes-sigs/kro/releases/download/v0.9.4/kro-core-install-manifests.yaml
curl -sLO https://github.com/pulumi/pulumi-kubernetes-operator/releases/download/pulumi-kubernetes-operator-2.9.3/pulumi-kubernetes-operator-2.9.3.tgz
helm install pko ./pulumi-kubernetes-operator-2.9.3.tgz -n pulumi-operator-system --create-namespace
# passphrase secret for the local file backend (POC only)
kubectl create secret generic pulumi-passphrase --from-literal=PULUMI_CONFIG_PASSPHRASE=changeme
kubectl apply -f manifests/ && kubectl apply -f example/project.yaml
```

## Verified behavior (kind + rancher-desktop, 2026-10-15)

- Project `detonation2` → Stack Ready in ~30s; `status.services = [keycloak, gitlab, harbor]`
  projected from `Stack.status.outputs` via CEL.
- `kubectl delete project` cascades: Stack destroyed, workspace pod completed, gone.

## Field-verified pitfalls

- **KRO v0.9.4 ≠ old docs**: `spec.schema` + `spec.resources`, not `spec.template`;
  SimpleSchema fields are structured objects.
- **CEL dialect**: JSONPath `[?(@.x=='y')]` is a parse error; use
  `.filter(c, c.x == 'y')[0]`.
- **KRO RBAC**: without `manifests/rbac.yaml` the generated CRD's controller 403s silently.
- **PKO cross-namespace refs are forbidden** (v2.0+): Project and its secrets share a namespace.
- **Never patch volumes on a live Stack/Workspace**: PKO's workspace controller wedges on the
  statefulset immutable-field replace (`failed to delete statefulset: not found` loop).
  Change spec via full re-create (delete Stack → re-apply RGD), or start with the right shape.
- **File backend**: PKO workspace pod's `/share` dir is the only path guaranteed to exist;
  `/share/source` is wiped by the fetch init container on restart. The workspace `share` volume
  is emptyDir — PVC persistence via `workspaceTemplate.podTemplate` is untested against this
  wedge. Real deployments should use a real backend (Pulumi Cloud / S3 / object store).
