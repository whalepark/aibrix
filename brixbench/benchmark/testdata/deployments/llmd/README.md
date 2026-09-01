# llm-d Deployment Notes

This directory contains Kubernetes manifests and Helm values for the Brixbench
llm-d scenario. It does not contain container images or model artifacts.

Model files are expected to already exist on the test cluster under
`/data01/models`, and the modelserver manifest mounts that host path.

## Images

The llm-d Brixbench testdata uses internal registry image names. If an image is
missing, mirror the matching upstream image before running the smoke benchmark.

### v0.8.1

Modelserver image:

- Source: `guides/recipes/modelserver/components/images/gpu-vllm`
- Upstream image: `vllm/vllm-openai:v0.23.0`
- Brixbench image: `aibrix-public-release-cn-beijing.cr.volces.com/aibrix/llmd-vllm-openai:v0.23.0`

Routing sidecar image:

- Source: `guides/recipes/modelserver/components/images/routing-sidecar`
- Upstream image: `ghcr.io/llm-d/llm-d-router-disagg-sidecar:v0.9.0`
- Brixbench image: `aibrix-public-release-cn-beijing.cr.volces.com/aibrix/llmd-router-disagg-sidecar:v0.9.0`

Router/EPP image:

- Source: `oci://ghcr.io/llm-d/charts/llm-d-router-standalone`
- Chart version: `v0.9.0`
- Upstream EPP image: `ghcr.io/llm-d/llm-d-router-endpoint-picker:v0.9.0`
- Brixbench EPP image: `aibrix-public-release-cn-beijing.cr.volces.com/aibrix/llm-d-router-endpoint-picker:v0.9.0`
- Upstream Envoy image: `docker.io/envoyproxy/envoy:distroless-v1.33.2`
- Brixbench Envoy image: `aibrix-public-release-cn-beijing.cr.volces.com/aibrix/envoyproxy-envoy:distroless-v1.33.2`

### v0.9.0

Modelserver image:

- Source: `guides/recipes/modelserver/components/images/gpu-vllm/release` (tag `v0.9.0`)
- Upstream image: `vllm/vllm-openai:v0.26.0`
- Brixbench image: `aibrix-public-release-cn-beijing.cr.volces.com/aibrix/llmd-vllm-openai:v0.26.0`

Routing sidecar image:

- Source: `guides/recipes/modelserver/components/images/routing-sidecar/release` (tag `v0.9.0`)
- Upstream image: `ghcr.io/llm-d/llm-d-router-disagg-sidecar:v0.10.0`
- Brixbench image: `aibrix-public-release-cn-beijing.cr.volces.com/aibrix/llmd-router-disagg-sidecar:v0.10.0`

Router/EPP image:

- Source: `oci://ghcr.io/llm-d/charts/llm-d-router-standalone`
- Chart version: `v0.10.0`
- Upstream EPP image: `ghcr.io/llm-d/llm-d-router-endpoint-picker:v0.10.0`
- Brixbench EPP image: `aibrix-public-release-cn-beijing.cr.volces.com/aibrix/llm-d-router-endpoint-picker:v0.10.0`
- Upstream Envoy image: `docker.io/envoyproxy/envoy:distroless-v1.33.2`
- Brixbench Envoy image: `aibrix-public-release-cn-beijing.cr.volces.com/aibrix/envoyproxy-envoy:distroless-v1.33.2`

## Scenario `version` vs router chart version

Scenario YAML `version` is the **llm-d git release tag**. The deployer
validates that tag with `git ls-remote` against
`https://github.com/llm-d/llm-d.git` (no local llm-d checkout / `LLMD_REPO`
required) and selects the matching standalone router chart:

- `v0.8.1` → chart `v0.9.0` and `router/base-values.yaml`
- `v0.9.0` and later → chart `v0.10.0` and `router/base-values-v0.9.0.yaml`

Keep version-specific fixtures separate. Do not change v0.8.1 files when
adding a newer llm-d release.

## Engine deployment name contract

`LLMdDeployer.WaitForReady` currently waits on fixed Deployment names for the
checked-in P/D smoke manifests:

- `llmd-brixbench-epp` (Helm release `llmd-brixbench`)
- `pd-disaggregation-nvidia-gpu-vllm-prefill`
- `pd-disaggregation-nvidia-gpu-vllm-decode`

Custom engine manifests used with this provider must keep those Deployment
names (or update the deployer constants in lockstep). Arbitrary Deployment
name discovery is not supported yet.

## Routing Policy

v0.8.1 uses the upstream cache-plus-load policy:

```text
brixbench/benchmark/testdata/deployments/llmd/router/policies/pd-disaggregation-official.yaml
```

- prefill: prefix-cache (weight 3), queue (2), KV-cache utilization (2)
- decode: active requests (2), prefix-cache (3)

v0.9.0 uses the upstream token-based P/D policy:

```text
brixbench/benchmark/testdata/deployments/llmd/router/policies/pd-disaggregation-official-v0.9.0.yaml
```

- prefill: prefix-cache affinity filter, token-load scorer, max-score picker
- decode: active-request scorer, max-score picker

The policy is selected by the scenario's ordered `controlplane` Helm values
files. v0.8.1:

```yaml
controlplane:
- brixbench/benchmark/testdata/deployments/llmd/router/base-values.yaml
- brixbench/benchmark/testdata/deployments/llmd/router/policies/pd-disaggregation-official.yaml
```

To benchmark another llm-d policy, create a new values overlay under
`brixbench/benchmark/testdata/deployments/llmd/router/policies/` and reference
it as the second `controlplane` file in a new scenario test under
`brixbench/benchmark/testdata/scenarios/`. The runner passes the files to Helm
in order, so the policy overlay replaces the EPP plugin configuration without
changes to the deployer, modelserver manifest, or benchmark client.

llm-d endpoint routing is a filter, scorer, and picker pipeline rather than a
single `router-mode` value. In particular, `round-robin-fairness-policy`
controls flow-control queue fairness and is not equivalent to Dynamo's
`--router-mode round-robin`. See the [llm-d EPP scheduling
reference](https://github.com/llm-d/llm-d/blob/main/docs/architecture/core/router/epp/scheduling.md)
for supported filters, scorers, and pickers.
