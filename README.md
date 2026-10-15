# cpin-operator

cloud-pi-native console and socle, reimplemented as KRO ResourceGraphDefinitions
backed by Pulumi programs — no controllers, no database.

```
Project CR ──► KRO (RGD) ──► pulumi.com/v1 Stack ×N ──► PKO workspace pods ──► pulumi up
     ▲                                                              │
     └──────────── status ◄── CEL ◄── Stack.status.outputs ◄────────┘
```

## Layout

```
manifests/base/                 Project/Zone/Cluster/Stage RGDs + RBAC
manifests/components/programs/  pulumi.com/v1 Program CRs, one per service
manifests/overlays/<cluster>/   cluster entrypoint (base + programs component)
```

Deploy path: Flux `clusters/<c>/components/cpin/ks.yaml` in
shikanime-labs/manifests points at `./manifests/overlays/<c>`, with kro and
pulumi-kubernetes-operator installed under `infrastructure/`.

## CRD model (console entities → KRO)

| Console entity | CRD | Scope | Provisions via |
| --- | --- | --- | --- |
| Project | `Project` | Namespaced | one Stack per `spec.services` (forEach) |
| Zone | `Zone` | Cluster | vault Stack (zone mount + AppRole) |
| Cluster | `Cluster` | Cluster | kubeconfig secret ref |
| Stage | `Stage` | Cluster | name-only dimension |

Environment, roles, members are Project sub-fields fanned into every Stack
config. Permissions stay integer bitmasks (decode at API layer). Owner is
exclusive of members — admission CEL rejects violations.

## Conventions

- One Kubernetes kind per file, `<short-name>.yaml`, sorted `resources:` lists
- Programs are CRDs, not generated artifacts — edit
  `manifests/components/programs/<svc>-program.yaml` directly
- Render gate: `kubectl kustomize manifests/overlays/<cluster>`
- Beyond POC: replace `backend: file:///share` per Stack with a real backend

## Commit Style

- Plain-text capitalized title, no conventional-commit prefix
- Trailers: `Signed-off-by: Shikanime Deva <william.phetsinorath@shikanime.studio>`
  and `Co-authored-by: Automata <automata@shikanime.studio>`
