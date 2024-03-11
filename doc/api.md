<p>Packages:</p>
<ul>
<li>
<a href="#monitoring.googleapis.com%2fv1">monitoring.googleapis.com/v1</a>
</li>
</ul>
<h2 id="monitoring.googleapis.com/v1">monitoring.googleapis.com/v1</h2>
<div>
<p>Package v1 is the v1 version of the API.</p>
</div>
Resource Types:
<ul><li>
<a href="#monitoring.googleapis.com/v1.AlertingSpec">AlertingSpec</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.AlertmanagerEndpoints">AlertmanagerEndpoints</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.Auth">Auth</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.Authorization">Authorization</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.BasicAuth">BasicAuth</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.ClusterNodeMonitoring">ClusterNodeMonitoring</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.ClusterNodeMonitoringSpec">ClusterNodeMonitoringSpec</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.ClusterPodMonitoring">ClusterPodMonitoring</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.ClusterPodMonitoringSpec">ClusterPodMonitoringSpec</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.ClusterRules">ClusterRules</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.CollectionSpec">CollectionSpec</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.CompressionType">CompressionType</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.ConfigSpec">ConfigSpec</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.ExportFilters">ExportFilters</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.ExportSpec">ExportSpec</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.GlobalRules">GlobalRules</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.HTTPClientConfig">HTTPClientConfig</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.KubeletScraping">KubeletScraping</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.LabelMapping">LabelMapping</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.ManagedAlertmanagerSpec">ManagedAlertmanagerSpec</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.MonitoringCRD">MonitoringCRD</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.MonitoringCondition">MonitoringCondition</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.MonitoringConditionType">MonitoringConditionType</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.MonitoringStatus">MonitoringStatus</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.OAuth2">OAuth2</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.OperatorConfig">OperatorConfig</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.OperatorFeatures">OperatorFeatures</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.PodMonitoring">PodMonitoring</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.PodMonitoringCRD">PodMonitoringCRD</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.PodMonitoringSpec">PodMonitoringSpec</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.PodMonitoringStatus">PodMonitoringStatus</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.ProxyConfig">ProxyConfig</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.RelabelingRule">RelabelingRule</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.Rule">Rule</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.RuleEvaluatorSpec">RuleEvaluatorSpec</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.RuleGroup">RuleGroup</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.Rules">Rules</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.RulesSpec">RulesSpec</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.RulesStatus">RulesStatus</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.SampleGroup">SampleGroup</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.SampleTarget">SampleTarget</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.ScrapeEndpoint">ScrapeEndpoint</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.ScrapeEndpointStatus">ScrapeEndpointStatus</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.ScrapeLimits">ScrapeLimits</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.ScrapeNodeEndpoint">ScrapeNodeEndpoint</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.SecretOrConfigMap">SecretOrConfigMap</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.TLS">TLS</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.TLSConfig">TLSConfig</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.TargetLabels">TargetLabels</a>
</li><li>
<a href="#monitoring.googleapis.com/v1.TargetStatusSpec">TargetStatusSpec</a>
</li></ul>
<h3 id="monitoring.googleapis.com/v1.AlertingSpec">
<span id="AlertingSpec">AlertingSpec
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.RuleEvaluatorSpec">RuleEvaluatorSpec</a>)
</p>
<div>
<p>AlertingSpec defines alerting configuration.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>alertmanagers</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.AlertmanagerEndpoints">
[]AlertmanagerEndpoints
</a>
</em>
</td>
<td>
<p>Alertmanagers contains endpoint configuration for designated Alertmanagers.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.AlertmanagerEndpoints">
<span id="AlertmanagerEndpoints">AlertmanagerEndpoints
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.AlertingSpec">AlertingSpec</a>)
</p>
<div>
<p>AlertmanagerEndpoints defines a selection of a single Endpoints object
containing alertmanager IPs to fire alerts against.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>namespace</code><br/>
<em>
string
</em>
</td>
<td>
<p>Namespace of Endpoints object.</p>
</td>
</tr>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<p>Name of Endpoints object in Namespace.</p>
</td>
</tr>
<tr>
<td>
<code>port</code><br/>
<em>
k8s.io/apimachinery/pkg/util/intstr.IntOrString
</em>
</td>
<td>
<p>Port the Alertmanager API is exposed on.</p>
</td>
</tr>
<tr>
<td>
<code>scheme</code><br/>
<em>
string
</em>
</td>
<td>
<p>Scheme to use when firing alerts.</p>
</td>
</tr>
<tr>
<td>
<code>pathPrefix</code><br/>
<em>
string
</em>
</td>
<td>
<p>Prefix for the HTTP path alerts are pushed to.</p>
</td>
</tr>
<tr>
<td>
<code>tls</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.TLSConfig">
TLSConfig
</a>
</em>
</td>
<td>
<p>TLS Config to use for alertmanager connection.</p>
</td>
</tr>
<tr>
<td>
<code>authorization</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.Authorization">
Authorization
</a>
</em>
</td>
<td>
<p>Authorization section for this alertmanager endpoint</p>
</td>
</tr>
<tr>
<td>
<code>apiVersion</code><br/>
<em>
string
</em>
</td>
<td>
<p>Version of the Alertmanager API that rule-evaluator uses to send alerts. It
can be &ldquo;v1&rdquo; or &ldquo;v2&rdquo;.</p>
</td>
</tr>
<tr>
<td>
<code>timeout</code><br/>
<em>
string
</em>
</td>
<td>
<p>Timeout is a per-target Alertmanager timeout when pushing alerts.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.Auth">
<span id="Auth">Auth
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.HTTPClientConfig">HTTPClientConfig</a>)
</p>
<div>
<p>Auth sets the <code>Authorization</code> header on every scrape request.</p>
<p>Currently the credentials are not configurable and always empty.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>type</code><br/>
<em>
string
</em>
</td>
<td>
<p>The authentication type. Defaults to Bearer, Basic will cause an error.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.Authorization">
<span id="Authorization">Authorization
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.AlertmanagerEndpoints">AlertmanagerEndpoints</a>)
</p>
<div>
<p>Authorization specifies a subset of the Authorization struct, that is
safe for use in Endpoints (no CredentialsFile field).</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>type</code><br/>
<em>
string
</em>
</td>
<td>
<p>Set the authentication type. Defaults to Bearer, Basic will cause an
error</p>
</td>
</tr>
<tr>
<td>
<code>credentials</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#secretkeyselector-v1-core">
Kubernetes core/v1.SecretKeySelector
</a>
</em>
</td>
<td>
<p>The secret&rsquo;s key that contains the credentials of the request</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.BasicAuth">
<span id="BasicAuth">BasicAuth
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.HTTPClientConfig">HTTPClientConfig</a>)
</p>
<div>
<p>BasicAuth sets the <code>Authorization</code> header on every scrape request with the
configured username.</p>
<p>Currently the password is not configurable and always empty.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>username</code><br/>
<em>
string
</em>
</td>
<td>
<p>The username for authentication.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.ClusterNodeMonitoring">
<span id="ClusterNodeMonitoring">ClusterNodeMonitoring
</span>
</h3>
<div>
<p>ClusterNodeMonitoring defines monitoring for a set of nodes.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.ClusterNodeMonitoringSpec">
ClusterNodeMonitoringSpec
</a>
</em>
</td>
<td>
<p>Specification of desired node selection for target discovery by
Prometheus.</p>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.MonitoringStatus">
MonitoringStatus
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Most recently observed status of the resource.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.ClusterNodeMonitoringSpec">
<span id="ClusterNodeMonitoringSpec">ClusterNodeMonitoringSpec
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.ClusterNodeMonitoring">ClusterNodeMonitoring</a>)
</p>
<div>
<p>ClusterNodeMonitoringSpec contains specification parameters for ClusterNodeMonitoring.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>selector</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#labelselector-v1-meta">
Kubernetes meta/v1.LabelSelector
</a>
</em>
</td>
<td>
<p>Label selector that specifies which nodes are selected for this monitoring
configuration. If left empty all nodes are selected.</p>
</td>
</tr>
<tr>
<td>
<code>endpoints</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.ScrapeNodeEndpoint">
[]ScrapeNodeEndpoint
</a>
</em>
</td>
<td>
<p>The endpoints to scrape on the selected nodes.</p>
</td>
</tr>
<tr>
<td>
<code>limits</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.ScrapeLimits">
ScrapeLimits
</a>
</em>
</td>
<td>
<p>Limits to apply at scrape time.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.ClusterPodMonitoring">
<span id="ClusterPodMonitoring">ClusterPodMonitoring
</span>
</h3>
<div>
<p>ClusterPodMonitoring defines monitoring for a set of pods, scoped to all
pods within the cluster.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.ClusterPodMonitoringSpec">
ClusterPodMonitoringSpec
</a>
</em>
</td>
<td>
<p>Specification of desired Pod selection for target discovery by
Prometheus.</p>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.PodMonitoringStatus">
PodMonitoringStatus
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Most recently observed status of the resource.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.ClusterPodMonitoringSpec">
<span id="ClusterPodMonitoringSpec">ClusterPodMonitoringSpec
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.ClusterPodMonitoring">ClusterPodMonitoring</a>)
</p>
<div>
<p>ClusterPodMonitoringSpec contains specification parameters for ClusterPodMonitoring.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>selector</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#labelselector-v1-meta">
Kubernetes meta/v1.LabelSelector
</a>
</em>
</td>
<td>
<p>Label selector that specifies which pods are selected for this monitoring
configuration.</p>
</td>
</tr>
<tr>
<td>
<code>endpoints</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.ScrapeEndpoint">
[]ScrapeEndpoint
</a>
</em>
</td>
<td>
<p>The endpoints to scrape on the selected pods.</p>
</td>
</tr>
<tr>
<td>
<code>targetLabels</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.TargetLabels">
TargetLabels
</a>
</em>
</td>
<td>
<p>Labels to add to the Prometheus target for discovered endpoints.
The <code>instance</code> label is always set to <code>&lt;pod_name&gt;:&lt;port&gt;</code> or <code>&lt;node_name&gt;:&lt;port&gt;</code>
if the scraped pod is controlled by a DaemonSet.</p>
</td>
</tr>
<tr>
<td>
<code>limits</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.ScrapeLimits">
ScrapeLimits
</a>
</em>
</td>
<td>
<p>Limits to apply at scrape time.</p>
</td>
</tr>
<tr>
<td>
<code>filterRunning</code><br/>
<em>
bool
</em>
</td>
<td>
<p>FilterRunning will drop any pods that are in the &ldquo;Failed&rdquo; or &ldquo;Succeeded&rdquo;
pod lifecycle.
See: <a href="https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-phase">https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-phase</a>
Specifically, this prevents scraping Succeeded pods from K8s jobs, which
could contribute to noisy logs or irrelevant metrics.
Additionally, it can mitigate issues with reusing stale target
labels in cases where Pod IPs are reused (e.g. spot containers).
See: <a href="https://github.com/GoogleCloudPlatform/prometheus-engine/issues/145">https://github.com/GoogleCloudPlatform/prometheus-engine/issues/145</a></p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.ClusterRules">
<span id="ClusterRules">ClusterRules
</span>
</h3>
<div>
<p>ClusterRules defines Prometheus alerting and recording rules that are scoped
to the current cluster. Only metric data from the current cluster is processed
and all rule results have their project_id and cluster label preserved
for query processing.
If the location label is not preserved by the rule, it defaults to the cluster&rsquo;s location.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.RulesSpec">
RulesSpec
</a>
</em>
</td>
<td>
<p>Specification of rules to record and alert on.</p>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.RulesStatus">
RulesStatus
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Most recently observed status of the resource.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.CollectionSpec">
<span id="CollectionSpec">CollectionSpec
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.OperatorConfig">OperatorConfig</a>)
</p>
<div>
<p>CollectionSpec specifies how the operator configures collection of metric data.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>externalLabels</code><br/>
<em>
map[string]string
</em>
</td>
<td>
<p>ExternalLabels specifies external labels that are attached to all scraped
data before being written to Google Cloud Monitoring or any other additional exports
specified in the OperatorConfig. The precedence behavior matches that of Prometheus.</p>
</td>
</tr>
<tr>
<td>
<code>filter</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.ExportFilters">
ExportFilters
</a>
</em>
</td>
<td>
<p>Filter limits which metric data is sent to Cloud Monitoring (it doesn&rsquo;t apply to additional exports).</p>
</td>
</tr>
<tr>
<td>
<code>credentials</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#secretkeyselector-v1-core">
Kubernetes core/v1.SecretKeySelector
</a>
</em>
</td>
<td>
<p>A reference to GCP service account credentials with which Prometheus collectors
are run. It needs to have metric write permissions for all project IDs to which
data is written.
Within GKE, this can typically be left empty if the compute default
service account has the required permissions.</p>
</td>
</tr>
<tr>
<td>
<code>kubeletScraping</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.KubeletScraping">
KubeletScraping
</a>
</em>
</td>
<td>
<p>Configuration to scrape the metric endpoints of the Kubelets.</p>
</td>
</tr>
<tr>
<td>
<code>compression</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.CompressionType">
CompressionType
</a>
</em>
</td>
<td>
<p>Compression enables compression of metrics collection data</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.CompressionType">
<span id="CompressionType">CompressionType
(<code>string</code> alias)</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.CollectionSpec">CollectionSpec</a>, <a href="#monitoring.googleapis.com/v1.ConfigSpec">ConfigSpec</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;gzip&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;none&#34;</p></td>
<td></td>
</tr></tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.ConfigSpec">
<span id="ConfigSpec">ConfigSpec
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.OperatorFeatures">OperatorFeatures</a>)
</p>
<div>
<p>ConfigSpec holds configurations for the Prometheus configuration.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>compression</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.CompressionType">
CompressionType
</a>
</em>
</td>
<td>
<p>Compression enables compression of the config data propagated by the operator to collectors.
It is recommended to use the gzip option when using a large number of ClusterPodMonitoring
and/or PodMonitoring.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.ExportFilters">
<span id="ExportFilters">ExportFilters
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.CollectionSpec">CollectionSpec</a>)
</p>
<div>
<p>ExportFilters provides mechanisms to filter the scraped data that&rsquo;s sent to GMP.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>matchOneOf</code><br/>
<em>
[]string
</em>
</td>
<td>
<p>A list of Prometheus time series matchers. Every time series must match at least one
of the matchers to be exported. This field can be used equivalently to the match[]
parameter of the Prometheus federation endpoint to selectively export data.
Example: <code>[&quot;{job!='foobar'}&quot;, &quot;{__name__!~'container_foo.*|container_bar.*'}&quot;]</code></p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.ExportSpec">
<span id="ExportSpec">ExportSpec
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.OperatorConfig">OperatorConfig</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>url</code><br/>
<em>
string
</em>
</td>
<td>
<p>The URL of the endpoint that supports Prometheus Remote Write to export samples to.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.GlobalRules">
<span id="GlobalRules">GlobalRules
</span>
</h3>
<div>
<p>GlobalRules defines Prometheus alerting and recording rules that are scoped
to all data in the queried project.
If the project_id or location labels are not preserved by the rule, they default to
the values of the cluster.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.RulesSpec">
RulesSpec
</a>
</em>
</td>
<td>
<p>Specification of rules to record and alert on.</p>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.RulesStatus">
RulesStatus
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Most recently observed status of the resource.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.HTTPClientConfig">
<span id="HTTPClientConfig">HTTPClientConfig
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.ScrapeEndpoint">ScrapeEndpoint</a>)
</p>
<div>
<p>HTTPClientConfig stores HTTP-client configurations.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>authorization</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.Auth">
Auth
</a>
</em>
</td>
<td>
<p>The HTTP authorization credentials for the targets.</p>
</td>
</tr>
<tr>
<td>
<code>basicAuth</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.BasicAuth">
BasicAuth
</a>
</em>
</td>
<td>
<p>The HTTP basic authentication credentials for the targets.</p>
</td>
</tr>
<tr>
<td>
<code>tls</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.TLS">
TLS
</a>
</em>
</td>
<td>
<p>Configures the scrape request&rsquo;s TLS settings.</p>
</td>
</tr>
<tr>
<td>
<code>oauth2</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.OAuth2">
OAuth2
</a>
</em>
</td>
<td>
<p>The OAuth2 client credentials used to fetch a token for the targets.</p>
</td>
</tr>
<tr>
<td>
<code>ProxyConfig</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.ProxyConfig">
ProxyConfig
</a>
</em>
</td>
<td>
<p>
(Members of <code>ProxyConfig</code> are embedded into this type.)
</p>
<p>Proxy configuration.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.KubeletScraping">
<span id="KubeletScraping">KubeletScraping
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.CollectionSpec">CollectionSpec</a>)
</p>
<div>
<p>KubeletScraping allows enabling scraping of the Kubelets&rsquo; metric endpoints.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>interval</code><br/>
<em>
string
</em>
</td>
<td>
<p>The interval at which the metric endpoints are scraped.</p>
</td>
</tr>
<tr>
<td>
<code>tlsInsecureSkipVerify</code><br/>
<em>
bool
</em>
</td>
<td>
<p>TLSInsecureSkipVerify disables verifying the target cert.
This can be useful for clusters provisioned with kubeadm.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.LabelMapping">
<span id="LabelMapping">LabelMapping
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.TargetLabels">TargetLabels</a>)
</p>
<div>
<p>LabelMapping specifies how to transfer a label from a Kubernetes resource
onto a Prometheus target.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>from</code><br/>
<em>
string
</em>
</td>
<td>
<p>Kubernetes resource label to remap.</p>
</td>
</tr>
<tr>
<td>
<code>to</code><br/>
<em>
string
</em>
</td>
<td>
<p>Remapped Prometheus target label.
Defaults to the same name as <code>From</code>.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.ManagedAlertmanagerSpec">
<span id="ManagedAlertmanagerSpec">ManagedAlertmanagerSpec
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.OperatorConfig">OperatorConfig</a>)
</p>
<div>
<p>ManagedAlertmanagerSpec holds configuration information for the managed
Alertmanager instance.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>configSecret</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#secretkeyselector-v1-core">
Kubernetes core/v1.SecretKeySelector
</a>
</em>
</td>
<td>
<p>ConfigSecret refers to the name of a single-key Secret in the public namespace that
holds the managed Alertmanager config file.</p>
</td>
</tr>
<tr>
<td>
<code>externalURL</code><br/>
<em>
string
</em>
</td>
<td>
<p>ExternalURL is the URL under which Alertmanager is externally reachable
(for example, if Alertmanager is served via a reverse proxy).
Used for generating relative and absolute links back to Alertmanager
itself. If the URL has a path portion, it will be used to prefix all HTTP
endpoints served by Alertmanager.
If omitted, relevant URL components will be derived automatically.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.MonitoringCRD">
<span id="MonitoringCRD">MonitoringCRD
</span>
</h3>
<div>
</div>
<h3 id="monitoring.googleapis.com/v1.MonitoringCondition">
<span id="MonitoringCondition">MonitoringCondition
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.MonitoringStatus">MonitoringStatus</a>)
</p>
<div>
<p>MonitoringCondition describes the condition of a PodMonitoring.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>type</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.MonitoringConditionType">
MonitoringConditionType
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#conditionstatus-v1-core">
Kubernetes core/v1.ConditionStatus
</a>
</em>
</td>
<td>
<p>Status of the condition, one of True, False, Unknown.</p>
</td>
</tr>
<tr>
<td>
<code>lastUpdateTime</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#time-v1-meta">
Kubernetes meta/v1.Time
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The last time this condition was updated.</p>
</td>
</tr>
<tr>
<td>
<code>lastTransitionTime</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#time-v1-meta">
Kubernetes meta/v1.Time
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Last time the condition transitioned from one status to another.</p>
</td>
</tr>
<tr>
<td>
<code>reason</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The reason for the condition&rsquo;s last transition.</p>
</td>
</tr>
<tr>
<td>
<code>message</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>A human-readable message indicating details about the transition.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.MonitoringConditionType">
<span id="MonitoringConditionType">MonitoringConditionType
(<code>string</code> alias)</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.MonitoringCondition">MonitoringCondition</a>)
</p>
<div>
<p>MonitoringConditionType is the type of MonitoringCondition.</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;ConfigurationCreateSuccess&#34;</p></td>
<td><p>ConfigurationCreateSuccess indicates that the config generated from the
monitoring resource was created successfully.</p>
</td>
</tr></tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.MonitoringStatus">
<span id="MonitoringStatus">MonitoringStatus
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.ClusterNodeMonitoring">ClusterNodeMonitoring</a>, <a href="#monitoring.googleapis.com/v1.PodMonitoringStatus">PodMonitoringStatus</a>)
</p>
<div>
<p>MonitoringStatus holds status information of a monitoring resource.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>observedGeneration</code><br/>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>The generation observed by the controller.</p>
</td>
</tr>
<tr>
<td>
<code>conditions</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.MonitoringCondition">
[]MonitoringCondition
</a>
</em>
</td>
<td>
<p>Represents the latest available observations of a podmonitor&rsquo;s current state.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.OAuth2">
<span id="OAuth2">OAuth2
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.HTTPClientConfig">HTTPClientConfig</a>)
</p>
<div>
<p>OAuth2 is the OAuth2 client configuration.</p>
<p>Currently the client secret is not configurable and always empty.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>clientID</code><br/>
<em>
string
</em>
</td>
<td>
<p>Public identifier for the client.</p>
</td>
</tr>
<tr>
<td>
<code>scopes</code><br/>
<em>
[]string
</em>
</td>
<td>
<p>Scopes for the token request.</p>
</td>
</tr>
<tr>
<td>
<code>tokenURL</code><br/>
<em>
string
</em>
</td>
<td>
<p>The URL to fetch the token from.</p>
</td>
</tr>
<tr>
<td>
<code>endpointParams</code><br/>
<em>
map[string]string
</em>
</td>
<td>
<p>Optional parameters to append to the token URL.</p>
</td>
</tr>
<tr>
<td>
<code>tlsConfig</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.TLS">
TLS
</a>
</em>
</td>
<td>
<p>Configures the token request&rsquo;s TLS settings.</p>
</td>
</tr>
<tr>
<td>
<code>ProxyConfig</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.ProxyConfig">
ProxyConfig
</a>
</em>
</td>
<td>
<p>
(Members of <code>ProxyConfig</code> are embedded into this type.)
</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.OperatorConfig">
<span id="OperatorConfig">OperatorConfig
</span>
</h3>
<div>
<p>OperatorConfig defines configuration of the gmp-operator.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>rules</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.RuleEvaluatorSpec">
RuleEvaluatorSpec
</a>
</em>
</td>
<td>
<p>Rules specifies how the operator configures and deploys rule-evaluator.</p>
</td>
</tr>
<tr>
<td>
<code>collection</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.CollectionSpec">
CollectionSpec
</a>
</em>
</td>
<td>
<p>Collection specifies how the operator configures collection, including
scraping and an integrated export to Google Cloud Monitoring.</p>
</td>
</tr>
<tr>
<td>
<code>exports</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.ExportSpec">
[]ExportSpec
</a>
</em>
</td>
<td>
<p>Exports is an EXPERIMENTAL feature that specifies additional, optional endpoints to export to,
on top of Google Cloud Monitoring collection.
Note: To disable integrated export to Google Cloud Monitoring specify a non-matching filter in the &ldquo;collection.filter&rdquo; field.</p>
</td>
</tr>
<tr>
<td>
<code>managedAlertmanager</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.ManagedAlertmanagerSpec">
ManagedAlertmanagerSpec
</a>
</em>
</td>
<td>
<p>ManagedAlertmanager holds information for configuring the managed instance of Alertmanager.</p>
</td>
</tr>
<tr>
<td>
<code>features</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.OperatorFeatures">
OperatorFeatures
</a>
</em>
</td>
<td>
<p>Features holds configuration for optional managed-collection features.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.OperatorFeatures">
<span id="OperatorFeatures">OperatorFeatures
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.OperatorConfig">OperatorConfig</a>)
</p>
<div>
<p>OperatorFeatures holds configuration for optional managed-collection features.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>targetStatus</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.TargetStatusSpec">
TargetStatusSpec
</a>
</em>
</td>
<td>
<p>Configuration of target status reporting.</p>
</td>
</tr>
<tr>
<td>
<code>config</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.ConfigSpec">
ConfigSpec
</a>
</em>
</td>
<td>
<p>Settings for the collector configuration propagation.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.PodMonitoring">
<span id="PodMonitoring">PodMonitoring
</span>
</h3>
<div>
<p>PodMonitoring defines monitoring for a set of pods, scoped to pods
within the PodMonitoring&rsquo;s namespace.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.PodMonitoringSpec">
PodMonitoringSpec
</a>
</em>
</td>
<td>
<p>Specification of desired Pod selection for target discovery by
Prometheus.</p>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.PodMonitoringStatus">
PodMonitoringStatus
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Most recently observed status of the resource.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.PodMonitoringCRD">
<span id="PodMonitoringCRD">PodMonitoringCRD
</span>
</h3>
<div>
<p>PodMonitoringCRD represents a Kubernetes CRD that monitors Pod endpoints.</p>
</div>
<h3 id="monitoring.googleapis.com/v1.PodMonitoringSpec">
<span id="PodMonitoringSpec">PodMonitoringSpec
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.PodMonitoring">PodMonitoring</a>)
</p>
<div>
<p>PodMonitoringSpec contains specification parameters for PodMonitoring.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>selector</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#labelselector-v1-meta">
Kubernetes meta/v1.LabelSelector
</a>
</em>
</td>
<td>
<p>Label selector that specifies which pods are selected for this monitoring
configuration.</p>
</td>
</tr>
<tr>
<td>
<code>endpoints</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.ScrapeEndpoint">
[]ScrapeEndpoint
</a>
</em>
</td>
<td>
<p>The endpoints to scrape on the selected pods.</p>
</td>
</tr>
<tr>
<td>
<code>targetLabels</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.TargetLabels">
TargetLabels
</a>
</em>
</td>
<td>
<p>Labels to add to the Prometheus target for discovered endpoints.
The <code>instance</code> label is always set to <code>&lt;pod_name&gt;:&lt;port&gt;</code> or <code>&lt;node_name&gt;:&lt;port&gt;</code>
if the scraped pod is controlled by a DaemonSet.</p>
</td>
</tr>
<tr>
<td>
<code>limits</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.ScrapeLimits">
ScrapeLimits
</a>
</em>
</td>
<td>
<p>Limits to apply at scrape time.</p>
</td>
</tr>
<tr>
<td>
<code>filterRunning</code><br/>
<em>
bool
</em>
</td>
<td>
<p>FilterRunning will drop any pods that are in the &ldquo;Failed&rdquo; or &ldquo;Succeeded&rdquo;
pod lifecycle.
See: <a href="https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-phase">https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-phase</a></p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.PodMonitoringStatus">
<span id="PodMonitoringStatus">PodMonitoringStatus
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.ClusterPodMonitoring">ClusterPodMonitoring</a>, <a href="#monitoring.googleapis.com/v1.PodMonitoring">PodMonitoring</a>)
</p>
<div>
<p>PodMonitoringStatus holds status information of a PodMonitoring resource.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>MonitoringStatus</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.MonitoringStatus">
MonitoringStatus
</a>
</em>
</td>
<td>
<p>
(Members of <code>MonitoringStatus</code> are embedded into this type.)
</p>
</td>
</tr>
<tr>
<td>
<code>endpointStatuses</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.ScrapeEndpointStatus">
[]ScrapeEndpointStatus
</a>
</em>
</td>
<td>
<p>Represents the latest available observations of target state for each ScrapeEndpoint.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.ProxyConfig">
<span id="ProxyConfig">ProxyConfig
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.HTTPClientConfig">HTTPClientConfig</a>, <a href="#monitoring.googleapis.com/v1.OAuth2">OAuth2</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>proxyUrl</code><br/>
<em>
string
</em>
</td>
<td>
<p>HTTP proxy server to use to connect to the targets. Encoded passwords are not supported.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.RelabelingRule">
<span id="RelabelingRule">RelabelingRule
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.ScrapeEndpoint">ScrapeEndpoint</a>, <a href="#monitoring.googleapis.com/v1.ScrapeNodeEndpoint">ScrapeNodeEndpoint</a>)
</p>
<div>
<p>RelabelingRule defines a single Prometheus relabeling rule.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>sourceLabels</code><br/>
<em>
[]string
</em>
</td>
<td>
<p>The source labels select values from existing labels. Their content is concatenated
using the configured separator and matched against the configured regular expression
for the replace, keep, and drop actions.</p>
</td>
</tr>
<tr>
<td>
<code>separator</code><br/>
<em>
string
</em>
</td>
<td>
<p>Separator placed between concatenated source label values. Defaults to &lsquo;;&rsquo;.</p>
</td>
</tr>
<tr>
<td>
<code>targetLabel</code><br/>
<em>
string
</em>
</td>
<td>
<p>Label to which the resulting value is written in a replace action.
It is mandatory for replace actions. Regex capture groups are available.</p>
</td>
</tr>
<tr>
<td>
<code>regex</code><br/>
<em>
string
</em>
</td>
<td>
<p>Regular expression against which the extracted value is matched. Defaults to &lsquo;(.*)&rsquo;.</p>
</td>
</tr>
<tr>
<td>
<code>modulus</code><br/>
<em>
uint64
</em>
</td>
<td>
<p>Modulus to take of the hash of the source label values.</p>
</td>
</tr>
<tr>
<td>
<code>replacement</code><br/>
<em>
string
</em>
</td>
<td>
<p>Replacement value against which a regex replace is performed if the
regular expression matches. Regex capture groups are available. Defaults to &lsquo;$1&rsquo;.</p>
</td>
</tr>
<tr>
<td>
<code>action</code><br/>
<em>
string
</em>
</td>
<td>
<p>Action to perform based on regex matching. Defaults to &lsquo;replace&rsquo;.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.Rule">
<span id="Rule">Rule
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.RuleGroup">RuleGroup</a>)
</p>
<div>
<p>Rule is a single rule in the Prometheus format:
<a href="https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/">https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/</a></p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>record</code><br/>
<em>
string
</em>
</td>
<td>
<p>Record the result of the expression to this metric name.
Only one of <code>record</code> and <code>alert</code> must be set.</p>
</td>
</tr>
<tr>
<td>
<code>alert</code><br/>
<em>
string
</em>
</td>
<td>
<p>Name of the alert to evaluate the expression as.
Only one of <code>record</code> and <code>alert</code> must be set.</p>
</td>
</tr>
<tr>
<td>
<code>expr</code><br/>
<em>
string
</em>
</td>
<td>
<p>The PromQL expression to evaluate.</p>
</td>
</tr>
<tr>
<td>
<code>for</code><br/>
<em>
string
</em>
</td>
<td>
<p>The duration to wait before a firing alert produced by this rule is sent to Alertmanager.
Only valid if <code>alert</code> is set.</p>
</td>
</tr>
<tr>
<td>
<code>labels</code><br/>
<em>
map[string]string
</em>
</td>
<td>
<p>A set of labels to attach to the result of the query expression.</p>
</td>
</tr>
<tr>
<td>
<code>annotations</code><br/>
<em>
map[string]string
</em>
</td>
<td>
<p>A set of annotations to attach to alerts produced by the query expression.
Only valid if <code>alert</code> is set.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.RuleEvaluatorSpec">
<span id="RuleEvaluatorSpec">RuleEvaluatorSpec
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.OperatorConfig">OperatorConfig</a>)
</p>
<div>
<p>RuleEvaluatorSpec defines configuration for deploying rule-evaluator.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>externalLabels</code><br/>
<em>
map[string]string
</em>
</td>
<td>
<p>ExternalLabels specifies external labels that are attached to any rule
results and alerts produced by rules. The precedence behavior matches that
of Prometheus.</p>
</td>
</tr>
<tr>
<td>
<code>queryProjectID</code><br/>
<em>
string
</em>
</td>
<td>
<p>QueryProjectID is the GCP project ID to evaluate rules against.
If left blank, the rule-evaluator will try attempt to infer the Project ID
from the environment.</p>
</td>
</tr>
<tr>
<td>
<code>generatorUrl</code><br/>
<em>
string
</em>
</td>
<td>
<p>The base URL used for the generator URL in the alert notification payload.
Should point to an instance of a query frontend that gives access to queryProjectID.</p>
</td>
</tr>
<tr>
<td>
<code>alerting</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.AlertingSpec">
AlertingSpec
</a>
</em>
</td>
<td>
<p>Alerting contains how the rule-evaluator configures alerting.</p>
</td>
</tr>
<tr>
<td>
<code>credentials</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#secretkeyselector-v1-core">
Kubernetes core/v1.SecretKeySelector
</a>
</em>
</td>
<td>
<p>A reference to GCP service account credentials with which the rule
evaluator container is run. It needs to have metric read permissions
against queryProjectId and metric write permissions against all projects
to which rule results are written.
Within GKE, this can typically be left empty if the compute default
service account has the required permissions.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.RuleGroup">
<span id="RuleGroup">RuleGroup
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.RulesSpec">RulesSpec</a>)
</p>
<div>
<p>RuleGroup declares rules in the Prometheus format:
<a href="https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/">https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/</a></p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<p>The name of the rule group.</p>
</td>
</tr>
<tr>
<td>
<code>interval</code><br/>
<em>
string
</em>
</td>
<td>
<p>The interval at which to evaluate the rules. Must be a valid Prometheus duration.</p>
</td>
</tr>
<tr>
<td>
<code>rules</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.Rule">
[]Rule
</a>
</em>
</td>
<td>
<p>A list of rules that are executed sequentially as part of this group.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.Rules">
<span id="Rules">Rules
</span>
</h3>
<div>
<p>Rules defines Prometheus alerting and recording rules that are scoped
to the namespace of the resource. Only metric data from this namespace is processed
and all rule results have their project_id, cluster, and namespace label preserved
for query processing.
If the location label is not preserved by the rule, it defaults to the cluster&rsquo;s location.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.RulesSpec">
RulesSpec
</a>
</em>
</td>
<td>
<p>Specification of rules to record and alert on.</p>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.RulesStatus">
RulesStatus
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Most recently observed status of the resource.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.RulesSpec">
<span id="RulesSpec">RulesSpec
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.ClusterRules">ClusterRules</a>, <a href="#monitoring.googleapis.com/v1.GlobalRules">GlobalRules</a>, <a href="#monitoring.googleapis.com/v1.Rules">Rules</a>)
</p>
<div>
<p>RulesSpec contains specification parameters for a Rules resource.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>groups</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.RuleGroup">
[]RuleGroup
</a>
</em>
</td>
<td>
<p>A list of Prometheus rule groups.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.RulesStatus">
<span id="RulesStatus">RulesStatus
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.ClusterRules">ClusterRules</a>, <a href="#monitoring.googleapis.com/v1.GlobalRules">GlobalRules</a>, <a href="#monitoring.googleapis.com/v1.Rules">Rules</a>)
</p>
<div>
<p>RulesStatus contains status information for a Rules resource.</p>
</div>
<h3 id="monitoring.googleapis.com/v1.SampleGroup">
<span id="SampleGroup">SampleGroup
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.ScrapeEndpointStatus">ScrapeEndpointStatus</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>sampleTargets</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.SampleTarget">
[]SampleTarget
</a>
</em>
</td>
<td>
<p>Targets emitting the error message.</p>
</td>
</tr>
<tr>
<td>
<code>count</code><br/>
<em>
int32
</em>
</td>
<td>
<em>(Optional)</em>
<p>Total count of similar errors.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.SampleTarget">
<span id="SampleTarget">SampleTarget
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.SampleGroup">SampleGroup</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>labels</code><br/>
<em>
github.com/prometheus/common/model.LabelSet
</em>
</td>
<td>
<p>The label set, keys and values, of the target.</p>
</td>
</tr>
<tr>
<td>
<code>lastError</code><br/>
<em>
string
</em>
</td>
<td>
<p>Error message.</p>
</td>
</tr>
<tr>
<td>
<code>lastScrapeDurationSeconds</code><br/>
<em>
string
</em>
</td>
<td>
<p>Scrape duration in seconds.</p>
</td>
</tr>
<tr>
<td>
<code>health</code><br/>
<em>
string
</em>
</td>
<td>
<p>Health status.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.ScrapeEndpoint">
<span id="ScrapeEndpoint">ScrapeEndpoint
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.ClusterPodMonitoringSpec">ClusterPodMonitoringSpec</a>, <a href="#monitoring.googleapis.com/v1.PodMonitoringSpec">PodMonitoringSpec</a>)
</p>
<div>
<p>ScrapeEndpoint specifies a Prometheus metrics endpoint to scrape.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>port</code><br/>
<em>
k8s.io/apimachinery/pkg/util/intstr.IntOrString
</em>
</td>
<td>
<p>Name or number of the port to scrape.
The container metadata label is only populated if the port is referenced by name
because port numbers are not unique across containers.</p>
</td>
</tr>
<tr>
<td>
<code>scheme</code><br/>
<em>
string
</em>
</td>
<td>
<p>Protocol scheme to use to scrape.</p>
</td>
</tr>
<tr>
<td>
<code>path</code><br/>
<em>
string
</em>
</td>
<td>
<p>HTTP path to scrape metrics from. Defaults to &ldquo;/metrics&rdquo;.</p>
</td>
</tr>
<tr>
<td>
<code>params</code><br/>
<em>
map[string][]string
</em>
</td>
<td>
<p>HTTP GET params to use when scraping.</p>
</td>
</tr>
<tr>
<td>
<code>interval</code><br/>
<em>
string
</em>
</td>
<td>
<p>Interval at which to scrape metrics. Must be a valid Prometheus duration.</p>
</td>
</tr>
<tr>
<td>
<code>timeout</code><br/>
<em>
string
</em>
</td>
<td>
<p>Timeout for metrics scrapes. Must be a valid Prometheus duration.
Must not be larger than the scrape interval.</p>
</td>
</tr>
<tr>
<td>
<code>metricRelabeling</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.RelabelingRule">
[]RelabelingRule
</a>
</em>
</td>
<td>
<p>Relabeling rules for metrics scraped from this endpoint. Relabeling rules that
override protected target labels (project_id, location, cluster, namespace, job,
instance, or <strong>address</strong>) are not permitted. The labelmap action is not permitted
in general.</p>
</td>
</tr>
<tr>
<td>
<code>HTTPClientConfig</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.HTTPClientConfig">
HTTPClientConfig
</a>
</em>
</td>
<td>
<p>
(Members of <code>HTTPClientConfig</code> are embedded into this type.)
</p>
<p>Prometheus HTTP client configuration.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.ScrapeEndpointStatus">
<span id="ScrapeEndpointStatus">ScrapeEndpointStatus
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.PodMonitoringStatus">PodMonitoringStatus</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<p>The name of the ScrapeEndpoint.</p>
</td>
</tr>
<tr>
<td>
<code>activeTargets</code><br/>
<em>
int64
</em>
</td>
<td>
<p>Total number of active targets.</p>
</td>
</tr>
<tr>
<td>
<code>unhealthyTargets</code><br/>
<em>
int64
</em>
</td>
<td>
<p>Total number of active, unhealthy targets.</p>
</td>
</tr>
<tr>
<td>
<code>lastUpdateTime</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#time-v1-meta">
Kubernetes meta/v1.Time
</a>
</em>
</td>
<td>
<p>Last time this status was updated.</p>
</td>
</tr>
<tr>
<td>
<code>sampleGroups</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.SampleGroup">
[]SampleGroup
</a>
</em>
</td>
<td>
<p>A fixed sample of targets grouped by error type.</p>
</td>
</tr>
<tr>
<td>
<code>collectorsFraction</code><br/>
<em>
string
</em>
</td>
<td>
<p>Fraction of collectors included in status, bounded [0,1].
Ideally, this should always be 1. Anything less can
be considered a problem and should be investigated.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.ScrapeLimits">
<span id="ScrapeLimits">ScrapeLimits
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.ClusterNodeMonitoringSpec">ClusterNodeMonitoringSpec</a>, <a href="#monitoring.googleapis.com/v1.ClusterPodMonitoringSpec">ClusterPodMonitoringSpec</a>, <a href="#monitoring.googleapis.com/v1.PodMonitoringSpec">PodMonitoringSpec</a>)
</p>
<div>
<p>ScrapeLimits limits applied to scraped targets.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>samples</code><br/>
<em>
uint64
</em>
</td>
<td>
<p>Maximum number of samples accepted within a single scrape.
Uses Prometheus default if left unspecified.</p>
</td>
</tr>
<tr>
<td>
<code>labels</code><br/>
<em>
uint64
</em>
</td>
<td>
<p>Maximum number of labels accepted for a single sample.
Uses Prometheus default if left unspecified.</p>
</td>
</tr>
<tr>
<td>
<code>labelNameLength</code><br/>
<em>
uint64
</em>
</td>
<td>
<p>Maximum label name length.
Uses Prometheus default if left unspecified.</p>
</td>
</tr>
<tr>
<td>
<code>labelValueLength</code><br/>
<em>
uint64
</em>
</td>
<td>
<p>Maximum label value length.
Uses Prometheus default if left unspecified.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.ScrapeNodeEndpoint">
<span id="ScrapeNodeEndpoint">ScrapeNodeEndpoint
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.ClusterNodeMonitoringSpec">ClusterNodeMonitoringSpec</a>)
</p>
<div>
<p>ScrapeNodeEndpoint specifies a Prometheus metrics endpoint on a node to scrape.
It contains all the fields used in the ScrapeEndpoint except for port and HTTPClientConfig.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>scheme</code><br/>
<em>
string
</em>
</td>
<td>
<p>Protocol scheme to use to scrape.</p>
</td>
</tr>
<tr>
<td>
<code>path</code><br/>
<em>
string
</em>
</td>
<td>
<p>HTTP path to scrape metrics from. Defaults to &ldquo;/metrics&rdquo;.</p>
</td>
</tr>
<tr>
<td>
<code>params</code><br/>
<em>
map[string][]string
</em>
</td>
<td>
<p>HTTP GET params to use when scraping.</p>
</td>
</tr>
<tr>
<td>
<code>interval</code><br/>
<em>
string
</em>
</td>
<td>
<p>Interval at which to scrape metrics. Must be a valid Prometheus duration.</p>
</td>
</tr>
<tr>
<td>
<code>timeout</code><br/>
<em>
string
</em>
</td>
<td>
<p>Timeout for metrics scrapes. Must be a valid Prometheus duration.
Must not be larger then the scrape interval.</p>
</td>
</tr>
<tr>
<td>
<code>metricRelabeling</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.RelabelingRule">
[]RelabelingRule
</a>
</em>
</td>
<td>
<p>Relabeling rules for metrics scraped from this endpoint. Relabeling rules that
override protected target labels (project_id, location, cluster, namespace, job,
instance, or <strong>address</strong>) are not permitted. The labelmap action is not permitted
in general.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.SecretOrConfigMap">
<span id="SecretOrConfigMap">SecretOrConfigMap
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.TLSConfig">TLSConfig</a>)
</p>
<div>
<p>SecretOrConfigMap allows to specify data as a Secret or ConfigMap. Fields are mutually exclusive.
Taking inspiration from prometheus-operator: <a href="https://github.com/prometheus-operator/prometheus-operator/blob/2c81b0cf6a5673e08057499a08ddce396b19dda4/Documentation/api.md#secretorconfigmap">https://github.com/prometheus-operator/prometheus-operator/blob/2c81b0cf6a5673e08057499a08ddce396b19dda4/Documentation/api.md#secretorconfigmap</a></p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>secret</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#secretkeyselector-v1-core">
Kubernetes core/v1.SecretKeySelector
</a>
</em>
</td>
<td>
<p>Secret containing data to use for the targets.</p>
</td>
</tr>
<tr>
<td>
<code>configMap</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#configmapkeyselector-v1-core">
Kubernetes core/v1.ConfigMapKeySelector
</a>
</em>
</td>
<td>
<p>ConfigMap containing data to use for the targets.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.TLS">
<span id="TLS">TLS
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.HTTPClientConfig">HTTPClientConfig</a>, <a href="#monitoring.googleapis.com/v1.OAuth2">OAuth2</a>)
</p>
<div>
<p>TLS specifies TLS configuration parameters from Kubernetes resources.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>serverName</code><br/>
<em>
string
</em>
</td>
<td>
<p>Used to verify the hostname for the targets.</p>
</td>
</tr>
<tr>
<td>
<code>insecureSkipVerify</code><br/>
<em>
bool
</em>
</td>
<td>
<p>Disable target certificate validation.</p>
</td>
</tr>
<tr>
<td>
<code>minVersion</code><br/>
<em>
string
</em>
</td>
<td>
<p>Minimum TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS 1.1), TLS12 (TLS 1.2), TLS13 (TLS 1.3).
If unset, Prometheus will use Go default minimum version, which is TLS 1.2.
See MinVersion in <a href="https://pkg.go.dev/crypto/tls#Config">https://pkg.go.dev/crypto/tls#Config</a>.</p>
</td>
</tr>
<tr>
<td>
<code>maxVersion</code><br/>
<em>
string
</em>
</td>
<td>
<p>Maximum TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS 1.1), TLS12 (TLS 1.2), TLS13 (TLS 1.3).
If unset, Prometheus will use Go default minimum version, which is TLS 1.2.
See MinVersion in <a href="https://pkg.go.dev/crypto/tls#Config">https://pkg.go.dev/crypto/tls#Config</a>.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.TLSConfig">
<span id="TLSConfig">TLSConfig
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.AlertmanagerEndpoints">AlertmanagerEndpoints</a>)
</p>
<div>
<p>TLSConfig specifies TLS configuration parameters from Kubernetes resources.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>ca</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.SecretOrConfigMap">
SecretOrConfigMap
</a>
</em>
</td>
<td>
<p>Struct containing the CA cert to use for the targets.</p>
</td>
</tr>
<tr>
<td>
<code>cert</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.SecretOrConfigMap">
SecretOrConfigMap
</a>
</em>
</td>
<td>
<p>Struct containing the client cert file for the targets.</p>
</td>
</tr>
<tr>
<td>
<code>keySecret</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#secretkeyselector-v1-core">
Kubernetes core/v1.SecretKeySelector
</a>
</em>
</td>
<td>
<p>Secret containing the client key file for the targets.</p>
</td>
</tr>
<tr>
<td>
<code>serverName</code><br/>
<em>
string
</em>
</td>
<td>
<p>Used to verify the hostname for the targets.</p>
</td>
</tr>
<tr>
<td>
<code>insecureSkipVerify</code><br/>
<em>
bool
</em>
</td>
<td>
<p>Disable target certificate validation.</p>
</td>
</tr>
<tr>
<td>
<code>minVersion</code><br/>
<em>
string
</em>
</td>
<td>
<p>Minimum TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS 1.1), TLS12 (TLS 1.2), TLS13 (TLS 1.3).
If unset, Prometheus will use Go default minimum version, which is TLS 1.2.
See MinVersion in <a href="https://pkg.go.dev/crypto/tls#Config">https://pkg.go.dev/crypto/tls#Config</a>.</p>
</td>
</tr>
<tr>
<td>
<code>maxVersion</code><br/>
<em>
string
</em>
</td>
<td>
<p>Maximum TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS 1.1), TLS12 (TLS 1.2), TLS13 (TLS 1.3).
If unset, Prometheus will use Go default minimum version, which is TLS 1.2.
See MinVersion in <a href="https://pkg.go.dev/crypto/tls#Config">https://pkg.go.dev/crypto/tls#Config</a>.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.TargetLabels">
<span id="TargetLabels">TargetLabels
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.ClusterPodMonitoringSpec">ClusterPodMonitoringSpec</a>, <a href="#monitoring.googleapis.com/v1.PodMonitoringSpec">PodMonitoringSpec</a>)
</p>
<div>
<p>TargetLabels configures labels for the discovered Prometheus targets.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadata</code><br/>
<em>
[]string
</em>
</td>
<td>
<p>Pod metadata labels that are set on all scraped targets.
Permitted keys are <code>pod</code>, <code>container</code>, and <code>node</code> for PodMonitoring and
<code>pod</code>, <code>container</code>, <code>node</code>, and <code>namespace</code> for ClusterPodMonitoring. The <code>container</code>
label is only populated if the scrape port is referenced by name.
Defaults to [pod, container] for PodMonitoring and [namespace, pod, container]
for ClusterPodMonitoring.
If set to null, it will be interpreted as the empty list for PodMonitoring
and to [namespace] for ClusterPodMonitoring. This is for backwards-compatibility
only.</p>
</td>
</tr>
<tr>
<td>
<code>fromPod</code><br/>
<em>
<a href="#monitoring.googleapis.com/v1.LabelMapping">
[]LabelMapping
</a>
</em>
</td>
<td>
<p>Labels to transfer from the Kubernetes Pod to Prometheus target labels.
Mappings are applied in order.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="monitoring.googleapis.com/v1.TargetStatusSpec">
<span id="TargetStatusSpec">TargetStatusSpec
</span>
</h3>
<p>
(<em>Appears in: </em><a href="#monitoring.googleapis.com/v1.OperatorFeatures">OperatorFeatures</a>)
</p>
<div>
<p>TargetStatusSpec holds configuration for target status reporting.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>enabled</code><br/>
<em>
bool
</em>
</td>
<td>
<p>Enable target status reporting.</p>
</td>
</tr>
</tbody>
</table>
<hr/>
<p><em>
Generated with <code>gen-crd-api-reference-docs</code>
</em></p>
