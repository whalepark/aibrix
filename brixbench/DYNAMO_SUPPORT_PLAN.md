# NVIDIA AI Dynamo Support Plan

## 목적

`brixbench`에 NVIDIA AI Dynamo provider를 추가한다. 목표는 Dynamo의 모든 배포 옵션을 감싸는 풀 기능 구현이 아니라, AIBrix provider와 같은 benchmark lifecycle 안에서 release version 기반 Dynamo 실행 경로를 제공하는 것이다.

이 문서는 구현 범위, 입력 정책, release 판정 기준, deployer 책임 경계, 그리고 레퍼런스 배포 환경에서 가져올 동작 흐름을 정리한다.

참고한 레퍼런스:

```text
brixbench/.tmp/dynamo-reference/Routing_Test_dynamo_1.2.0-ds-v4-dev3_vllm_0.21.0/
```

## 핵심 결정

- `provider: dynamo`는 `version`만 받는다.
- `commit`, `localPath`, `controlplane`은 Dynamo provider에서 허용하지 않는다.
- Dynamo release source of truth는 `https://github.com/ai-dynamo/dynamo`의 Git tag로 둔다.
- 가능하면 단순 tag 존재 여부가 아니라 `main` branch에서 reachable한 tag인지 확인한다.
- 기본 정책은 stable semver release tag만 허용한다.
- dev, rc, post, vendor-specific tag는 기본 차단하고, 필요할 때 별도 opt-in 정책으로 확장한다.
- node, registry, storage, network, image mirror, RDMA 같은 cluster-specific 설정은 Go 코드에 넣지 않고 사용자가 제공하는 YAML에 남긴다.

## Release Tag 정책

현재 `ai-dynamo/dynamo`에는 다음 계열의 tag가 존재한다.

Stable semver 예:

```text
v0.1.0
v0.1.1
v0.2.0
v0.2.1
v0.3.0
v0.3.1
v0.3.2
v0.4.0
v0.4.1
v0.5.0
v0.5.1
v0.6.0
v0.6.1
v0.7.0
v0.7.1
v0.8.0
v0.8.1
v0.9.0
v0.9.1
v1.0.0
v1.0.1
v1.0.2
v1.1.0
v1.1.1
v1.2.0
v1.2.1
```

Non-stable tag 예:

```text
v0.4.0.post0
v0.7.0.post1
v0.8.1-rc0
v0.9.1-rc0
v0.9.1-rc1
v1.1.0-dev.1
v1.1.0-rc0
v1.2.0-deepseek-v4-dev.3
v1.2.0-sglang-deepseek-v4-dev.1
v1.3.0-dev.1
```

제안하는 입력 normalizer:

- `1.2.1` 입력은 `v1.2.1`로 normalize한다.
- `v1.2.1` 입력은 그대로 둔다.
- stable mode에서는 `^v[0-9]+\.[0-9]+\.[0-9]+$`만 허용한다.
- dev/rc/post tag는 별도 설정이 생기기 전까지 거부한다.

main reachable 검증은 deploy 실행 단계에서 수행한다. resolver unit test가 네트워크에 의존하지 않도록, resolver는 형식과 provider 입력 조합만 검증한다.

구현 후보:

```text
git ls-remote --tags --refs https://github.com/ai-dynamo/dynamo.git
git fetch --depth=1 origin main
git fetch --depth=1 origin refs/tags/<version>:refs/tags/<version>
git merge-base --is-ancestor <version> origin/main
```

실제 구현에서는 shallow fetch의 reachability 한계를 고려해야 한다. 우선순위는 다음과 같다.

1. Network helper는 tag 존재 여부를 빠르게 확인한다.
2. main reachability 확인은 가능한 환경에서만 수행하고, 실패 시 명확한 에러를 낸다.
3. 테스트는 fake tag source를 주입해서 네트워크 없이 검증한다.

## Scenario 입력 형태

