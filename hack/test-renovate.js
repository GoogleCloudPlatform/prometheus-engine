// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

const fs = require('fs');
const path = require('path');
const { createRequire } = require('module');

// Load Renovate internal modules from container environment
const renovateRequire = createRequire('/usr/local/renovate/package.json');
const json5 = renovateRequire('json5');
const { applyPackageRules } = renovateRequire('./dist/util/package-rules/index.js');

const configFile = path.resolve(__dirname, '../.github/renovate.json5');
const configRaw = fs.readFileSync(configFile, 'utf8');
const config = json5.parse(configRaw);

let totalTests = 0;
let passedTests = 0;
let failedTests = 0;

function assert(condition, message) {
  totalTests++;
  if (condition) {
    passedTests++;
    console.log(`  ✓ ${message}`);
  } else {
    failedTests++;
    console.error(`  ✗ FAIL: ${message}`);
  }
}

async function simulateDep(depName, opts = {}) {
  return await applyPackageRules({
    ...config,
    packageFile: opts.packageFile || 'go.mod',
    manager: opts.manager || 'gomod',
    packageName: depName,
    depName: depName,
    updateType: opts.updateType || 'minor',
    baseBranch: opts.baseBranch || 'main',
    ...opts,
  });
}

async function runTests() {
  console.log('Running Renovate Configuration Regression Test Suite...\n');

  // Suite 1: Prometheus Special Case
  console.log('Test Suite 1: Prometheus Repositories (GMP fork management)');
  assert(
    Array.isArray(config.ignoreDeps) &&
      config.ignoreDeps.includes('github.com/prometheus/prometheus'),
    'ignoreDeps contains github.com/prometheus/prometheus'
  );
  assert(
    Array.isArray(config.ignoreDeps) &&
      config.ignoreDeps.includes('github.com/GoogleCloudPlatform/prometheus'),
    'ignoreDeps contains github.com/GoogleCloudPlatform/prometheus'
  );

  const promCore = await simulateDep('github.com/prometheus/prometheus', {
    updateType: 'patch',
  });
  assert(
    promCore.automerge !== true,
    'github.com/prometheus/prometheus patch update is excluded from automerge'
  );

  const gmpProm = await simulateDep('github.com/GoogleCloudPlatform/prometheus', {
    updateType: 'patch',
  });
  assert(
    gmpProm.automerge !== true,
    'github.com/GoogleCloudPlatform/prometheus patch update is excluded from automerge'
  );

  const promClient = await simulateDep('github.com/prometheus/client_golang', {
    updateType: 'patch',
  });
  assert(
    promClient.automerge !== true,
    'Prometheus client libraries are excluded from automerge'
  );

  // Suite 2: Docker Dependencies
  console.log('\nTest Suite 2: Docker Dependencies');
  const dockerBases = [
    'golang:1.24',
    'gcr.io/distroless/static:latest',
    'alpine:3.20',
  ];
  for (const img of dockerBases) {
    const res = await simulateDep(img, {
      manager: 'dockerfile',
      packageFile: 'Dockerfile',
    });
    assert(
      res.groupName === 'docker' && res.groupSlug === 'docker',
      `Base image ${img} is grouped under 'docker'`
    );
    assert(
      res.automerge === true && res.automergeType === 'pr',
      `Base image ${img} has automerge enabled`
    );
    assert(
      res.enabled !== false,
      `Base image ${img} upgrades are enabled`
    );
  }

  const selfImages = [
    'gke.gcr.io/prometheus-engine/frontend',
    'gke-release/prometheus-engine/config-reloader',
    'gcr.io/gke-release/prometheus-engine/rule-evaluator',
  ];
  for (const img of selfImages) {
    const res = await simulateDep(img, {
      manager: 'dockerfile',
      packageFile: 'Dockerfile',
    });
    assert(
      res.enabled === false,
      `Self-referencing engine image ${img} is disabled (managed by make regen)`
    );
  }

  // Suite 3: OpenTelemetry Synchronization
  console.log('\nTest Suite 3: OpenTelemetry Module Synchronization');
  const otelModules = [
    'go.opentelemetry.io/otel',
    'go.opentelemetry.io/otel/sdk',
    'go.opentelemetry.io/otel/trace',
    'go.opentelemetry.io/otel/metric',
    'go.opentelemetry.io/otel/exporters/otlp/otlptrace',
    'go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc',
    'go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp',
  ];
  for (const mod of otelModules) {
    const res = await simulateDep(mod, { updateType: 'minor' });
    assert(
      res.groupName === 'opentelemetry' && res.groupSlug === 'opentelemetry',
      `${mod} resolves to groupName 'opentelemetry'`
    );
    assert(
      res.groupName !== 'deps',
      `${mod} is not overwritten by the generic 'deps' rule`
    );
  }

  // Suite 4: Kubernetes Dependencies
  console.log('\nTest Suite 4: Kubernetes Dependencies');
  const k8sModules = [
    'k8s.io/client-go',
    'k8s.io/api',
    'k8s.io/apimachinery',
    'sigs.k8s.io/controller-runtime',
  ];
  for (const mod of k8sModules) {
    const res = await simulateDep(mod, { updateType: 'minor' });
    assert(
      res.groupName === 'k8s-deps' && res.groupSlug === 'k8s-deps',
      `${mod} resolves to groupName 'k8s-deps'`
    );
    assert(
      !res.reviewers,
      `${mod} does not assign invalid reviewers`
    );
  }

  // Suite 5: General Go Dependencies & Automerge
  console.log('\nTest Suite 5: General Go Dependencies');
  const generalMinor = await simulateDep('github.com/google/go-cmp', {
    updateType: 'minor',
  });
  assert(
    generalMinor.groupName === 'deps' && generalMinor.groupSlug === 'deps',
    'General Go dependency resolves to groupName "deps"'
  );
  assert(
    generalMinor.automerge !== true,
    'General Go minor update is not automerged'
  );

  const generalPatch = await simulateDep('github.com/google/go-cmp', {
    updateType: 'patch',
  });
  assert(
    generalPatch.groupName === 'deps',
    'General Go patch dependency resolves to groupName "deps"'
  );
  assert(
    generalPatch.automerge === true,
    'General Go patch update is automerged'
  );

  // Suite 6: Auxiliary Go Modules
  console.log('\nTest Suite 6: Auxiliary Go Modules');
  const auxTools = await simulateDep('github.com/golangci/golangci-lint', {
    packageFile: 'tools/go.mod',
  });
  assert(
    auxTools.groupName === 'auxiliary-modules',
    'tools/go.mod module resolves to groupName "auxiliary-modules"'
  );

  const auxOps = await simulateDep('github.com/spf13/cobra', {
    packageFile: 'ops/gmpctl/go.mod',
  });
  assert(
    auxOps.groupName === 'auxiliary-modules',
    'ops/gmpctl module resolves to groupName "auxiliary-modules"'
  );

  // Suite 7: Maintenance Release Branch Policies
  console.log('\nTest Suite 7: Release Branch Policies (release/*)');
  const releaseBranch = 'release/0.19';

  const relRootMinor = await simulateDep('github.com/google/go-cmp', {
    baseBranch: releaseBranch,
    updateType: 'minor',
  });
  assert(
    relRootMinor.enabled === false,
    'Non-security update for root go.mod is disabled on release branches'
  );

  const relRootPatch = await simulateDep('github.com/google/go-cmp', {
    baseBranch: releaseBranch,
    updateType: 'patch',
  });
  assert(
    relRootPatch.enabled === false,
    'Non-security patch update for root go.mod is disabled on release branches'
  );

  const relRootVuln = await simulateDep('github.com/google/go-cmp', {
    baseBranch: releaseBranch,
    updateType: 'vulnerability',
  });
  assert(
    relRootVuln.enabled !== false,
    'Security/vulnerability update for root go.mod remains enabled on release branches'
  );

  const relToolsVuln = await simulateDep('github.com/efficientgo/tools', {
    packageFile: 'tools/go.mod',
    baseBranch: releaseBranch,
    updateType: 'vulnerability',
  });
  assert(
    relToolsVuln.enabled === false,
    'All updates (including security) for auxiliary modules are disabled on release branches'
  );

  const relDocker = await simulateDep('golang:1.24', {
    manager: 'dockerfile',
    packageFile: 'Dockerfile',
    baseBranch: releaseBranch,
  });
  assert(
    relDocker.enabled === false,
    'Dockerfile updates are disabled on release branches'
  );

  const relActions = await simulateDep('actions/checkout', {
    manager: 'github-actions',
    packageFile: '.github/workflows/presubmit.yml',
    baseBranch: releaseBranch,
  });
  assert(
    relActions.enabled === false,
    'GitHub Actions updates are disabled on release branches'
  );

  console.log(`\n========================================`);
  console.log(`Total: ${totalTests} | Passed: ${passedTests} | Failed: ${failedTests}`);
  console.log(`========================================\n`);

  if (failedTests > 0) {
    process.exit(1);
  }
}

runTests().catch((err) => {
  console.error('Unexpected error running tests:', err);
  process.exit(1);
});
