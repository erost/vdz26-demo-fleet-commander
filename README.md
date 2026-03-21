# Fleet Commander Demo

This is a demonstrator, with public available data and components only, of the conference talk [BREAK THE SILO: How we built a decentralized control plan for service orchestration](https://github.com/juliusbaer/conferences/blob/main/2026/voxxed-days-ticino/Fleet%20Commander%20-%20Voxxed%20Days%20Ticino%202026.pdf)

[Voxxed Days Ticino 2026 Recording](https://www.youtube.com/watch?v=136SC6nKJqc)

## Concept

The concept of the demo is simple:
1. apply a custom resource on the commander (Public API)
2. the public composition function will decompose it in a provider-kubernetes `Object` that wraps the internal custom resource
3. provider-kubernetes applies the custom resources on the Unit cluster
4. the internal composition decomposes it on the final object: for simplicita, a provider-nop `NopResource`

In a productive setup, provider-nop is replaced by providers managing real resources.

```mermaid
flowchart TD
    KUBECTL["kubectl apply"]

    subgraph COMMANDER["Commander (Public API)"]
        API["Custom Resources"]
        PC["public-compositions"]
        PK["provider-kubernetes"]
        API --> PC
        PC --> PK
    end

    subgraph UNIT-NUMBER["Unit-Numbers"]
        NAPI["Kind: Number"]
        NIC["internal-composition"]
        NPN["provider-noop"]
        NAPI --> NIC
        NIC --> NPN
    end

    subgraph UNIT-STRINGS["Unit-Strings"]
        SAPI["Kind: String"]
        SIC["internal-composition"]
        SPN["provider-noop"]
        SAPI --> SIC
        SIC --> SPN
    end

    KUBECTL --> API
    PK --> NAPI
    PK --> SAPI
```

## Requirements
- [mise](https://mise.jdx.dev/) — tool version manager and task runner

All other tools (kind, kubectl, crossplane CLI, Go) are managed by mise and installed automatically.

### Development Requirements
- [Docker](https://www.docker.com/) with [buildx](https://github.com/docker/buildx) support
- A container registry account with push access (e.g. ghcr.io) or a [local registry](https://hub.docker.com/_/registry)

## Run the demo locally

```sh
mise all:setup
mise all:deploy
```

Once the commander and the units are deployed, deploy a test custom resource with:

```sh
mise numbers:create
mise strings:create
```

and use `kubectl` to verify the state.

```sh
# cleanup the examples
mise numbers:delete
mise strings:delete
```

### Use custom registry

`imagePullSecrets` and `packagePullSecrets` should be manually configured on the clusters.

## Clean up

```sh
mise all:teardown
```

## How to build

Function images are pushed to an OCI registry and pulled by Crossplane inside each KinD cluster.
The registry is configurable so the project can be forked without code changes.

`docker login` is required before starting.

### Create a local config file

Copy the example and fill in your values:

```sh
cp mise.local.toml.example mise.local.toml
```

`mise.local.toml` is gitignored and never committed.

Configure the `BUILD_` variables according to your setup.

| Variable         | Description                                            | Example         |
|------------------|--------------------------------------------------------|-----------------|
| `BUILD_REGISTRY` | Registry prefix including namespace, no trailing slash | `ghcr.io/erost` |
| `BUILD_VERSION`  | Valid Semver format                                    | `0.0.1`         |

### Build compositions

Check the `[tasks."composition:build"]` and `[tasks."composition:build:all"] for details.

New compositions can be added as long as they follow the correct convention.

```
commander/public-function-<name>/             # the implementation of the public composition
commander/public-function-<name>/chart        # the chart to deploy the composition
units/unit-<purpose>/function-<name>/         # the implementation of the internal composition
units/unit-<purpose>/function-<name>/chart    # the chart to deploy the internal composition
```

Once built, the new composition can be configured in `deployment.yaml`.

### Other resources

```
.scripts                          # build and deploy scripts
.scripts/kind                     # kind cluster configurations
common/deployment/*.yaml          # additional deployment configuration (e.g.: providers)
common/generic-composition-chart  # a library chart used as the base for all compositions
deployment.yaml                   # simplified deployment descriptor for all components
```

## Troubleshooting

### Too many open files

Running multiple `kind` clusters may result in issues with too many open files.

Example:

```sh
❯ kubectl --context=kind-commander -n kube-system logs -l k8s-app=kube-proxy
E0319 21:27:41.893683       1 run.go:72] "command failed" err="failed complete: too many open files"
```

Using colima, add the following entry to `colima.yaml`:

```yaml
provision:
  - mode: system
    script: |
      sysctl -w fs.inotify.max_user_instances=1280
      sysctl -w fs.inotify.max_user_watches=655360
```

Using another docker runtime may require a different solution.