최소 Dynamo scenario 예:

```yaml
Scenario: dynamo-vllm-basic
Tests:
  - name: dynamo-v1.2.1-vllm
    provider: dynamo
    version: v1.2.1
    engine:
      type: vllm
      manifest: testdata/deployments/dynamo/vllm-dynamo.yaml
    benchmark: testdata/benchmarks/vllm-chat-smoke.yaml
```

허용하지 않는 입력:

```yaml
provider: dynamo
commit: <sha>          # 금지
localPath: <path>      # 금지
controlplane: [...]    # 금지
```

`engine.manifest`는 사용자가 제공한 Dynamo CR YAML을 그대로 적용한다. brixbench는 CR 내부의 topology, image, routing mode, RDMA, PVC, nodeSelector 같은 세부 설정을 해석하거나 생성하지 않는다. 특정 클러스터에서만 의미가 있는 registry secret, storage path, PVC/PV, node affinity, toleration, CNI/RDMA annotation, image mirror 값도 provider 코드의 상수나 기본값으로 넣지 않는다.

## Deployer 책임 경계

`DynamoDeployer`는 기존 `Deployer` interface를 그대로 따른다.

```text
Initialize
DeployControlPlane
DeployGateway
DeployEngine
WaitForReady
GetGatewayEndpoint
CaptureArtifacts
Teardown
```

각 단계의 책임:

- `Initialize`: namespace, logDir, projectRoot, resolver에서 normalize된 version, engine manifest 경로를 저장한다.
- `DeployControlPlane`: release version 기반으로 Dynamo platform을 설치한다.
- `DeployGateway`: Dynamo에서는 별도 gateway deploy를 하지 않는 no-op이다. Dynamo Frontend service가 serving endpoint 역할을 한다.
- `DeployEngine`: `engine.manifest`의 Dynamo CR을 `kubectl apply`로 적용한다.
- `WaitForReady`: Dynamo operator가 생성한 Frontend/worker workload를 필수로 기다리고, 모델명을 확인할 수 있을 때만 `/v1/models` 또는 lightweight inference probe를 추가 수행한다.
- `GetGatewayEndpoint`: Dynamo Frontend service의 cluster-local URL을 반환한다.
- `CaptureArtifacts`: pods, services, events, Dynamo CR, generated deployments, relevant logs를 저장한다.
- `Teardown`: Dynamo CR finalizer 처리, non-platform workload 삭제, Helm release uninstall, PVC/PV cleanup을 수행한다.

## 레퍼런스 배포 흐름에서 가져올 부분

레퍼런스의 `deploy_multi_prefix.sh`는 다음 흐름을 가진다.

1. namespace 생성 및 기존 Dynamo resource cleanup
2. registry secret 생성
3. Dynamo platform Helm install
4. model PV/PVC 및 MPI secret 생성
5. `DynamoGraphDeployment` 적용
6. operator가 materialize한 Frontend/worker deployments 대기
7. PodMonitor label 및 fallback PodMonitor 적용
8. model registration 및 inference readiness 확인
9. benchmark client pod 실행
10. 결과 수집
11. cleanup

brixbench에 반영할 범위:

- 1, 3, 5, 6, 8, 10, 11은 deployer lifecycle에 맞춰 반영한다.
- 2, 4, 7은 provider config나 별도 manifest hook이 생기기 전까지 최소화한다.
- 9는 기존 benchmark driver 책임이므로 Dynamo deployer에 넣지 않는다.

## 레퍼런스에서 바로 일반화하지 않을 부분

다음 항목은 특정 클러스터와 실험 환경에 강하게 묶여 있으므로 1차 구현 범위에서 제외한다.

- hard-coded node IP 또는 hostname
- hard-coded registry credential
- `/root/models`, `/data01/models` 같은 cluster-local path
- operator image를 직접 build/push하는 흐름
- 특정 AIBrix mirror registry 전제
- 특정 RDMA CNI annotation
- 특정 vLLM runtime image naming convention 강제
- routing test matrix batch runner
- benchmark client pod YAML 자체 생성

