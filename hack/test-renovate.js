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
const { Vulnerabilities } = renovateRequire('./dist/workers/repository/process/vulnerabilities.js');
const { applyVulnerabilityFixFilter } = renovateRequire('./dist/workers/repository/process/lookup/vulnerability.js');
const { classifyRelease } = renovateRequire('./dist/workers/repository/process/lookup/update-type.js');
const { get: getVersioning } = renovateRequire('./dist/modules/versioning/index.js');

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
      Array.isArray(res.addLabels) && res.addLabels.includes('automerge'),
      `Base image ${img} has automerge label`
    );
    assert(
      res.enabled !== false,
      `Base image ${img} upgrades are enabled`
    );
  }

  const actionsUpdate = await simulateDep('actions/checkout', {
    manager: 'github-actions',
    packageFile: '.github/workflows/presubmit.yml',
    updateType: 'patch',
  });
  assert(
    actionsUpdate.groupName === 'github-actions' && actionsUpdate.groupSlug === 'github-actions',
    'GitHub Actions update is grouped under "github-actions"'
  );
  assert(
    actionsUpdate.automerge === true && actionsUpdate.automergeType === 'pr',
    'GitHub Actions update has automerge enabled'
  );
  assert(
    Array.isArray(actionsUpdate.addLabels) && actionsUpdate.addLabels.includes('automerge'),
    'GitHub Actions update has automerge label'
  );

  const selfImages = [
    'gke.gcr.io/prometheus-engine/frontend',
    'gke-release/prometheus-engine/config-reloader',
    'gcr.io/gke-release/prometheus-engine/rule-evaluator',
  ];
  for (const img of selfImages) {
    for (const manager of ['dockerfile', 'custom.regex']) {
      const packageFile = manager === 'dockerfile' ? 'Dockerfile' : 'charts/values.global.yaml';

      // Lookup-phase evaluation (no updateType yet) resolves custom regex versioning
      const lookupRes = await applyPackageRules({
        ...config,
        packageFile,
        manager,
        packageName: img,
        depName: img,
        baseBranch: 'main',
      });
      assert(
        typeof lookupRes.versioning === 'string' && lookupRes.versioning.startsWith('regex:'),
        `Self-referencing engine image ${img} (${manager}) configures regex versioning`
      );

      for (const blockedType of ['major', 'minor']) {
        const blockedRes = await simulateDep(img, {
          manager,
          packageFile,
          updateType: blockedType,
        });
        assert(
          blockedRes.enabled === false,
          `Self-referencing engine image ${img} (${manager}) disables ${blockedType} updates`
        );
      }

      for (const allowedType of ['patch', 'digest']) {
        const allowedRes = await simulateDep(img, {
          manager,
          packageFile,
          updateType: allowedType,
        });
        assert(
          allowedRes.enabled !== false,
          `Self-referencing engine image ${img} (${manager}) enables ${allowedType} updates`
        );
        assert(
          allowedRes.groupName === 'docker' && allowedRes.automerge === true,
          `Self-referencing engine image ${img} (${manager}, ${allowedType}) is grouped under 'docker' with automerge`
        );
      }
    }
  }

  // Verify prometheus-engine regex versioning classifies patch, build (-gmp.N), and revision (-gke.N) updates as 'patch'
  const engineLookup = await applyPackageRules({
    ...config,
    packageFile: 'charts/values.global.yaml',
    manager: 'custom.regex',
    packageName: 'gke.gcr.io/prometheus-engine/operator',
    depName: 'gke.gcr.io/prometheus-engine/operator',
    baseBranch: 'main',
  });
  const engineVersioning = getVersioning(engineLookup.versioning);
  const patchBuildRevisionPairs = [
    ['v0.17.2-gke.0', 'v0.17.2-gke.2', 'revision bump (-gke.0 -> -gke.2)'],
    ['v0.17.2-gke.2', 'v0.17.3-gke.0', 'patch bump across -gke.N suffixes'],
    ['v0.27.0-gmp.4-gke.4', 'v0.27.0-gmp.5-gke.0', 'build bump (-gmp.4 -> -gmp.5)'],
    ['v0.27.0-gmp.5-gke.0', 'v0.27.0-gmp.5-gke.1', 'revision bump on -gmp.M-gke.N tag'],
    ['v2.53.5-gmp.1-gke.2', 'v2.53.5-gmp.2-gke.0', 'Prometheus build bump (-gmp.1 -> -gmp.2)'],
  ];
  for (const [fromVer, toVer, desc] of patchBuildRevisionPairs) {
    assert(
      engineVersioning.isValid(fromVer) &&
        engineVersioning.isValid(toVer) &&
        engineVersioning.isCompatible(toVer, fromVer) &&
        engineVersioning.isGreaterThan(toVer, fromVer) &&
        classifyRelease(engineVersioning, fromVer, toVer) === 'patch',
      `Engine versioning classifies ${fromVer} -> ${toVer} (${desc}) as compatible 'patch'`
    );
  }
  assert(
    classifyRelease(engineVersioning, 'v0.17.3-gke.0', 'v0.18.1-gke.0') === 'minor',
    'Engine versioning classifies v0.17.3-gke.0 -> v0.18.1-gke.0 as "minor"'
  );
  assert(
    classifyRelease(engineVersioning, 'v2.53.5-gmp.2-gke.0', 'v3.13.0-gmp.1-gke.0') === 'major',
    'Engine versioning classifies v2.53.5-gmp.2-gke.0 -> v3.13.0-gmp.1-gke.0 as "major"'
  );

  // Verify 1P Google container images bypass minimumReleaseAge while 3P images keep '7 days'
  const firstPartyImages = [
    'google-go.pkg.dev/golang',
    'gke.gcr.io/gke-distroless/libc',
    'gke.gcr.io/gke-distroless/bash',
    'gke.gcr.io/prometheus-engine/operator',
    'gcr.io/distroless/static',
    'gcr.io/gke-release/prometheus-engine/datasource-syncer',
    'us-central1-docker.pkg.dev/serverless-runtimes/google-24-full/runtimes/nodejs24',
  ];
  for (const img of firstPartyImages) {
    for (const manager of ['dockerfile', 'custom.regex']) {
      const res = await simulateDep(img, {
        manager,
        packageFile: manager === 'dockerfile' ? 'Dockerfile' : 'charts/values.global.yaml',
        updateType: 'digest',
      });
      assert(
        res.minimumReleaseAge === null,
        `1P Google image ${img} (${manager}) sets minimumReleaseAge to null`
      );
    }
  }

  const thirdPartyImages = [
    'varnish',
    'docker',
    'debian',
    'docker.io/node',
    'quay.io/prometheus/alertmanager',
    'gcr.io/other-project/app',
    'us-central1-docker.pkg.dev/other-project/repo/app',
  ];
  for (const img of thirdPartyImages) {
    for (const manager of ['dockerfile', 'custom.regex']) {
      const res = await simulateDep(img, {
        manager,
        packageFile: manager === 'dockerfile' ? 'Dockerfile' : 'manifests/operator.yaml',
        updateType: 'digest',
      });
      assert(
        res.minimumReleaseAge === '7 days',
        `3P image ${img} (${manager}) retains 7-day minimumReleaseAge`
      );
    }
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
  assert(
    !generalMinor.addLabels || !generalMinor.addLabels.includes('automerge'),
    'General Go minor update does not have automerge label'
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
    Array.isArray(generalPatch.addLabels) && generalPatch.addLabels.includes('automerge'),
    'General Go patch update has automerge label'
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
  const sampleReleaseBranches = [
    'release/0.14',
    'release/0.25',
    'release/1.0',
    'release/future-branch',
  ];

  for (const relBranch of sampleReleaseBranches) {
    const relRootMinor = await simulateDep('github.com/google/go-cmp', {
      baseBranch: relBranch,
      updateType: 'minor',
    });
    assert(
      relRootMinor.enabled === false,
      `Non-security minor update for root go.mod is disabled on ${relBranch}`
    );

    const relRootPatch = await simulateDep('github.com/google/go-cmp', {
      baseBranch: relBranch,
      updateType: 'patch',
    });
    assert(
      relRootPatch.enabled === false,
      `Non-security patch update for root go.mod is disabled on ${relBranch}`
    );

    const relRootVuln = await simulateDep('github.com/google/go-cmp', {
      baseBranch: relBranch,
      updateType: 'patch',
      force: { ...config.vulnerabilityAlerts },
      isVulnerabilityAlert: true,
    });
    assert(
      relRootVuln.enabled === true,
      `Security/vulnerability update for root go.mod remains enabled on ${relBranch}`
    );
    assert(
      relRootVuln.vulnerabilityFixStrategy === 'lowest',
      `vulnerabilityFixStrategy is "lowest" on ${relBranch}`
    );
    assert(
      relRootVuln.groupName === 'security-fixes',
      `Root go.mod vulnerability update resolves to groupName "security-fixes" on ${relBranch}`
    );

    const relToolsRoutine = await simulateDep('oras.land/oras-go/v2', {
      packageFile: 'tools/go.mod',
      baseBranch: relBranch,
      updateType: 'minor',
    });
    assert(
      relToolsRoutine.enabled === false,
      `Routine updates for auxiliary modules are disabled on ${relBranch}`
    );

    const relToolsVuln = await simulateDep('oras.land/oras-go/v2', {
      packageFile: 'tools/go.mod',
      baseBranch: relBranch,
      updateType: 'patch',
      force: { ...config.vulnerabilityAlerts },
      isVulnerabilityAlert: true,
    });
    assert(
      relToolsVuln.enabled === true,
      `Security/vulnerability updates for auxiliary modules remain enabled on ${relBranch}`
    );
    assert(
      relToolsVuln.vulnerabilityFixStrategy === 'lowest',
      `vulnerabilityFixStrategy is "lowest" for auxiliary modules on ${relBranch}`
    );
    assert(
      relToolsVuln.groupName === 'security-fixes',
      `Auxiliary module vulnerability update resolves to groupName "security-fixes" on ${relBranch}`
    );

    // Docker updates on release branches
    const relDockerPatch = await simulateDep('golang:1.24', {
      manager: 'dockerfile',
      packageFile: 'Dockerfile',
      baseBranch: relBranch,
      updateType: 'patch',
    });
    assert(
      relDockerPatch.enabled !== false,
      `Dockerfile patch updates are enabled on ${relBranch}`
    );

    const relDockerDigest = await simulateDep('golang:1.24', {
      manager: 'dockerfile',
      packageFile: 'Dockerfile',
      baseBranch: relBranch,
      updateType: 'digest',
    });
    assert(
      relDockerDigest.enabled !== false,
      `Dockerfile digest updates are enabled on ${relBranch}`
    );

    const relDockerVuln = await simulateDep('golang:1.24', {
      manager: 'dockerfile',
      packageFile: 'Dockerfile',
      baseBranch: relBranch,
      updateType: 'patch',
      force: { ...config.vulnerabilityAlerts },
      isVulnerabilityAlert: true,
    });
    assert(
      relDockerVuln.enabled !== false,
      `Dockerfile vulnerability updates are enabled on ${relBranch}`
    );

    const relDockerMinor = await simulateDep('golang:1.24', {
      manager: 'dockerfile',
      packageFile: 'Dockerfile',
      baseBranch: relBranch,
      updateType: 'minor',
    });
    assert(
      relDockerMinor.enabled === false,
      `Dockerfile minor updates are disabled on ${relBranch}`
    );

    // GitHub Actions updates on release branches
    const relActionsPatch = await simulateDep('actions/checkout', {
      manager: 'github-actions',
      packageFile: '.github/workflows/presubmit.yml',
      baseBranch: relBranch,
      updateType: 'patch',
    });
    assert(
      relActionsPatch.enabled !== false,
      `GitHub Actions patch updates are enabled on ${relBranch}`
    );

    const relActionsVuln = await simulateDep('actions/checkout', {
      manager: 'github-actions',
      packageFile: '.github/workflows/presubmit.yml',
      baseBranch: relBranch,
      updateType: 'patch',
      force: { ...config.vulnerabilityAlerts },
      isVulnerabilityAlert: true,
    });
    assert(
      relActionsVuln.enabled !== false,
      `GitHub Actions vulnerability updates are enabled on ${relBranch}`
    );

    const relActionsMinor = await simulateDep('actions/checkout', {
      manager: 'github-actions',
      packageFile: '.github/workflows/presubmit.yml',
      baseBranch: relBranch,
      updateType: 'minor',
    });
    assert(
      relActionsMinor.enabled === false,
      `GitHub Actions minor updates are disabled on ${relBranch}`
    );

    // Grouping rules verification on release branches (matching main)
    assert(
      relRootPatch.groupName === 'deps',
      `General Go dependencies resolve to groupName "deps" on ${relBranch}`
    );

    const relK8sRoutine = await simulateDep('k8s.io/client-go', {
      baseBranch: relBranch,
      updateType: 'patch',
    });
    assert(
      relK8sRoutine.groupName === 'k8s-deps',
      `Kubernetes dependencies resolve to groupName "k8s-deps" on ${relBranch}`
    );

    const relOtelRoutine = await simulateDep('go.opentelemetry.io/otel', {
      baseBranch: relBranch,
      updateType: 'patch',
    });
    assert(
      relOtelRoutine.groupName === 'opentelemetry',
      `OpenTelemetry dependencies resolve to groupName "opentelemetry" on ${relBranch}`
    );

    assert(
      relToolsRoutine.groupName === 'auxiliary-modules',
      `tools/go.mod module resolves to groupName "auxiliary-modules" on ${relBranch}`
    );

    const relOpsRoutine = await simulateDep('github.com/spf13/cobra', {
      packageFile: 'ops/gmpctl/go.mod',
      baseBranch: relBranch,
      updateType: 'patch',
    });
    assert(
      relOpsRoutine.groupName === 'auxiliary-modules',
      `ops/gmpctl module resolves to groupName "auxiliary-modules" on ${relBranch}`
    );

    assert(
      relDockerPatch.groupName === 'docker',
      `Dockerfile patch updates resolve to groupName "docker" on ${relBranch}`
    );
    assert(
      relDockerVuln.groupName === 'security-fixes',
      `Dockerfile vulnerability updates resolve to groupName "security-fixes" on ${relBranch}`
    );

    assert(
      relActionsPatch.groupName === 'github-actions',
      `GitHub Actions patch updates resolve to groupName "github-actions" on ${relBranch}`
    );
    assert(
      relActionsVuln.groupName === 'security-fixes',
      `GitHub Actions vulnerability updates resolve to groupName "security-fixes" on ${relBranch}`
    );
  }

  const mainRootVuln = await simulateDep('github.com/google/go-cmp', {
    baseBranch: 'main',
    updateType: 'patch',
    force: { ...config.vulnerabilityAlerts },
    isVulnerabilityAlert: true,
  });
  assert(
    mainRootVuln.vulnerabilityFixStrategy === 'lowest',
    'vulnerabilityFixStrategy is "lowest" globally on main and release branches'
  );

  // Suite 8: Indirect Go Module Dependencies
  console.log('\nTest Suite 8: Indirect Go Module Dependencies');
  const hasInvalidVulnUpdateType = (config.packageRules || []).some(
    (r) => Array.isArray(r.matchUpdateTypes) && r.matchUpdateTypes.includes('vulnerability')
  );
  assert(
    !hasInvalidVulnUpdateType,
    'packageRules do not use invalid matchUpdateTypes: ["vulnerability"]'
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

  // Renovate vulnerabilityAlerts worker overrides enabled: false by injecting force: { ...config.vulnerabilityAlerts }
  const indirectVulnSimulated = await simulateDep('go.mongodb.org/mongo-driver', {
    manager: 'gomod',
    packageFile: 'go.mod',
    depType: 'indirect',
    updateType: 'patch',
    force: { ...config.vulnerabilityAlerts },
  });
  assert(
    indirectVulnSimulated.enabled === true,
    'Vulnerability alerts override disabled indirect updates via force: { enabled: true }'
  );

  // Suite 9: Repository and Base Branch Configuration
  console.log('\nTest Suite 9: Repository Configuration & Base Branch Patterns');
  assert(
    Array.isArray(config.ignorePaths) && config.ignorePaths.includes('**/vendor/**'),
    'ignorePaths excludes vendor directory from package scanning'
  );

  assert(
    config.osvVulnerabilityAlerts === true,
    'osvVulnerabilityAlerts is enabled for cross-branch vulnerability tracking'
  );

  function matchesBaseBranch(branchName) {
    return (config.baseBranchPatterns || []).some((pattern) => {
      if (pattern === branchName) return true;
      if (typeof pattern === 'string' && pattern.startsWith('/') && pattern.endsWith('/')) {
        return new RegExp(pattern.slice(1, -1)).test(branchName);
      }
      return false;
    });
  }

  const testReleaseBranches = [
    'release/0.14',
    'release/0.25',
    'release/1.0',
    'release/2026.01',
    'release/v2.0',
  ];
  for (const b of testReleaseBranches) {
    assert(
      matchesBaseBranch(b),
      `baseBranchPatterns dynamically matches release branch format ${b}`
    );
  }

  const testNonReleaseBranches = [
    'archive/0.13',
    'archive/0.16',
    'feature/test',
    'hotfix/123',
    'pr/456',
  ];
  for (const b of testNonReleaseBranches) {
    assert(
      !matchesBaseBranch(b),
      `baseBranchPatterns rejects non-release branch ${b}`
    );
  }

  // Suite 10: Custom Managers for Chart & Manifest Images
  console.log('\nTest Suite 10: Custom Managers for Chart & Manifest Images');
  assert(
    Array.isArray(config.customManagers) && config.customManagers.length > 0,
    'customManagers is configured'
  );

  if (Array.isArray(config.customManagers) && config.customManagers.length > 0) {
    const { extractPackageFile } = renovateRequire('./dist/modules/manager/custom/regex/index.js');
    const { matchRegexOrGlob } = renovateRequire('./dist/util/string-match.js');
    const customManager = config.customManagers[0];

    assert(
      Array.isArray(customManager.managerFilePatterns) &&
        customManager.managerFilePatterns.length > 0 &&
        customManager.fileMatch === undefined,
      'customManager uses managerFilePatterns instead of deprecated fileMatch'
    );

    const expectedMatchedFiles = [
      'charts/values.global.yaml',
      'manifests/operator.yaml',
      'cmd/datasource-syncer/datasource-syncer.yaml',
    ];
    for (const file of expectedMatchedFiles) {
      const matched = customManager.managerFilePatterns.some((pattern) =>
        matchRegexOrGlob(file, pattern)
      );
      assert(matched, `managerFilePatterns matches ${file}`);
    }

    const expectedUnmatchedFiles = [
      'charts/Chart.yaml',
      'README.md',
    ];
    for (const file of expectedUnmatchedFiles) {
      const matched = customManager.managerFilePatterns.some((pattern) =>
        matchRegexOrGlob(file, pattern)
      );
      assert(!matched, `managerFilePatterns does not match ${file}`);
    }

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

    // Unquoted image tag test for charts
    const unquotedChart = "image: gke.gcr.io/gke-distroless/bash\n    tag: gke_distroless_20260815.00_p0";
    const unquotedChartRes = extractPackageFile(unquotedChart, 'charts/values.global.yaml', customManager);
    assert(
      unquotedChartRes &&
        unquotedChartRes.deps.length === 1 &&
        unquotedChartRes.deps[0].currentValue === 'gke_distroless_20260815.00_p0',
      'customManager extracts unquoted tag from chart format'
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

    // Generic registry image test for manifests
    const genericManifest = "image: 'quay.io/prometheus/alertmanager:v0.28.0'";
    const genericManifestRes = extractPackageFile(genericManifest, 'manifests/operator.yaml', customManager);
    assert(
      genericManifestRes &&
        genericManifestRes.deps.length === 1 &&
        genericManifestRes.deps[0].depName === 'quay.io/prometheus/alertmanager' &&
        genericManifestRes.deps[0].currentValue === 'v0.28.0',
      'customManager extracts generic registry image from manifest format'
    );
  }

  // Suite 11: End-to-End Vulnerability Alert Engine Integration
  console.log('\nTest Suite 11: End-to-End Vulnerability Alert Engine Integration');
  const vulnHelper = new Vulnerabilities();
  const semverVersioning = getVersioning('semver');

  // 11.1: Root go.mod vulnerability alert on release branch using real config.vulnerabilityAlerts
  const rootVulnRule = vulnHelper.vulnerabilityToPackageRules({
    vulnerability: { id: 'GHSA-test-root', severity: [{ type: 'CVSS_V3', score: '9.8' }] },
    affected: {},
    packageName: 'github.com/google/go-cmp',
    depVersion: '0.5.9',
    fixedVersion: '>= 0.6.0',
    datasource: 'go',
    packageFileConfig: {
      vulnerabilityAlerts: config.vulnerabilityAlerts,
    },
  });
  assert(
    rootVulnRule && rootVulnRule.isVulnerabilityAlert === true && rootVulnRule.force?.enabled === true,
    'vulnerabilityToPackageRules generates package rule with isVulnerabilityAlert and force: { enabled: true }'
  );

  const rootRoutineRel = await applyPackageRules({
    ...config,
    depName: 'github.com/google/go-cmp',
    packageName: 'github.com/google/go-cmp',
    currentValue: '0.5.9',
    manager: 'gomod',
    packageFile: 'go.mod',
    baseBranch: 'release/0.14',
    updateType: 'patch',
    datasource: 'go',
  });
  assert(
    rootRoutineRel.enabled === false,
    'Routine patch update on release branch evaluates to enabled: false without vulnerability rule'
  );

  const rootVulnRel = await applyPackageRules({
    ...config,
    depName: 'github.com/google/go-cmp',
    packageName: 'github.com/google/go-cmp',
    currentValue: '0.5.9',
    manager: 'gomod',
    packageFile: 'go.mod',
    baseBranch: 'release/0.14',
    updateType: 'patch',
    datasource: 'go',
    packageRules: [...config.packageRules, rootVulnRule],
  });
  assert(
    rootVulnRel.enabled === true,
    'Vulnerability alert overrides enabled: false on release branch via engine package rule'
  );
  assert(
    rootVulnRel.groupName === 'security-fixes' && rootVulnRel.groupSlug === 'security-fixes',
    'Root go.mod vulnerability alert resolves to groupName "security-fixes" on release branch'
  );
  assert(
    rootVulnRel.vulnerabilityFixStrategy === 'lowest',
    'Root go.mod vulnerability alert sets top-level vulnerabilityFixStrategy to "lowest" on release branch'
  );

  // 11.2: Auxiliary Go module vulnerability alert on release branch using real config.vulnerabilityAlerts
  const auxVulnRule = vulnHelper.vulnerabilityToPackageRules({
    vulnerability: { id: 'GHSA-test-aux', severity: [{ type: 'CVSS_V3', score: '9.8' }] },
    affected: {},
    packageName: 'oras.land/oras-go/v2',
    depVersion: '2.3.0',
    fixedVersion: '>= 2.5.0',
    datasource: 'go',
    packageFileConfig: {
      vulnerabilityAlerts: config.vulnerabilityAlerts,
    },
  });

  const auxRoutineRel = await applyPackageRules({
    ...config,
    depName: 'oras.land/oras-go/v2',
    packageName: 'oras.land/oras-go/v2',
    currentValue: '2.3.0',
    manager: 'gomod',
    packageFile: 'tools/go.mod',
    baseBranch: 'release/0.14',
    updateType: 'patch',
    datasource: 'go',
  });
  assert(
    auxRoutineRel.enabled === false,
    'Routine patch update for auxiliary module on release branch evaluates to enabled: false'
  );

  const auxVulnRel = await applyPackageRules({
    ...config,
    depName: 'oras.land/oras-go/v2',
    packageName: 'oras.land/oras-go/v2',
    currentValue: '2.3.0',
    manager: 'gomod',
    packageFile: 'tools/go.mod',
    baseBranch: 'release/0.14',
    updateType: 'patch',
    datasource: 'go',
    packageRules: [...config.packageRules, auxVulnRule],
  });
  assert(
    auxVulnRel.enabled === true,
    'Auxiliary module vulnerability alert overrides enabled: false on release branch'
  );
  assert(
    auxVulnRel.groupName === 'security-fixes' && auxVulnRel.groupSlug === 'security-fixes',
    'Auxiliary module vulnerability alert resolves to groupName "security-fixes"'
  );
  assert(
    auxVulnRel.vulnerabilityFixStrategy === 'lowest',
    'Auxiliary module vulnerability alert sets top-level vulnerabilityFixStrategy to "lowest" on release branch'
  );

  const auxCandidateReleases = [{ version: '2.5.0' }, { version: '2.6.0' }];
  const auxFixFilterRes = applyVulnerabilityFixFilter(auxVulnRel, {}, semverVersioning, auxCandidateReleases);
  assert(
    auxFixFilterRes.shrinkedViaVulnerability === true &&
      auxFixFilterRes.releases.length === 1 &&
      auxFixFilterRes.releases[0].version === '2.5.0',
    'applyVulnerabilityFixFilter selects the lowest fixed version (2.5.0) for auxiliary module vulnerability'
  );

  // 11.3: Indirect dependency vulnerability alert
  const indirectVulnRule = vulnHelper.vulnerabilityToPackageRules({
    vulnerability: { id: 'GHSA-test-indirect', severity: [{ type: 'CVSS_V3', score: '7.5' }] },
    affected: {},
    packageName: 'go.mongodb.org/mongo-driver',
    depType: 'indirect',
    depVersion: '1.11.0',
    fixedVersion: '>= 1.11.8',
    datasource: 'go',
    packageFileConfig: {
      vulnerabilityAlerts: config.vulnerabilityAlerts,
    },
  });

  const indirectVulnApplied = await applyPackageRules({
    ...config,
    depName: 'go.mongodb.org/mongo-driver',
    packageName: 'go.mongodb.org/mongo-driver',
    depType: 'indirect',
    currentValue: '1.11.0',
    manager: 'gomod',
    packageFile: 'go.mod',
    baseBranch: 'main',
    updateType: 'patch',
    datasource: 'go',
    packageRules: [...config.packageRules, indirectVulnRule],
  });
  assert(
    indirectVulnApplied.enabled === true,
    'Indirect dependency vulnerability alert overrides enabled: false via engine package rule'
  );

  // 11.4: Main branch vulnerability fix strategy
  const mainVulnRule = vulnHelper.vulnerabilityToPackageRules({
    vulnerability: { id: 'GHSA-test-main', severity: [] },
    affected: {},
    packageName: 'github.com/google/go-cmp',
    depVersion: '0.5.9',
    fixedVersion: '>= 0.6.0',
    datasource: 'go',
    packageFileConfig: {
      vulnerabilityAlerts: config.vulnerabilityAlerts,
    },
  });
  const mainVulnApplied = await applyPackageRules({
    ...config,
    depName: 'github.com/google/go-cmp',
    packageName: 'github.com/google/go-cmp',
    currentValue: '0.5.9',
    manager: 'gomod',
    packageFile: 'go.mod',
    baseBranch: 'main',
    updateType: 'patch',
    datasource: 'go',
    packageRules: [...config.packageRules, mainVulnRule],
  });
  assert(
    mainVulnApplied.vulnerabilityFixStrategy === 'lowest' && mainVulnApplied.groupName === 'security-fixes',
    'Vulnerability alerts use "lowest" fix strategy and "security-fixes" group on main'
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
