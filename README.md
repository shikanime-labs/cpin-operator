# cpin-operator

cloud-pi-native console and socle, reimplemented as KRO ResourceGraphDefinitions
backed by Pulumi programs — no controllers, no database.

**Deploy path:** `apps/cpin-operator/overlays/<cluster>` — rendered by Flux
(`clusters/<c>/components/cpin/ks.yaml` in shikanime-labs/manifests) with kro +
pulumi-kubernetes-operator installed under `infrastructure/`.

## Layout

- `apps/cpin-operator/base/` — the deployable unit: Project + topology RGDs
  (`project-rgd.yaml`, `topology.yaml`), operator RBAC (`backend-rbac.yaml`),
  and one `pulumi.com/v1 Program` per service under `programs/` (generated
  from `programs/<svc>/Pulumi.yaml` — edit the source, regenerate the CR)
- `apps/cpin-operator/overlays/<cluster>/` — per-cluster composition entrypoint
- `programs/<svc>/Pulumi.yaml` — source of truth per Program CR (keycloak,
  gitlab, harbor, sonarqube, nexus, vault, argocd = console sync plane;
  socle = service-install plane, helm Releases pinned to socle versions.md)
- `manifests/` — POC-era files (superseded by `apps/`; deletion pending)

## CRD model (console data structures as KRO CRDs)

| Console entity | CRD | Scope | Provisions via |
| --- | --- | --- | --- |
| Project | `Project` | Namespaced | one Stack per `spec.services` (KRO forEach) |
| Zone | `Zone` | Cluster | vault Stack (zone mount + AppRole) |
| Cluster | `Cluster` | Cluster | kubeconfig secret ref (metadata) |
| Stage | `Stage` | Cluster | none — name-only dimension |
| Environment / roles / members | Project sub-fields | — | fan into every service Stack config |

Owner exclusive of members (admission CEL). Permissions stay integer bitmasks.

## Conventions

- One kind per file, `<short-name>.yaml`, sorted `resources:` lists
- After editing `programs/<svc>/Pulumi.yaml`, regenerate
  `apps/.../programs/<svc>-program.yaml` (python3 + yaml.safe_dump)
- Render gate: `kubectl kustomize apps/cpin-operator/overlays/<cluster>`
- Beyond POC: replace `backend: file:///share` per Stack with a real backend

## Commit Style

- Plain-text capitalized title, no conventional-commit prefix
- Trailers: `Signed-off-by: Shikanime Deva <william.phetsinorath@shikanime.studio>`
  and `Co-authored-by: Automata <automata@shikanime.studio>`