이 값들은 사용자가 제공하는 `engine.manifest` 또는 benchmark YAML에 남긴다. brixbench provider는 release 기반 platform 설치와 lifecycle orchestration만 책임진다. 클러스터별 값이 필요해지는 경우에도 Go 코드에 조건 분기나 hard-coded fallback을 추가하지 않고, 명시적인 YAML manifest 또는 향후 `platform.valuesFile` 같은 사용자 제공 hook으로만 전달한다.

## Dynamo platform 설치 방식

1차 구현은 Helm chart 기반 설치를 전제로 한다.

레퍼런스 기준 Dynamo 1.x chart path:

```text
deploy/helm/charts/platform
```

구현 선택지:

- Dynamo source checkout을 `.tmp/dynamo/<version>`에 준비하고 해당 chart path로 `helm upgrade --install`을 수행한다.
- chart archive나 OCI chart가 공식적으로 안정 제공되면, checkout 없이 chart version으로 설치하는 경로를 추가한다.

초기 구현에서는 checkout 기반이 가장 명확하다. 단, 사용자 입력은 checkout path가 아니라 `version` 하나로 유지한다. 내부적으로만 version tag를 checkout한다.

## Version과 image tag 관계

`version`은 Dynamo platform release version이다. runtime image tag까지 자동 파생하는 것은 1차 범위에서 제한적으로만 다룬다.

이유:

- 레퍼런스의 `v1.2.0-deepseek-v4-dev.3` 같은 tag는 stable semver가 아니다.
- runtime image는 vLLM version과 Dynamo flavor가 함께 들어간다.
- operator image mirror 상태는 registry마다 다르다.

따라서 1차 구현 원칙은 다음과 같다.

- platform install은 `version`에서 파생한다.
- engine runtime image는 `engine.manifest` 안에 명시한다.
- provider가 manifest 내부 image를 rewrite하지 않는다.
- 나중에 image policy가 필요해지면 `dynamo.imagePolicy` 같은 명시적 config로 확장한다.

## Readiness 기준

레퍼런스에서 확인한 안정적인 readiness 조건:

- `nvidia.com/dynamo-component=Frontend` pod가 Ready
- worker component pods가 Ready
- Frontend `/v1/models`가 target model을 반환
- 짧은 `/v1/completions` probe가 성공

1차 구현에서는 범용성을 위해 다음 순서로 둔다. hard requirement는 1-3까지이며, 4-5는 benchmark config에서 모델명을 확인할 수 있을 때만 수행한다.

1. Frontend service 존재 확인
2. Frontend pod Ready 확인
3. worker pod Ready 확인
4. `/v1/models` probe, optional
5. 가능하면 lightweight inference probe, optional

모델명은 benchmark config의 `vllmArgs.model` 또는 manifest에서 추론하지 않고, 초기에는 benchmark config에서 읽는 방향이 안전하다. 모델명을 찾을 수 없으면 pod readiness와 service readiness까지만 확인하고 명시적 warning을 남긴다.

## Endpoint 반환

Dynamo Frontend service가 OpenAI-compatible endpoint 역할을 한다.

기본 endpoint 형태:

```text
http://<frontend-service>.<namespace>.svc.cluster.local:8000
```

Frontend service 이름은 CR 이름과 component 이름으로 생성되는 경우가 많다. 레퍼런스에서는 다음 이름을 사용한다.

```text
vllm-dynamo-frontend
```

초기 구현은 다음 순서로 service를 찾는다.

1. `nvidia.com/dynamo-component=Frontend` label이 있는 service
2. `<DynamoGraphDeployment metadata.name>-frontend`
3. 실패 시 명확한 에러

## Artifact 수집

