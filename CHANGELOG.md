# Changelog

## [0.11.0-rc.1](https://github.com/GoogleCloudPlatform/prometheus-engine/compare/v0.10.0-rc.1...v0.11.0-rc.1) (2024-02-15)


### Features

* add secretless OAuth 2 support ([8194baa](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/8194baae3e30dcec6b88d66ae2cc361dcb33aeef))


### Bug Fixes

* **deps:** bump golang in /cmd/frontend ([ff3902a](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/ff3902aeddb41eab6ac36bc58bb577118dd0a56d))
* **deps:** bump golang in /cmd/operator ([ffc436b](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/ffc436b184ea7bc70ab3a4b73d8af437ffaddc90))
* **deps:** bump golang in /cmd/rule-evaluator ([9977fc2](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/9977fc2f5af68db164c559d45f1f17d6ea844235))
* **deps:** bump golang in /hack ([05e53de](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/05e53de9ceae91df95f28ed1d1912f2458f1f98a))
* **deps:** bump the go-deps group with 5 updates ([9fb575e](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/9fb575e2b0a7709194bd41a8934987d7bce3f489))
* **deps:** update k8s.io dependencies ([110c4fb](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/110c4fb3ed9bf5a692f747a3dd6ecd44b1931e20))
* limit size of kind cluster names ([3c74e4a](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/3c74e4a0c39c412c8871bb61a74b2a071c5437a2))
* make NodeMonitoring cluster-scoped ([e271548](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/e2715488d72b2733b73735db76c2aa0d091e7f14))
* match exact test on e2e test matrix ([982a814](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/982a8144b8e048b0545c0f8be801c230c0a14433))
* remove replica count for rule-evaluator ([2801179](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/2801179640b9410405290829bd3822672855a513))
* resolve enabling NodeMonitoring breaks target status ([a9cf882](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/a9cf8826042147b5f04a0edd649d67c986f85fb4))
* update self-pod-monitoring.yaml example to reduce ingestion of self-scraped metrics. ([#794](https://github.com/GoogleCloudPlatform/prometheus-engine/issues/794)) ([686fc10](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/686fc100079d5f9c9b89fba04b6c9b38986eb938))
* use operator namespace to check if a NodeMonitoring should be applied ([#797](https://github.com/GoogleCloudPlatform/prometheus-engine/issues/797)) ([bcafc67](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/bcafc67fad3de0b777d11b6d02678ae84ca56122))
