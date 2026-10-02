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
  assert(
    config.commitMessageTopic === '{{depName}}',
    'commitMessageTopic uses {{depName}}'
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
  assert(
    relRootVuln.vulnerabilityAlerts?.vulnerabilityFixStrategy === 'lowest',
    'vulnerabilityFixStrategy is "lowest" on release branches'
  );

  const mainRootVuln = await simulateDep('github.com/google/go-cmp', {
    baseBranch: 'main',
    updateType: 'vulnerability',
  });
  assert(
    mainRootVuln.vulnerabilityAlerts?.vulnerabilityFixStrategy === 'highest',
    'vulnerabilityFixStrategy defaults to "highest" on main'
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

  // Docker updates on release branches
  const relDockerPatch = await simulateDep('golang:1.24', {
    manager: 'dockerfile',
    packageFile: 'Dockerfile',
    baseBranch: releaseBranch,
    updateType: 'patch',
  });
  assert(
    relDockerPatch.enabled !== false,
    'Dockerfile patch updates are enabled on release branches'
  );

  const relDockerDigest = await simulateDep('golang:1.24', {
    manager: 'dockerfile',
    packageFile: 'Dockerfile',
    baseBranch: releaseBranch,
    updateType: 'digest',
  });
  assert(
    relDockerDigest.enabled !== false,
    'Dockerfile digest updates are enabled on release branches'
  );

  const relDockerVuln = await simulateDep('golang:1.24', {
    manager: 'dockerfile',
    packageFile: 'Dockerfile',
    baseBranch: releaseBranch,
    updateType: 'vulnerability',
  });
  assert(
    relDockerVuln.enabled !== false,
    'Dockerfile vulnerability updates are enabled on release branches'
  );

  const relDockerMinor = await simulateDep('golang:1.24', {
    manager: 'dockerfile',
    packageFile: 'Dockerfile',
    baseBranch: releaseBranch,
    updateType: 'minor',
  });
  assert(
    relDockerMinor.enabled === false,
    'Dockerfile minor updates are disabled on release branches'
  );

  // GitHub Actions updates on release branches
  const relActionsPatch = await simulateDep('actions/checkout', {
    manager: 'github-actions',
    packageFile: '.github/workflows/presubmit.yml',
    baseBranch: releaseBranch,
    updateType: 'patch',
  });
  assert(
    relActionsPatch.enabled !== false,
    'GitHub Actions patch updates are enabled on release branches'
  );

  const relActionsVuln = await simulateDep('actions/checkout', {
    manager: 'github-actions',
    packageFile: '.github/workflows/presubmit.yml',
    baseBranch: releaseBranch,
    updateType: 'vulnerability',
  });
  assert(
    relActionsVuln.enabled !== false,
    'GitHub Actions vulnerability updates are enabled on release branches'
  );

  const relActionsMinor = await simulateDep('actions/checkout', {
    manager: 'github-actions',
    packageFile: '.github/workflows/presubmit.yml',
    baseBranch: releaseBranch,
    updateType: 'minor',
  });
  assert(
    relActionsMinor.enabled === false,
    'GitHub Actions minor updates are disabled on release branches'
  );

  // Suite 8: Indirect Go Module Dependencies
  console.log('\nTest Suite 8: Indirect Go Module Dependencies');
  const indirectVuln = await simulateDep('go.mongodb.org/mongo-driver', {
    manager: 'gomod',
    packageFile: 'go.mod',
    depType: 'indirect',
    updateType: 'vulnerability',
  });
  assert(
    indirectVuln.enabled !== false,
    'Indirect Go module vulnerability updates are enabled'
  );

  const indirectPatch = await simulateDep('go.mongodb.org/mongo-driver', {
    manager: 'gomod',
    packageFile: 'go.mod',
    depType: 'indirect',
    updateType: 'patch',
  });
  assert(
    indirectPatch.enabled === false,
    'Indirect Go module routine patch updates are disabled'
  );

  const indirectMinor = await simulateDep('go.mongodb.org/mongo-driver', {
    manager: 'gomod',
    packageFile: 'go.mod',
    depType: 'indirect',
    updateType: 'minor',
  });
  assert(
    indirectMinor.enabled === false,
    'Indirect Go module routine minor updates are disabled'
  );

  // Suite 9: Repository and Base Branch Configuration
  console.log('\nTest Suite 9: Repository Configuration & Base Branch Patterns');
  assert(
    Array.isArray(config.ignorePaths) && !config.ignorePaths.includes('**/vendor/**'),
    'ignorePaths does not exclude vendor directory'
  );

  const hasDynamicReleasePattern = Array.isArray(config.baseBranchPatterns) &&
    config.baseBranchPatterns.includes('main') &&
    config.baseBranchPatterns.some(
      (p) => typeof p === 'string' && p.startsWith('/') && new RegExp(p.slice(1, -1)).test('release/0.14')
    );
  assert(
    hasDynamicReleasePattern,
    'baseBranchPatterns dynamically matches release/0.14'
  );

  const archiveMatches = (config.baseBranchPatterns || []).some(
    (p) => typeof p === 'string' && (p === 'archive/0.13' || (p.startsWith('/') && new RegExp(p.slice(1, -1)).test('archive/0.13')))
  );
  assert(
    !archiveMatches,
    'baseBranchPatterns rejects archive/* branches'
  );

  // Suite 10: Custom Managers for Chart & Manifest Images
  console.log('\nTest Suite 10: Custom Managers for Chart & Manifest Images');
  assert(
    Array.isArray(config.customManagers) && config.customManagers.length > 0,
    'customManagers is configured'
  );

  if (Array.isArray(config.customManagers) && config.customManagers.length > 0) {
    const { extractPackageFile } = renovateRequire('./dist/modules/manager/custom/regex/index.js');
    const customManager = config.customManagers[0];

    const chartContent = fs.readFileSync(path.resolve(__dirname, '../charts/values.global.yaml'), 'utf8');
    const chartRes = extractPackageFile(chartContent, 'charts/values.global.yaml', customManager);

    assert(
      chartRes && Array.isArray(chartRes.deps) && chartRes.deps.length === 7,
      'customManager extracts all 7 images from charts/values.global.yaml'
    );

    const expectedImages = [
      'gke.gcr.io/gke-distroless/bash',
      'gke.gcr.io/prometheus-engine/alertmanager',
      'gke.gcr.io/prometheus-engine/prometheus',
      'gke.gcr.io/prometheus-engine/config-reloader',
      'gke.gcr.io/prometheus-engine/operator',
      'gke.gcr.io/prometheus-engine/rule-evaluator',
      'gke.gcr.io/prometheus-engine/datasource-syncer',
    ];

    const extractedChartNames = (chartRes?.deps || []).map((d) => d.depName);
    for (const img of expectedImages) {
      assert(
        extractedChartNames.includes(img),
        `customManager extracts ${img} from charts/values.global.yaml`
      );
    }

    const manifestContent = fs.readFileSync(path.resolve(__dirname, '../manifests/operator.yaml'), 'utf8');
    const manifestRes = extractPackageFile(manifestContent, 'manifests/operator.yaml', customManager);
    assert(
      manifestRes && Array.isArray(manifestRes.deps) && manifestRes.deps.length > 0,
      'customManager extracts matching images from manifests/operator.yaml'
    );

    // Single-quoted image tag tests for charts and manifests
    const singleQuotedChart = "image: 'gke.gcr.io/gke-distroless/bash'\n    tag: 'gke_distroless_20260815.00_p0'";
    const singleQuotedChartRes = extractPackageFile(singleQuotedChart, 'charts/values.global.yaml', customManager);
    assert(
      singleQuotedChartRes &&
        singleQuotedChartRes.deps.length === 1 &&
        singleQuotedChartRes.deps[0].currentValue === 'gke_distroless_20260815.00_p0' &&
        !singleQuotedChartRes.deps[0].currentValue.endsWith("'"),
      'customManager does not include trailing single quote in currentValue for chart format'
    );

    const singleQuotedManifest = "image: 'gke.gcr.io/gke-distroless/bash:gke_distroless_20260815.00_p0'";
    const singleQuotedManifestRes = extractPackageFile(singleQuotedManifest, 'manifests/operator.yaml', customManager);
    assert(
      singleQuotedManifestRes &&
        singleQuotedManifestRes.deps.length === 1 &&
        singleQuotedManifestRes.deps[0].currentValue === 'gke_distroless_20260815.00_p0' &&
        !singleQuotedManifestRes.deps[0].currentValue.endsWith("'"),
      'customManager does not include trailing single quote in currentValue for manifest format'
    );
  }

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