최소 수집 대상:

- `kubectl get dynamographdeployment,dynamocomponentdeployment -o yaml`
- `kubectl get pods,svc,deploy,rs,events -n <namespace> -o wide`
- Frontend/worker pod logs
- Helm release status
- 적용한 engine manifest 사본
- readiness probe 결과

AIBrix deployer의 command log 패턴을 공통 helper로 추출해 각 `kubectl`/`helm` 호출의 stdout, stderr, exit code, timestamp를 남긴다. Dynamo deployer에서 AIBrix 전용 receiver에 묶인 helper를 복사하지 않는다.

## Teardown 정책

Dynamo CR은 finalizer 때문에 삭제가 막힐 수 있다. 레퍼런스처럼 teardown에서 finalizer 제거를 포함한다.

Teardown 순서:

1. benchmark driver가 만든 client pod는 driver가 정리한다.
2. DynamoGraphDeployment finalizer 제거
3. DynamoComponentDeployment finalizer 제거
4. Dynamo CR 삭제
5. non-platform deployment/service/replicaset/pod 삭제
6. Helm release uninstall
7. 필요 시 PVC/PV cleanup

PVC/PV 삭제는 위험할 수 있으므로 초기 구현에서는 brixbench가 생성한 PVC/PV만 삭제한다. 사용자 manifest가 만든 persistent resource는 owner label이 없으면 삭제하지 않는다.

## 코드 변경 계획

예상 변경 지점:

- `internal/resolver/resolver.go`
  - Scenario/Test schema, YAML parsing, `Resolve()` orchestration만 유지한다.
  - provider별 세부 정책은 직접 구현하지 않고 `validateProviderInputs()`만 호출한다.
- `internal/resolver/provider_inputs.go`
  - provider 이름별 입력 validation dispatcher를 둔다.
  - `aibrix`, `dynamo`, future `llmd`, explicit `provider: null`의 공통 진입점 역할을 한다.
  - provider 전용 파일에서 반환한 에러를 그대로 전달해 resolver core가 provider 세부 규칙을 몰라도 되게 한다.
- `internal/resolver/aibrix_provider.go`
  - AIBrix 입력 정책을 둔다.
  - `localPath`는 AIBrix에서만 허용한다.
  - `localPath`와 `version/commit` 조합 금지를 여기서 검증한다.
  - AIBrix source mode matrix를 명시적으로 검증한다. 현재 사용자 입력으로 허용하는 조합은 release `version`, source `commit`, staged `localPath` 중 하나로 제한한다.
  - AIBrix release/source resolution 자체는 기존 `workspace.go`, `artifact_resolution.go` 흐름을 유지한다.
- `internal/resolver/dynamo_provider.go`
  - Dynamo 입력 정책을 둔다.
  - Dynamo는 `version` 필수, `commit/localPath/controlplane` 금지로 검증한다.
  - `1.2.1` 입력을 `v1.2.1`로 normalize한다.
  - stable semver format validation을 담당한다.
- `internal/resolver/dynamo_provider_test.go`
  - Dynamo version normalize와 provider input policy를 네트워크 없이 검증한다.
- `internal/resolver/resolver_test.go`
  - YAML parsing부터 provider validation까지 이어지는 end-to-end resolver behavior만 남긴다.
  - provider별 세부 matrix 테스트는 provider별 test file로 점진적으로 옮긴다.
- `internal/deployers/deployer.go`
  - 1차 구현에서는 `Config.TestCase.Version`을 source of truth로 사용한다. `Config.Version`을 추가해 `TestCase.Version`과 중복 source of truth를 만들지 않는다.
  - 이후 deployer가 `resolver.Test`에 의존하지 않도록 `Config`를 정리할 때 provider-specific config struct 추가를 검토한다.
