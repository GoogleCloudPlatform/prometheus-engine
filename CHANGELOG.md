# Changelog

## [0.12.0-rc.1](https://github.com/GoogleCloudPlatform/prometheus-engine/compare/v0.11.0-rc.1...v0.12.0-rc.1) (2024-03-28)


### Features

* export write example ([41115f5](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/41115f54a68e6816dc1420cdac89bd449e2a2497))
* implement secret management for authorization ([d79f537](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/d79f5371c6199d86cd9609332c8959340ed10854))
* **operator:** add OperatorConfig compression settings for *Rules CRs. ([981fbc4](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/981fbc46f3b6e8114692c960b1502611441450c8))
* **prometheus:** add secret management to export library ([e1106d1](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/e1106d11fc4d21dc8e73ede497da3e6201dc5a2e))
* **rule-evaluator:** add runtime status endpoint ([afd32e4](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/afd32e4b88dd45d209a08add8adca4031e7a20d7))
* **rule-evaluator:** add runtime status endpoint ([#891](https://github.com/GoogleCloudPlatform/prometheus-engine/issues/891)) ([94fb052](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/94fb05225f30460a1d2662b1fc757cf9531984e7))
* support decompressing config directories in config-reloader ([3c8e384](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/3c8e38417644df7126cd89e8431a73ad5ac39826))
* update common library for secret management ([66df62a](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/66df62ac4f64ad126719f0d0d496024fa02b7630))


### Bug Fixes

* clean code for consistent selector; add validation. ([#783](https://github.com/GoogleCloudPlatform/prometheus-engine/issues/783)) ([99444af](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/99444afa29b50fb6da764f95c7ffa6bdeab9233a))
* default secret namespace for cluster-scoped resources ([dd52a8a](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/dd52a8ab7c66fc18c798869be79562e6c413260f))
* **deps:** bump docker from 25.0-cli to 26.0-cli in /hack ([7434206](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/743420662dc1ef4f1c5d5d5b01a8676469226c6f))
* **deps:** bump google.golang.org/protobuf from 1.32.0 to 1.33.0 ([a4a4d32](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/a4a4d322bc6c6d408f74b2bf95d6baf440c28013))
* **deps:** bump k8s.io/apiextensions-apiserver from 0.29.1 to 0.29.3 ([297e6ac](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/297e6ac27b590812e21010f53de4339920ceeaa9))
* **deps:** bump k8s.io/client-go from 0.29.1 to 0.29.3 ([8b66373](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/8b663730045cd800ba56d63b13f443cc8b1ef433))
* **deps:** bump k8s.io/code-generator from 0.29.1 to 0.29.2 ([9ac2649](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/9ac26499c2f720ceee01966178d03038d28d2ed9))
* **deps:** bump sigs.k8s.io/controller-runtime from 0.16.3 to 0.17.2 ([c1c5ba7](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/c1c5ba7d520e8bf92e8d837b0e7da3020d5d393f))
* **e2e:** start rule-evaluator without auth in e2e tests ([67da42b](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/67da42bcc4e8bf4e8bb6da9438341cf4e2290cb7))
* fix config reloader panics if the response is nil ([08dc997](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/08dc9971057ba943199f83337be5d81b787a1331))
* **operator:** add duplicate job name validation within PM, CPM and CNM ([0413831](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/0413831bdc78976d29be2ad01d792b6f7533f94e))
* **operator:** add duplicate job name validation within PM, CPM and CNM ([#912](https://github.com/GoogleCloudPlatform/prometheus-engine/issues/912)) ([82a84f9](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/82a84f9607595c01e8362f4032fa234b3c27a7b5))
* rename kubernetesSecret to secret ([6576bb1](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/6576bb1165f23014621b24163ab88e68d28c8aea))
* replace nodemonitoring with clusternodemonitoring ([2177a0e](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/2177a0ec93feefd0c5b0de9935e69ad3eb2a614e))
* skip delete legacy webhook if we have no permissions ([75bf155](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/75bf1559cb82f423ede2ac98eec11a7353a339ca))
* update pkg/operator/apis/monitoring/v1/operator_types.go ([e15078e](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/e15078ebbe4cb2060a1977702942b5217be57122))
* update pkg/operator/apis/monitoring/v1/operator_types.go ([17e3603](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/17e3603cb6e01316689f4dafff16a3bafc21296a))
* update pkg/operator/apis/monitoring/v1/operator_types.go ([fe7f2a1](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/fe7f2a12d8191af2bf550dae4e80eef511f85cfb))
* update pkg/operator/apis/monitoring/v1/operator_types.go ([8fd4c5f](https://github.com/GoogleCloudPlatform/prometheus-engine/commit/8fd4c5fbce3da1a225e40b61f146b380ae20f91d))

## [0.11.0-rc.1](https://github.com/GoogleCloudPlatform/prometheus-engine/compare/v0.10.0-rc.1...v0.11.0-rc.1) (2024-02-16)


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
