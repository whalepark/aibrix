/*
Copyright 2026 The Aibrix Team.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resolver

import (
	"context"
	"testing"
)

func TestPrepareGatewayImageUsesBenchmarkGatewayImageEnv(t *testing.T) {
	const image = "aibrix-public-release-cn-beijing.cr.volces.com/aibrix/gateway-plugins:v0.6.0-52405d78-benchmark"
	t.Setenv("BENCHMARK_GATEWAY_IMAGE", image)

	provider := "aibrix"
	testCase := &Test{
		Name:     "aibrix-pd-env-override",
		Provider: &provider,
		Version:  "v0.6.0",
		// Intentionally leave WorkspacePath/ResolvedCommit empty: env override
		// must short-circuit before the docker build path.
	}

	got, err := PrepareGatewayImage(context.Background(), t.TempDir(), testCase)
	if err != nil {
		t.Fatalf("PrepareGatewayImage returned error: %v", err)
	}
	if got == nil {
		t.Fatal("expected gateway image, got nil")
	}
	if got.Image != image {
		t.Fatalf("Image = %q, want %q", got.Image, image)
	}
	if got.Repository != "aibrix-public-release-cn-beijing.cr.volces.com/aibrix/gateway-plugins" {
		t.Fatalf("Repository = %q", got.Repository)
	}
	if got.Tag != "v0.6.0-52405d78-benchmark" {
		t.Fatalf("Tag = %q", got.Tag)
	}
	if testCase.GatewayImage != image || testCase.GatewayImageRepository != got.Repository || testCase.GatewayImageTag != got.Tag {
		t.Fatalf("test case fields not updated: image=%q repo=%q tag=%q", testCase.GatewayImage, testCase.GatewayImageRepository, testCase.GatewayImageTag)
	}
}

func TestPrepareGatewayImageRejectsInvalidBenchmarkGatewayImageEnv(t *testing.T) {
	t.Setenv("BENCHMARK_GATEWAY_IMAGE", "not-a-valid-image-ref")
	provider := "aibrix"
	_, err := PrepareGatewayImage(context.Background(), t.TempDir(), &Test{
		Name:     "aibrix-pd-bad-env",
		Provider: &provider,
	})
	if err == nil {
		t.Fatal("expected error for invalid BENCHMARK_GATEWAY_IMAGE")
	}
}