- `internal/deployers/dynamo.go`
  - placeholder를 release-based `DynamoDeployer`로 교체한다.
  - `Initialize()`에서 version, namespace, engine manifest, log dir를 저장한다.
  - `DeployControlPlane()`에서 Dynamo release checkout/chart path를 준비하고 Helm install을 수행한다.
  - `DeployGateway()`는 no-op으로 구현하되 lifecycle 호출이 성공하도록 유지한다.
  - `DeployEngine()`에서 user-provided `DynamoGraphDeployment` manifest를 그대로 apply한다.
  - cluster-specific 설정을 코드에서 생성하거나 보정하지 않는다. 필요한 값은 `engine.manifest` 또는 향후 명시적 values hook으로 전달한다.
  - `WaitForReady()`에서 Frontend/worker readiness를 필수 확인하고 OpenAI-compatible probe는 모델명을 확인할 수 있을 때 optional로 수행한다.
  - `GetGatewayEndpoint()`에서 Dynamo Frontend service URL을 반환한다.
  - `CaptureArtifacts()`와 `Teardown()`에서 Dynamo CR, generated workloads, logs, Helm release 상태를 다룬다.
- `internal/deployers/dynamo_release.go`
  - deploy 실행 단계의 Dynamo release source abstraction을 둔다.
  - `DynamoTagSource` interface와 `GitDynamoTagSource` production 구현을 둔다.
  - `DynamoDeployer` lifecycle code가 git command 세부사항을 알지 않도록 release tag 검증을 이 계약 뒤에 숨긴다.
  - `NewGitDynamoTagSource()`는 production runner와 upstream repo URL을 연결한다.
  - `ValidateDynamoReleaseTag(ctx, version)` public wrapper와 테스트 주입용 내부 helper를 둔다.
  - 우선 `git ls-remote --tags --refs https://github.com/ai-dynamo/dynamo.git refs/tags/<version>` 기반 remote tag existence 검증을 수행한다.
  - deployer 경계에서도 `^v[0-9]+\.[0-9]+\.[0-9]+$` stable tag format을 방어적으로 검증한다.
  - `ls-remote` output은 line/field 단위로 파싱해서 `refs/tags/<version>` exact match만 허용한다.
  - git command 실패 시 exit status뿐 아니라 trimmed command output도 error에 포함한다.
  - release checkout과 가능한 경우 main-reachable 검증은 같은 release 경계 안에서 별도 함수로 추가한다.
  - unit test는 fake tag source나 fake command runner를 주입해 네트워크 없이 검증한다.
- `internal/deployers/dynamo_commands.go`
  - Helm/kubectl command construction과 command logging helper를 둔다.
  - 실제 command 실행과 테스트 가능한 command builder를 분리한다.
- `internal/deployers/command_logs.go`
  - 현재 AIBrix receiver에 묶인 command log helper를 provider-neutral helper로 정리한다.
  - AIBrix와 Dynamo deployer가 같은 log format을 쓰되, provider-specific command builder는 각 deployer 파일에 둔다.
- `internal/deployers/dynamo_discovery.go`
  - Frontend service discovery, pod label discovery, component readiness helper를 둔다.
  - label 기반 탐색을 우선하고, 필요 시 CR name 기반 fallback을 둔다.
- `benchmark/runner_test.go`
  - `provider: dynamo`를 `NewDynamoDeployer()`에 연결한다.
  - `DeployControlPlane()`, `DeployGateway()`, `DeployEngine()`, `WaitForReady()` 순서로 lifecycle을 명시적으로 호출한다.
  - Dynamo deployer는 `deployers.Config.TestCase.Version`에서 resolver가 normalize한 version을 읽는다.
  - benchmark driver는 기존처럼 provider와 분리해 유지한다.
- `benchmark/testdata/scenarios/*.yaml`
  - `dynamo-hello-world.yaml`을 추가한다.
  - scenario에는 `provider: dynamo`, `version`, `engine.manifest`, `benchmark`만 넣는다.
  - `version: 1.2.1`이 resolver에서 `v1.2.1`로 normalize되는 경로는 resolver 테스트에서 검증한다.
