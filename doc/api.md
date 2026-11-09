# API Reference

## Packages
- [monitoring.googleapis.com/v1](#monitoringgoogleapiscomv1)


## monitoring.googleapis.com/v1

Package v1 is the v1 version of the API.

### Resource Types
- [ClusterNodeMonitoring](#clusternodemonitoring)
- [ClusterPodMonitoring](#clusterpodmonitoring)
- [ClusterRules](#clusterrules)
- [GlobalRules](#globalrules)
- [OperatorConfig](#operatorconfig)
- [PodMonitoring](#podmonitoring)
- [Rules](#rules)



#### AlertingSpec



AlertingSpec defines alerting configuration.



_Appears in:_
- [RuleEvaluatorSpec](#ruleevaluatorspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `alertmanagers` _[AlertmanagerEndpoints](#alertmanagerendpoints) array_ | Alertmanagers contains endpoint configuration for designated Alertmanagers. |  |  |


#### AlertmanagerEndpoints



AlertmanagerEndpoints defines a selection of a single Endpoints object
containing alertmanager IPs to fire alerts against.



_Appears in:_
- [AlertingSpec](#alertingspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `namespace` _string_ | Namespace of Endpoints object. |  |  |
| `name` _string_ | Name of Endpoints object in Namespace. |  |  |
| `port` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util)_ | Port the Alertmanager API is exposed on. |  |  |
| `scheme` _string_ | Scheme to use when firing alerts. |  |  |
| `pathPrefix` _string_ | Prefix for the HTTP path alerts are pushed to. |  |  |
| `tls` _[TLSConfig](#tlsconfig)_ | TLS Config to use for alertmanager connection. |  |  |
| `authorization` _[Authorization](#authorization)_ | Authorization section for this alertmanager endpoint. |  |  |
| `apiVersion` _string_ | Version of the Alertmanager API that rule-evaluator uses to send alerts. It<br />can be "v1" or "v2". |  |  |
| `timeout` _string_ | Timeout is a per-target Alertmanager timeout when pushing alerts. |  | Format: duration <br /> |


#### Auth



Auth sets the `Authorization` header on every HTTP request.



_Appears in:_
- [HTTPClientConfig](#httpclientconfig)
- [ScrapeEndpoint](#scrapeendpoint)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _string_ | Type is the authentication type. Defaults to Bearer.<br />Basic will cause an error, as the BasicAuth object should be used instead. |  | Optional: \{\} <br /> |
| `credentials` _[SecretSelector](#secretselector)_ | Credentials uses the secret as the credentials (token) for the authentication header. |  | Optional: \{\} <br /> |


#### Authorization



Authorization specifies a subset of the Authorization struct, that is
safe for use in Endpoints (no CredentialsFile field).



_Appears in:_
- [AlertmanagerEndpoints](#alertmanagerendpoints)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _string_ | Set the authentication type. Defaults to Bearer, Basic will cause an<br />error. |  |  |
| `credentials` _[SecretKeySelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#secretkeyselector-v1-core)_ | The secret's key that contains the credentials of the request |  |  |


#### BasicAuth



BasicAuth sets the `Authorization` header on every HTTP request with the configured username
and password.



_Appears in:_
- [HTTPClientConfig](#httpclientconfig)
- [ScrapeEndpoint](#scrapeendpoint)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `username` _string_ | Username is the BasicAuth username. |  | Optional: \{\} <br /> |
| `password` _[SecretSelector](#secretselector)_ | Password uses the secret as the BasicAuth password. |  | Optional: \{\} <br /> |


#### ClusterNodeMonitoring



ClusterNodeMonitoring defines monitoring for a set of nodes.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `monitoring.googleapis.com/v1` | | |
| `kind` _string_ | `ClusterNodeMonitoring` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ClusterNodeMonitoringSpec](#clusternodemonitoringspec)_ | Specification of desired node selection for target discovery by<br />Prometheus. |  |  |
| `status` _[MonitoringStatus](#monitoringstatus)_ | Most recently observed status of the resource. |  | Optional: \{\} <br /> |


#### ClusterNodeMonitoringSpec



ClusterNodeMonitoringSpec contains specification parameters for ClusterNodeMonitoring.



_Appears in:_
- [ClusterNodeMonitoring](#clusternodemonitoring)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `selector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#labelselector-v1-meta)_ | Label selector that specifies which nodes are selected for this monitoring<br />configuration. If left empty all nodes are selected. |  |  |
| `endpoints` _[ScrapeNodeEndpoint](#scrapenodeendpoint) array_ | The endpoints to scrape on the selected nodes. |  | MaxItems: 10 <br />MinItems: 1 <br /> |
| `limits` _[ScrapeLimits](#scrapelimits)_ | Limits to apply at scrape time. |  |  |


#### ClusterNodeTLS







_Appears in:_
- [ScrapeNodeEndpoint](#scrapenodeendpoint)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `insecureSkipVerify` _boolean_ | InsecureSkipVerify disables target certificate validation. |  | Optional: \{\} <br /> |


#### ClusterPodMonitoring



ClusterPodMonitoring defines monitoring for a set of pods, scoped to all
pods within the cluster.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `monitoring.googleapis.com/v1` | | |
| `kind` _string_ | `ClusterPodMonitoring` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ClusterPodMonitoringSpec](#clusterpodmonitoringspec)_ | Specification of desired Pod selection for target discovery by<br />Prometheus. |  |  |
| `status` _[PodMonitoringStatus](#podmonitoringstatus)_ | Most recently observed status of the resource. |  | Optional: \{\} <br /> |


#### ClusterPodMonitoringSpec



ClusterPodMonitoringSpec contains specification parameters for ClusterPodMonitoring.



_Appears in:_
- [ClusterPodMonitoring](#clusterpodmonitoring)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `selector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#labelselector-v1-meta)_ | Label selector that specifies which pods are selected for this monitoring<br />configuration. |  |  |
| `endpoints` _[ScrapeEndpoint](#scrapeendpoint) array_ | The endpoints to scrape on the selected pods. |  | MaxItems: 10 <br />MinItems: 1 <br /> |
| `targetLabels` _[ClusterTargetLabels](#clustertargetlabels)_ | Labels to add to the Prometheus target for discovered endpoints.<br />The `instance` label is always set to `<pod_name>:<port>` or `<node_name>:<port>`<br />if the scraped pod is controlled by a DaemonSet. | \{  \} | Optional: \{\} <br /> |
| `limits` _[ScrapeLimits](#scrapelimits)_ | Limits to apply at scrape time. |  |  |
| `filterRunning` _boolean_ | FilterRunning will drop any pods that are in the "Failed" or "Succeeded"<br />pod lifecycle.<br />See: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-phase<br />Specifically, this prevents scraping Succeeded pods from K8s jobs, which<br />could contribute to noisy logs or irrelevant metrics.<br />Additionally, it can mitigate issues with reusing stale target<br />labels in cases where Pod IPs are reused (e.g. spot containers).<br />See: https://github.com/GoogleCloudPlatform/prometheus-engine/issues/145 |  |  |


#### ClusterRules



ClusterRules defines Prometheus alerting and recording rules that are scoped
to the current cluster. Only metric data from the current cluster is processed
and all rule results have their project_id and cluster label preserved
for query processing.
If the location label is not preserved by the rule, it defaults to the cluster's location.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `monitoring.googleapis.com/v1` | | |
| `kind` _string_ | `ClusterRules` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[RulesSpec](#rulesspec)_ | Specification of rules to record and alert on. |  |  |
| `status` _[RulesStatus](#rulesstatus)_ | Most recently observed status of the resource. |  | Optional: \{\} <br /> |


#### ClusterTargetLabels



ClusterTargetLabels configures labels for the discovered Prometheus targets.



_Appears in:_
- [ClusterPodMonitoringSpec](#clusterpodmonitoringspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _string array_ | Pod metadata labels that are set on all scraped targets.<br />Permitted keys are `container`, `namespace`, `node`, `pod`,<br />`top_level_controller_name` and `top_level_controller_type`. The `container`<br />label is only populated if the scrape port is referenced by name.<br />Defaults to [container, namespace, pod, top_level_controller_name, top_level_controller_type].<br />If set to null, it will be interpreted as  [namespace]. This is for backwards-compatibility<br />only. | [container namespace pod top_level_controller_name top_level_controller_type] | items:Enum: [container namespace node pod top_level_controller_name top_level_controller_type] <br />Optional: \{\} <br /> |
| `fromPod` _[LabelMapping](#labelmapping) array_ | Labels to transfer from the Kubernetes Pod to Prometheus target labels.<br />Mappings are applied in order. |  | MaxItems: 100 <br /> |


#### CollectionSpec



CollectionSpec specifies how the operator configures collection of metric data.



_Appears in:_
- [OperatorConfig](#operatorconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `externalLabels` _object (keys:string, values:string)_ | ExternalLabels specifies external labels that are attached to all scraped<br />data before being written to Google Cloud Monitoring or any other additional exports<br />specified in the OperatorConfig. The precedence behavior matches that of Prometheus. |  |  |
| `filter` _[ExportFilters](#exportfilters)_ | Filter limits which metric data is sent to Cloud Monitoring (it doesn't apply to additional exports). |  |  |
| `credentials` _[SecretKeySelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#secretkeyselector-v1-core)_ | A reference to GCP service account credentials with which Prometheus collectors<br />are run. It needs to have metric write permissions for all project IDs to which<br />data is written.<br />Within GKE, this can typically be left empty if the compute default<br />service account has the required permissions. |  |  |
| `kubeletScraping` _[KubeletScraping](#kubeletscraping)_ | Configuration to scrape the metric endpoints of the Kubelets. |  |  |
| `compression` _[CompressionType](#compressiontype)_ | Compression enables compression of metrics collection data. |  | Enum: [none gzip] <br /> |


#### CompressionType

_Underlying type:_ _string_

CompressionType is the compression type.

_Validation:_
- Enum: [none gzip]

_Appears in:_
- [CollectionSpec](#collectionspec)
- [ConfigSpec](#configspec)

| Field | Description |
| --- | --- |
| `none` |  |
| `gzip` |  |


#### ConfigSpec



ConfigSpec holds configurations for the Prometheus configuration.



_Appears in:_
- [OperatorFeatures](#operatorfeatures)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `compression` _[CompressionType](#compressiontype)_ | Compression enables compression of the config data propagated by the operator to collectors<br />and the rule-evaluator. It is recommended to use the gzip option when using a large number of<br />ClusterPodMonitoring, PodMonitoring, GlobalRules, ClusterRules, and/or Rules. |  | Enum: [none gzip] <br /> |


#### ExportFilters



ExportFilters provides mechanisms to filter the scraped data that's sent to GMP.



_Appears in:_
- [CollectionSpec](#collectionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enableMatchOneOf` _boolean_ | EnableMatchOneOf allows additional control over MatchOneOf filtering.<br />Available settings:<br />* not set: The MatchOneOf settings are ignored, default is used.<br />* false: MatchOneOf feature is explicitly disabled; export is forced to match all series.<br />* true: The MatchOneOf settings are used, overwriting a default.<br />See MatchOneOf IMPORTANT section to learn about the MatchOneOf default. |  |  |
| `matchOneOf` _string array_ | MatchOneOf, if EnableMatchOneOf is true, controls the export filtering setting.<br />MatchOneOf expects a list of Prometheus time series matchers. Every time series<br />must match at least one of the matchers to be exported. This field can be used<br />equivalently to the match[] parameter of the Prometheus federation endpoint to<br />selectively export data.<br />Example: `["\{job!='foobar'\}", "\{__name__!~'container_foo.*\|container_bar.*'\}"]`<br />IMPORTANT: MatchOneOf is guarded by the additional flag (EnableMatchOneOf)<br />and removed from the public docs. Replacements: https://cloud.google.com/stackdriver/docs/managed-prometheus/setup-managed#filter-metrics.<br />Rationales:<br />* This option is prone to misconfiguration, e.g. you need to manually<br />  match important built-in metrics (up, scrape_samples_added, etc.).<br />* Unmatched metrics are still in the local collector memory<br />  (this filters on export only).<br />* Typically, the default (empty array), means match all metrics. However,<br />  for the clusters that were using MatchOneOf filtering before the GMP 0.14.x,<br />  version, the default is what has been configured back then in the orphaned<br />  collector DaemonSet EXTRA_ARGS environment variable. If you are impacted:<br />  * use EnableMatchOneOf = false to reset the default<br />  * use EnableMatchOneOf = true to apply this configuration's MatchOneOf<br />  * Remove EXTRA_ARGS environment variable from the collector DaemonSet (it's safe to do so). |  |  |


#### ExportSpec







_Appears in:_
- [OperatorConfig](#operatorconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `url` _string_ | The URL of the endpoint that supports Prometheus Remote Write to export samples to. |  | Format: uri <br /> |


#### GlobalRules



GlobalRules defines Prometheus alerting and recording rules that are scoped
to all data in the queried project.
If the project_id or location labels are not preserved by the rule, they default to
the values of the cluster.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `monitoring.googleapis.com/v1` | | |
| `kind` _string_ | `GlobalRules` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[RulesSpec](#rulesspec)_ | Specification of rules to record and alert on. |  |  |
| `status` _[RulesStatus](#rulesstatus)_ | Most recently observed status of the resource. |  | Optional: \{\} <br /> |


#### HTTPClientConfig



HTTPClientConfig stores HTTP-client configurations.



_Appears in:_
- [ScrapeEndpoint](#scrapeendpoint)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `proxyUrl` _string_ | ProxyURL is the HTTP proxy server to use to connect to the targets.<br />Encoded passwords are not supported. |  | MaxLength: 2000 <br />Optional: \{\} <br /> |
| `authorization` _[Auth](#auth)_ | Authorization is the HTTP authorization credentials for the targets. |  | Optional: \{\} <br /> |
| `basicAuth` _[BasicAuth](#basicauth)_ | BasicAuth is the HTTP basic authentication credentials for the targets. |  | Optional: \{\} <br /> |
| `tls` _[TLS](#tls)_ | TLS configures the scrape request's TLS settings. |  | Optional: \{\} <br /> |
| `oauth2` _[OAuth2](#oauth2)_ | OAuth2 is the OAuth2 client credentials used to fetch a token for the targets. |  | Optional: \{\} <br /> |


#### KubeletScraping



KubeletScraping allows enabling scraping of the Kubelets' metric endpoints.



_Appears in:_
- [CollectionSpec](#collectionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `interval` _string_ | The interval at which the metric endpoints are scraped. |  | Format: duration <br /> |
| `tlsInsecureSkipVerify` _boolean_ | TLSInsecureSkipVerify disables verifying the target cert.<br />This can be useful for clusters provisioned with kubeadm. |  |  |


#### LabelMapping



LabelMapping specifies how to transfer a label from a Kubernetes resource
onto a Prometheus target.



_Appears in:_
- [ClusterTargetLabels](#clustertargetlabels)
- [TargetLabels](#targetlabels)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `from` _string_ | Kubernetes resource label to remap. |  |  |
| `to` _string_ | Remapped Prometheus target label.<br />Defaults to the same name as `From`. |  | Pattern: `^[a-zA-Z_][a-zA-Z0-9_]*$` <br /> |


#### ManagedAlertmanagerSpec



ManagedAlertmanagerSpec holds configuration information for the managed
Alertmanager instance.



_Appears in:_
- [OperatorConfig](#operatorconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `configSecret` _[SecretKeySelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#secretkeyselector-v1-core)_ | ConfigSecret refers to the name of a single-key Secret in the public namespace that<br />holds the managed Alertmanager config file. | \{ key:alertmanager.yaml name:alertmanager \} |  |
| `externalURL` _string_ | ExternalURL is the URL under which Alertmanager is externally reachable (for example, if<br />Alertmanager is served via a reverse proxy). Used for generating relative and absolute<br />links back to Alertmanager itself. If the URL has a path portion, it will be used to<br />prefix all HTTP endpoints served by Alertmanager, otherwise relevant URL components will<br />be derived automatically.<br />If no URL is provided, Alertmanager will point to the Google Cloud Metric Explorer page. |  | Format: uri <br /> |


#### MonitoringCondition



MonitoringCondition describes the condition of a PodMonitoring.



_Appears in:_
- [MonitoringStatus](#monitoringstatus)
- [PodMonitoringStatus](#podmonitoringstatus)
- [RulesStatus](#rulesstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _[MonitoringConditionType](#monitoringconditiontype)_ |  |  |  |
| `status` _[ConditionStatus](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#conditionstatus-v1-core)_ | Status of the condition, one of True, False, Unknown. |  |  |
| `lastUpdateTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#time-v1-meta)_ | The last time this condition was updated. |  | Optional: \{\} <br /> |
| `lastTransitionTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#time-v1-meta)_ | Last time the condition transitioned from one status to another. |  | Optional: \{\} <br /> |
| `reason` _string_ | The reason for the condition's last transition. |  | Optional: \{\} <br /> |
| `message` _string_ | A human-readable message indicating details about the transition. |  | Optional: \{\} <br /> |


#### MonitoringConditionType

_Underlying type:_ _string_

MonitoringConditionType is the type of MonitoringCondition.



_Appears in:_
- [MonitoringCondition](#monitoringcondition)

| Field | Description |
| --- | --- |
| `ConfigurationCreateSuccess` | ConfigurationCreateSuccess indicates that the config generated from the<br />monitoring resource was created successfully.<br /> |


#### MonitoringStatus



MonitoringStatus holds status information of a monitoring resource.



_Appears in:_
- [ClusterNodeMonitoring](#clusternodemonitoring)
- [PodMonitoringStatus](#podmonitoringstatus)
- [RulesStatus](#rulesstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | The generation observed by the controller. |  | Optional: \{\} <br /> |
| `conditions` _[MonitoringCondition](#monitoringcondition) array_ | Represents the latest available observations of a podmonitor's current state. |  |  |


#### OAuth2



OAuth2 is the OAuth2 client configuration.



_Appears in:_
- [HTTPClientConfig](#httpclientconfig)
- [ScrapeEndpoint](#scrapeendpoint)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `proxyUrl` _string_ | ProxyURL is the HTTP proxy server to use to connect to the targets.<br />Encoded passwords are not supported. |  | MaxLength: 2000 <br />Optional: \{\} <br /> |
| `clientID` _string_ | ClientID is the public identifier for the client. |  | Optional: \{\} <br /> |
| `clientSecret` _[SecretSelector](#secretselector)_ | ClientSecret uses the secret as the client secret token. |  | Optional: \{\} <br /> |
| `scopes` _string array_ | Scopes represents the scopes for the token request. |  | Optional: \{\} <br /> |
| `tokenURL` _string_ | TokenURL is the URL to fetch the token from. |  | Optional: \{\} <br /> |
| `endpointParams` _object (keys:string, values:string)_ | EndpointParams are additional parameters to append to the token URL. |  | Optional: \{\} <br /> |
| `tlsConfig` _[TLS](#tls)_ | TLS configures the token request's TLS settings. |  | Optional: \{\} <br /> |


#### OperatorConfig



OperatorConfig defines configuration of the gmp-operator.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `monitoring.googleapis.com/v1` | | |
| `kind` _string_ | `OperatorConfig` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `rules` _[RuleEvaluatorSpec](#ruleevaluatorspec)_ | Rules specifies how the operator configures and deploys rule-evaluator. |  |  |
| `collection` _[CollectionSpec](#collectionspec)_ | Collection specifies how the operator configures collection, including<br />scraping and an integrated export to Google Cloud Monitoring. |  |  |
| `exports` _[ExportSpec](#exportspec) array_ | Exports is an EXPERIMENTAL feature that specifies additional, optional endpoints to export to,<br />on top of Google Cloud Monitoring collection.<br />Note: To disable integrated export to Google Cloud Monitoring specify a non-matching filter in the "collection.filter" field. |  |  |
| `managedAlertmanager` _[ManagedAlertmanagerSpec](#managedalertmanagerspec)_ | ManagedAlertmanager holds information for configuring the managed instance of Alertmanager. | \{ configSecret:map[key:alertmanager.yaml name:alertmanager] \} |  |
| `features` _[OperatorFeatures](#operatorfeatures)_ | Features holds configuration for optional managed-collection features. |  |  |
| `scaling` _[ScalingSpec](#scalingspec)_ | Scaling contains configuration options for scaling GMP. |  |  |


#### OperatorFeatures



OperatorFeatures holds configuration for optional managed-collection features.



_Appears in:_
- [OperatorConfig](#operatorconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `targetStatus` _[TargetStatusSpec](#targetstatusspec)_ | Configuration of target status reporting. |  |  |
| `config` _[ConfigSpec](#configspec)_ | Settings for the collector configuration propagation. |  |  |


#### PodMonitoring



PodMonitoring defines monitoring for a set of pods, scoped to pods
within the PodMonitoring's namespace.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `monitoring.googleapis.com/v1` | | |
| `kind` _string_ | `PodMonitoring` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[PodMonitoringSpec](#podmonitoringspec)_ | Specification of desired Pod selection for target discovery by<br />Prometheus. |  |  |
| `status` _[PodMonitoringStatus](#podmonitoringstatus)_ | Most recently observed status of the resource. |  | Optional: \{\} <br /> |


#### PodMonitoringSpec



PodMonitoringSpec contains specification parameters for PodMonitoring.



_Appears in:_
- [PodMonitoring](#podmonitoring)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `selector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#labelselector-v1-meta)_ | Label selector that specifies which pods are selected for this monitoring<br />configuration. |  |  |
| `endpoints` _[ScrapeEndpoint](#scrapeendpoint) array_ | The endpoints to scrape on the selected pods. |  | MaxItems: 10 <br />MinItems: 1 <br /> |
| `targetLabels` _[TargetLabels](#targetlabels)_ | Labels to add to the Prometheus target for discovered endpoints.<br />The `instance` label is always set to `<pod_name>:<port>` or `<node_name>:<port>`<br />if the scraped pod is controlled by a DaemonSet. | \{  \} | Optional: \{\} <br /> |
| `limits` _[ScrapeLimits](#scrapelimits)_ | Limits to apply at scrape time. |  |  |
| `filterRunning` _boolean_ | FilterRunning will drop any pods that are in the "Failed" or "Succeeded"<br />pod lifecycle.<br />See: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-phase |  |  |


#### PodMonitoringStatus



PodMonitoringStatus holds status information of a PodMonitoring resource.



_Appears in:_
- [ClusterPodMonitoring](#clusterpodmonitoring)
- [PodMonitoring](#podmonitoring)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | The generation observed by the controller. |  | Optional: \{\} <br /> |
| `conditions` _[MonitoringCondition](#monitoringcondition) array_ | Represents the latest available observations of a podmonitor's current state. |  |  |
| `endpointStatuses` _[ScrapeEndpointStatus](#scrapeendpointstatus) array_ | Represents the latest available observations of target state for each ScrapeEndpoint. |  |  |


#### ProxyConfig



ProxyConfig specifies the proxy HTTP configuration.



_Appears in:_
- [HTTPClientConfig](#httpclientconfig)
- [OAuth2](#oauth2)
- [ScrapeEndpoint](#scrapeendpoint)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `proxyUrl` _string_ | ProxyURL is the HTTP proxy server to use to connect to the targets.<br />Encoded passwords are not supported. |  | MaxLength: 2000 <br />Optional: \{\} <br /> |


#### RelabelingRule



RelabelingRule defines a single Prometheus relabeling rule.



_Appears in:_
- [ScrapeEndpoint](#scrapeendpoint)
- [ScrapeNodeEndpoint](#scrapenodeendpoint)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `sourceLabels` _string array_ | The source labels select values from existing labels. Their content is concatenated<br />using the configured separator and matched against the configured regular expression<br />for the replace, keep, and drop actions. |  | MaxItems: 100 <br />items:Pattern: `^[a-zA-Z_][a-zA-Z0-9_]*$` <br /> |
| `separator` _string_ | Separator placed between concatenated source label values. Defaults to ';'. |  |  |
| `targetLabel` _string_ | Label to which the resulting value is written in a replace action.<br />It is mandatory for replace actions. Regex capture groups are available. |  | Pattern: `^[a-zA-Z_][a-zA-Z0-9_]*$` <br /> |
| `regex` _string_ | Regular expression against which the extracted value is matched. Defaults to '(.*)'. |  | MaxLength: 10000 <br /> |
| `modulus` _integer_ | Modulus to take of the hash of the source label values. |  |  |
| `replacement` _string_ | Replacement value against which a regex replace is performed if the<br />regular expression matches. Regex capture groups are available. Defaults to '$1'. |  |  |
| `action` _string_ | Action to perform based on regex matching. Defaults to 'replace'. |  | Enum: [replace lowercase uppercase keep drop keepequal dropequal hashmod labeldrop labelkeep] <br /> |


#### Rule



Rule is a single rule in the Prometheus format:
https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/



_Appears in:_
- [RuleGroup](#rulegroup)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `record` _string_ | Record the result of the expression to this metric name.<br />Only one of `record` and `alert` must be set. |  | MinLength: 1 <br />Pattern: `^[a-zA-Z_:][a-zA-Z0-9_:]*$` <br /> |
| `alert` _string_ | Name of the alert to evaluate the expression as.<br />Only one of `record` and `alert` must be set. |  | MinLength: 1 <br /> |
| `expr` _string_ | The PromQL expression to evaluate. |  | MinLength: 1 <br /> |
| `for` _string_ | The duration to wait before a firing alert produced by this rule is sent to Alertmanager.<br />Only valid if `alert` is set. |  | Format: duration <br />MinLength: 1 <br /> |
| `labels` _object (keys:string, values:string)_ | A set of labels to attach to the result of the query expression. |  | MaxProperties: 64 <br /> |
| `annotations` _object (keys:string, values:string)_ | A set of annotations to attach to alerts produced by the query expression.<br />Only valid if `alert` is set. |  | MaxProperties: 64 <br /> |


#### RuleEvaluatorSpec



RuleEvaluatorSpec defines configuration for deploying rule-evaluator.



_Appears in:_
- [OperatorConfig](#operatorconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `externalLabels` _object (keys:string, values:string)_ | ExternalLabels specifies external labels that are attached to any rule<br />results and alerts produced by rules. The precedence behavior matches that<br />of Prometheus. |  |  |
| `queryProjectID` _string_ | QueryProjectID is the GCP project ID to evaluate rules against.<br />If left blank, the rule-evaluator will try attempt to infer the Project ID<br />from the environment. |  |  |
| `generatorUrl` _string_ | The base URL used for the generator URL in the alert notification payload.<br />Should point to an instance of a query frontend that gives access to queryProjectID. |  | Format: uri <br /> |
| `alerting` _[AlertingSpec](#alertingspec)_ | Alerting contains how the rule-evaluator configures alerting. |  |  |
| `credentials` _[SecretKeySelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#secretkeyselector-v1-core)_ | A reference to GCP service account credentials with which the rule<br />evaluator container is run. It needs to have metric read permissions<br />against queryProjectId and metric write permissions against all projects<br />to which rule results are written.<br />Within GKE, this can typically be left empty if the compute default<br />service account has the required permissions. |  |  |


#### RuleGroup



RuleGroup declares rules in the Prometheus format:
https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/



_Appears in:_
- [RulesSpec](#rulesspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | The name of the rule group. |  | MaxLength: 1024 <br />MinLength: 1 <br /> |
| `interval` _string_ | The interval at which to evaluate the rules. Must be a valid Prometheus duration. | 1m | Format: duration <br />MinLength: 1 <br /> |
| `rules` _[Rule](#rule) array_ | A list of rules that are executed sequentially as part of this group. |  | MaxItems: 200 <br />MinItems: 1 <br /> |


#### Rules



Rules defines Prometheus alerting and recording rules that are scoped
to the namespace of the resource. Only metric data from this namespace is processed
and all rule results have their project_id, cluster, and namespace label preserved
for query processing.
If the location label is not preserved by the rule, it defaults to the cluster's location.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `monitoring.googleapis.com/v1` | | |
| `kind` _string_ | `Rules` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[RulesSpec](#rulesspec)_ | Specification of rules to record and alert on. |  |  |
| `status` _[RulesStatus](#rulesstatus)_ | Most recently observed status of the resource. |  | Optional: \{\} <br /> |


#### RulesSpec



RulesSpec contains specification parameters for a Rules resource.



_Appears in:_
- [ClusterRules](#clusterrules)
- [GlobalRules](#globalrules)
- [Rules](#rules)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `groups` _[RuleGroup](#rulegroup) array_ | A list of Prometheus rule groups. |  | MaxItems: 50 <br /> |


#### RulesStatus



RulesStatus contains status information for a Rules resource.



_Appears in:_
- [ClusterRules](#clusterrules)
- [GlobalRules](#globalrules)
- [Rules](#rules)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | The generation observed by the controller. |  | Optional: \{\} <br /> |
| `conditions` _[MonitoringCondition](#monitoringcondition) array_ | Represents the latest available observations of a podmonitor's current state. |  |  |


#### SampleGroup







_Appears in:_
- [ScrapeEndpointStatus](#scrapeendpointstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `sampleTargets` _[SampleTarget](#sampletarget) array_ | Targets emitting the error message. |  |  |
| `count` _integer_ | Total count of similar errors. |  | Optional: \{\} <br /> |


#### SampleTarget







_Appears in:_
- [SampleGroup](#samplegroup)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `labels` _[LabelSet](https://pkg.go.dev/github.com/prometheus/common/model#LabelSet)_ | The label set, keys and values, of the target. |  |  |
| `lastError` _string_ | Error message. |  |  |
| `lastScrapeDurationSeconds` _string_ | Scrape duration in seconds. |  |  |
| `health` _string_ | Health status. |  |  |


#### ScalingSpec



ScalingSpec defines configuration options for scaling GMP.



_Appears in:_
- [OperatorConfig](#operatorconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `vpa` _[VPASpec](#vpaspec)_ |  |  |  |


#### ScrapeEndpoint



ScrapeEndpoint specifies a Prometheus metrics endpoint to scrape.



_Appears in:_
- [ClusterPodMonitoringSpec](#clusterpodmonitoringspec)
- [PodMonitoringSpec](#podmonitoringspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `proxyUrl` _string_ | ProxyURL is the HTTP proxy server to use to connect to the targets.<br />Encoded passwords are not supported. |  | MaxLength: 2000 <br />Optional: \{\} <br /> |
| `authorization` _[Auth](#auth)_ | Authorization is the HTTP authorization credentials for the targets. |  | Optional: \{\} <br /> |
| `basicAuth` _[BasicAuth](#basicauth)_ | BasicAuth is the HTTP basic authentication credentials for the targets. |  | Optional: \{\} <br /> |
| `tls` _[TLS](#tls)_ | TLS configures the scrape request's TLS settings. |  | Optional: \{\} <br /> |
| `oauth2` _[OAuth2](#oauth2)_ | OAuth2 is the OAuth2 client credentials used to fetch a token for the targets. |  | Optional: \{\} <br /> |
| `port` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util)_ | Name or number of the port to scrape.<br />The container metadata label is only populated if the port is referenced by name<br />because port numbers are not unique across containers. |  | MaxLength: 253 <br />MinLength: 1 <br />XIntOrString: \{\} <br />Required: \{\} <br /> |
| `scheme` _string_ | Protocol scheme to use to scrape. |  | Enum: [http https] <br /> |
| `path` _string_ | HTTP path to scrape metrics from. Defaults to "/metrics". |  |  |
| `params` _object (keys:string, values:string array)_ | HTTP GET params to use when scraping. |  |  |
| `interval` _string_ | Interval at which to scrape metrics. Must be a valid Prometheus duration. |  | Format: duration <br />Required: \{\} <br /> |
| `timeout` _string_ | Timeout for metrics scrapes. Must be a valid Prometheus duration.<br />Must not be larger than the scrape interval. |  | Format: duration <br /> |
| `metricRelabeling` _[RelabelingRule](#relabelingrule) array_ | Relabeling rules for metrics scraped from this endpoint. Relabeling rules that<br />override protected target labels (project_id, location, cluster, namespace, job,<br />instance, top_level_controller, top_level_controller_type, or `__address__`) are<br />not permitted. The labelmap action is not permitted in general. |  | MaxItems: 250 <br /> |


#### ScrapeEndpointStatus







_Appears in:_
- [PodMonitoringStatus](#podmonitoringstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | The name of the ScrapeEndpoint. |  |  |
| `activeTargets` _integer_ | Total number of active targets. |  |  |
| `unhealthyTargets` _integer_ | Total number of active, unhealthy targets. |  |  |
| `lastUpdateTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#time-v1-meta)_ | Last time this status was updated. |  |  |
| `sampleGroups` _[SampleGroup](#samplegroup) array_ | A fixed sample of targets grouped by error type. |  |  |
| `collectorsFraction` _string_ | Fraction of collectors included in status, bounded [0,1].<br />Ideally, this should always be 1. Anything less can<br />be considered a problem and should be investigated. |  |  |


#### ScrapeLimits



ScrapeLimits limits applied to scraped targets.



_Appears in:_
- [ClusterNodeMonitoringSpec](#clusternodemonitoringspec)
- [ClusterPodMonitoringSpec](#clusterpodmonitoringspec)
- [PodMonitoringSpec](#podmonitoringspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `samples` _integer_ | Maximum number of samples accepted within a single scrape.<br />Uses Prometheus default if left unspecified. |  |  |
| `labels` _integer_ | Maximum number of labels accepted for a single sample.<br />Uses Prometheus default if left unspecified. |  |  |
| `labelNameLength` _integer_ | Maximum label name length.<br />Uses Prometheus default if left unspecified. |  |  |
| `labelValueLength` _integer_ | Maximum label value length.<br />Uses Prometheus default if left unspecified. |  |  |


#### ScrapeNodeEndpoint



ScrapeNodeEndpoint specifies a Prometheus metrics endpoint on a node to scrape.
It contains all the fields used in the ScrapeEndpoint except for port and HTTPClientConfig.



_Appears in:_
- [ClusterNodeMonitoringSpec](#clusternodemonitoringspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `scheme` _string_ | Protocol scheme to use to scrape. |  | Enum: [http https] <br /> |
| `path` _string_ | HTTP path to scrape metrics from. Defaults to "/metrics". |  |  |
| `params` _object (keys:string, values:string array)_ | HTTP GET params to use when scraping. |  |  |
| `interval` _string_ | Interval at which to scrape metrics. Must be a valid Prometheus duration. | 1m | Format: duration <br />Required: \{\} <br /> |
| `timeout` _string_ | Timeout for metrics scrapes. Must be a valid Prometheus duration.<br />Must not be larger then the scrape interval. |  | Format: duration <br /> |
| `metricRelabeling` _[RelabelingRule](#relabelingrule) array_ | Relabeling rules for metrics scraped from this endpoint. Relabeling rules that<br />override protected target labels (project_id, location, cluster, namespace, job,<br />instance, or `__address__`) are not permitted. The labelmap action is not permitted<br />in general. |  | MaxItems: 250 <br /> |
| `tls` _[ClusterNodeTLS](#clusternodetls)_ | TLS configures the scrape request's TLS settings. |  | Optional: \{\} <br /> |


#### SecretKeySelector



SecretKeySelector represents selector for Kubernetes secret.
It's similar to k8s.io/api/core/v1.SecretKeySelector, but allows cross namespace selections.



_Appears in:_
- [SecretSelector](#secretselector)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the secret to select from. |  |  |
| `key` _string_ | Key of the secret to select from. Must be a valid secret key. |  |  |
| `namespace` _string_ | Namespace of the secret to select from.<br />If empty the parent resource namespace will be chosen. |  | Optional: \{\} <br /> |


#### SecretOrConfigMap



SecretOrConfigMap allows to specify data as a Secret or ConfigMap. Fields are mutually exclusive.
Taking inspiration from prometheus-operator: https://github.com/prometheus-operator/prometheus-operator/blob/2c81b0cf6a5673e08057499a08ddce396b19dda4/Documentation/api.md#secretorconfigmap



_Appears in:_
- [TLSConfig](#tlsconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `secret` _[SecretKeySelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#secretkeyselector-v1-core)_ | Secret containing data to use for the targets. |  |  |
| `configMap` _[ConfigMapKeySelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#configmapkeyselector-v1-core)_ | ConfigMap containing data to use for the targets. |  |  |


#### SecretSelector



SecretSelector references a secret from a secret provider e.g. Kubernetes Secret. Only one
provider can be used at a time.



_Appears in:_
- [Auth](#auth)
- [BasicAuth](#basicauth)
- [OAuth2](#oauth2)
- [TLS](#tls)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `secret` _[SecretKeySelector](#secretkeyselector)_ | Secret represents reference to a given key from certain Secret in a given namespace. |  | Optional: \{\} <br /> |


#### TLS



TLS specifies TLS configuration used for HTTP requests.



_Appears in:_
- [HTTPClientConfig](#httpclientconfig)
- [OAuth2](#oauth2)
- [ScrapeEndpoint](#scrapeendpoint)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `serverName` _string_ | ServerName is used to verify the hostname for the targets. |  | Optional: \{\} <br /> |
| `insecureSkipVerify` _boolean_ | InsecureSkipVerify disables target certificate validation. |  | Optional: \{\} <br /> |
| `minVersion` _string_ | MinVersion is the minimum TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS 1.1),<br />TLS12 (TLS 1.2), TLS13 (TLS 1.3).<br />If unset, Prometheus will use Go default minimum version, which is TLS 1.2.<br />See MinVersion in https://pkg.go.dev/crypto/tls#Config. |  | Enum: [TLS10 TLS11 TLS12 TLS13] <br />Optional: \{\} <br /> |
| `maxVersion` _string_ | MaxVersion is the maximum TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS 1.1),<br />TLS12 (TLS 1.2), TLS13 (TLS 1.3).<br />If unset, Prometheus will use Go default minimum version, which is TLS 1.2.<br />See MinVersion in https://pkg.go.dev/crypto/tls#Config. |  | Enum: [TLS10 TLS11 TLS12 TLS13] <br />Optional: \{\} <br /> |
| `ca` _[SecretSelector](#secretselector)_ | CA uses the secret as the CA certificate to validate the server with. |  | Optional: \{\} <br /> |
| `cert` _[SecretSelector](#secretselector)_ | Cert uses the secret as the certificate for client authentication to the server. |  | Optional: \{\} <br /> |
| `key` _[SecretSelector](#secretselector)_ | Key uses the secret as the private key for client authentication to the server. |  | Optional: \{\} <br /> |


#### TLSConfig



TLSConfig specifies TLS configuration parameters from Kubernetes resources.



_Appears in:_
- [AlertmanagerEndpoints](#alertmanagerendpoints)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ca` _[SecretOrConfigMap](#secretorconfigmap)_ | Struct containing the CA cert to use for the targets. |  |  |
| `cert` _[SecretOrConfigMap](#secretorconfigmap)_ | Struct containing the client cert file for the targets. |  |  |
| `keySecret` _[SecretKeySelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#secretkeyselector-v1-core)_ | Secret containing the client key file for the targets. |  |  |
| `serverName` _string_ | Used to verify the hostname for the targets. |  |  |
| `insecureSkipVerify` _boolean_ | Disable target certificate validation. |  |  |
| `minVersion` _string_ | Minimum TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS 1.1), TLS12 (TLS 1.2), TLS13 (TLS 1.3).<br />If unset, Prometheus will use Go default minimum version, which is TLS 1.2.<br />See MinVersion in https://pkg.go.dev/crypto/tls#Config. |  |  |
| `maxVersion` _string_ | Maximum TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS 1.1), TLS12 (TLS 1.2), TLS13 (TLS 1.3).<br />If unset, Prometheus will use Go default minimum version, which is TLS 1.2.<br />See MinVersion in https://pkg.go.dev/crypto/tls#Config. |  |  |


#### TargetLabels



TargetLabels configures labels for the discovered Prometheus targets.



_Appears in:_
- [PodMonitoringSpec](#podmonitoringspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _string array_ | Pod metadata labels that are set on all scraped targets.<br />Permitted keys are `container`, `node`, `pod`, `top_level_controller_name`,<br />and `top_level_controller_type`. The `container`<br />label is only populated if the scrape port is referenced by name.<br />Defaults to [container, pod, top_level_controller_name, top_level_controller_type].<br />If set to null, it will be interpreted as the empty list.<br />This is for backwards-compatibility only. | [container pod top_level_controller_name top_level_controller_type] | items:Enum: [container node pod top_level_controller_name top_level_controller_type] <br />Optional: \{\} <br /> |
| `fromPod` _[LabelMapping](#labelmapping) array_ | Labels to transfer from the Kubernetes Pod to Prometheus target labels.<br />Mappings are applied in order. |  | MaxItems: 100 <br /> |


#### TargetStatusSpec



TargetStatusSpec holds configuration for target status reporting.



_Appears in:_
- [OperatorFeatures](#operatorfeatures)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enable target status reporting. |  |  |


#### VPASpec



VPASpec defines configuration options for vertical pod autoscaling.



_Appears in:_
- [ScalingSpec](#scalingspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled configures whether the operator configures Vertical Pod Autoscaling for GMP workloads.<br />In GKE, installing Vertical Pod Autoscaling requires a cluster restart, and therefore it also results in an operator restart.<br />In other environments, the operator may need to be restarted to enable VPA to run the following check again and watch for the objects. |  |  |


