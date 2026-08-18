// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package migrate_test

import (
	"fmt"
	"os"
	"strings"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/migrate"
)

// ExampleMigrator_Run demonstrates migrating a ServiceMonitor with a backing Service.
// This example is tested automatically during 'go test'.
func ExampleMigrator_Run() {
	inputYAML := `
apiVersion: v1
kind: Service
metadata:
  name: frontend-svc
  namespace: web
  labels:
    app: frontend
spec:
  selector:
    app: frontend-pod
  ports:
  - name: web-metrics
    port: 80
    targetPort: 8080
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: frontend-monitor
  namespace: web
  labels:
    app: frontend
spec:
  selector:
    matchLabels:
      app: frontend
  endpoints:
  - port: web-metrics
    interval: 30s
    path: /metrics
`

	migrator := migrate.NewMigrator()
	migrator.Stdin = strings.NewReader(inputYAML)
	migrator.Stdout = os.Stdout
	migrator.Stderr = os.Stdout // Direct logs to stdout for testable example capture.
	migrator.RegisterConverter(&migrate.ServiceMonitorConverter{})

	report, err := migrator.Run("-")
	if err != nil {
		fmt.Printf("Migration failed: %v\n", err)
		return
	}

	if err := migrator.WriteOutputs(report.ReadyOutputs); err != nil {
		fmt.Printf("Failed to write outputs: %v\n", err)
		return
	}

	migrator.PrintSummary(report, false)

	// Output:
	// [INFO] [ServiceMonitor:web/frontend-monitor] Successfully decoded ServiceMonitor
	// [INFO] [ServiceMonitor:web/frontend-monitor] Stripped all metadata labels and annotations. Reconfigure them manually if needed
	// [SUCCESS] [ServiceMonitor:web/frontend-monitor] Converted successfully
	// apiVersion: monitoring.googleapis.com/v1
	// kind: PodMonitoring
	// metadata:
	//   creationTimestamp: null
	//   name: frontend-monitor
	//   namespace: web
	// spec:
	//   endpoints:
	//   - interval: 30s
	//     path: /metrics
	//     port: 8080
	//   selector:
	//     matchLabels:
	//       app: frontend-pod
	//   targetLabels:
	//     metadata: null
	// status:
	//   observedGeneration: 0
	//
	// =========================================
	// Migration Complete Summary:
	//   Successfully Migrated:      1
	//   Migrated with Warnings:     0
	//   Migrated with Action Items: 0
	//   Skipped (Unsupported):      0
	//   Failed:                     0
	// =========================================
}