- `benchmark/testdata/deployments/dynamo/*.yaml`
  - `qwen3-32b-round-robin-4p8d.yaml` fixture를 추가한다.
  - `/brixbench/.tmp/dynamo-reference/Routing_Test_dynamo_1.2.0-ds-v4-dev3_vllm_0.21.0/deploy_server_4p8d.yaml`을 거의 그대로 가져온다.
  - 필요한 변경은 AIBrix smoke와 비교하기 쉬운 고정값으로 제한한다: `Qwen3-32B`, `qwen3-32b`, `round-robin`, `MAX_MODEL_LEN=40960`, `GPU_MEM_UTIL=0.90`, `TP=4`.
  - topology, image, PVC, nodeSelector 등 클러스터별 값은 manifest 안에 두고 deployer가 생성하지 않는다.
- `internal/resolver/*_test.go`
  - provider input validation과 version normalization은 네트워크 없는 단위 테스트로 유지한다.
- `internal/deployers/*_test.go`
  - command builder, service discovery, readiness predicate를 Kubernetes cluster 없이 테스트한다.

## 1차 구현 범위

포함:

- [x] Dynamo provider 입력 validation
- [x] Dynamo version normalize
- stable release tag 정책
  - [x] remote release tag existence 검증 helper
  - [x] reference 기반 DynamoGraphDeployment fixture 추가
  - [ ] deployer lifecycle 연결
  - [ ] main reachable 검증
- [x] deployer 단계의 tag source abstraction
- release checkout 또는 chart path 준비
- Dynamo platform Helm install
- user-provided Dynamo CR apply
- Frontend service endpoint resolution
- Frontend/worker 중심 basic readiness wait
- 모델명을 확인할 수 있을 때만 OpenAI-compatible readiness probe
- artifact capture
- teardown
- unit tests

현재 진행상황:

- AIBrix hello-world regression은 통과했다. benchmark는 100/100 successful이었고 `brixbench-adhoc` namespace cleanup까지 완료됐다.
- Dynamo는 아직 live deploy 범위가 아니다. 현재 검증 범위는 resolver의 version normalize와 deployer의 remote release tag existence helper를 각각 독립적으로 검증하는 단계다.
- Dynamo deployment fixture는 live deploy를 보장하는 축약 예제가 아니라, 제공된 reference `deploy_server_4p8d.yaml` 기반의 qwen3-32b round-robin 4P8D fixture다.

제외:

- Dynamo CR 생성기
- runtime image 자동 생성/치환
- cluster-specific registry secret 생성
- operator image build/push
- RDMA/NIXL tuning 자동화
- benchmark matrix runner
- PodMonitor/VMP 자동 설정
- dev/rc/post tag 기본 허용

## 추후 확장 포인트

- `allowPrerelease: true` 또는 환경변수 기반 prerelease 허용
- Dynamo chart가 공식 OCI chart로 제공될 경우 checkout 없는 install 경로
- `platform.values` 또는 `platform.valuesFile` 같은 명시적 Helm values hook
- provider별 artifact capture plugin
- PodMonitor hook
- image mirror policy
- Dynamo CR schema-aware validation

## 결론

`provider: dynamo`는 AIBrix와 동일한 deployer lifecycle에 들어가되, 입력 표면은 `version`과 `engine.manifest`로 제한한다. Dynamo release 판정은 `ai-dynamo/dynamo`의 main-reachable tag를 기준으로 삼고, resolver는 네트워크 없는 입력 검증만 담당한다. 레퍼런스 배포 환경은 Dynamo 1.x의 실제 platform 설치, CR 적용, readiness, cleanup 순서를 확인하는 자료로 사용하되, 클러스터별 값과 benchmark matrix는 brixbench core에 넣지 않는다.
