---
title: "API"
description: "Generated API docs for the GMP CRDs"
lead: ""
date: 2021-03-08T08:49:31+00:00
draft: false
images: []
menu:
  docs:
    parent: "operator"
weight: 1000
toc: true
---

This Document documents the types introduced by the GMP CRDs to be consumed by users.

> Note this document is generated from code comments. When contributing a change to this document please do so by changing the code comments.

## Table of Contents
* [AlertingSpec](#alertingspec)
* [AlertmanagerEndpoints](#alertmanagerendpoints)
* [Authorization](#authorization)
* [ClusterPodMonitoring](#clusterpodmonitoring)
* [ClusterPodMonitoringList](#clusterpodmonitoringlist)
* [ClusterPodMonitoringSpec](#clusterpodmonitoringspec)
* [ClusterRules](#clusterrules)
* [ClusterRulesList](#clusterruleslist)
* [CollectionSpec](#collectionspec)
* [ConfigSpec](#configspec)
* [ExportFilters](#exportfilters)
* [GlobalRules](#globalrules)
* [GlobalRulesList](#globalruleslist)
* [HTTPClientConfig](#httpclientconfig)
* [KubeletScraping](#kubeletscraping)
* [LabelMapping](#labelmapping)
* [ManagedAlertmanagerSpec](#managedalertmanagerspec)
* [MonitoringCondition](#monitoringcondition)
* [OperatorConfig](#operatorconfig)
* [OperatorConfigList](#operatorconfiglist)
* [OperatorFeatures](#operatorfeatures)
* [PodMonitoring](#podmonitoring)
* [PodMonitoringList](#podmonitoringlist)
* [PodMonitoringSpec](#podmonitoringspec)
* [PodMonitoringStatus](#podmonitoringstatus)
* [RelabelingRule](#relabelingrule)
* [Rule](#rule)
* [RuleEvaluatorSpec](#ruleevaluatorspec)
* [RuleGroup](#rulegroup)
* [Rules](#rules)
* [RulesList](#ruleslist)
* [RulesSpec](#rulesspec)
* [SampleGroup](#samplegroup)
* [SampleTarget](#sampletarget)
* [ScrapeEndpoint](#scrapeendpoint)
* [ScrapeEndpointStatus](#scrapeendpointstatus)
* [ScrapeLimits](#scrapelimits)
* [SecretOrConfigMap](#secretorconfigmap)
* [TLS](#tls)
* [TLSConfig](#tlsconfig)
* [TargetLabels](#targetlabels)
* [TargetStatusSpec](#targetstatusspec)

## AlertingSpec

AlertingSpec defines alerting configuration.


<em>appears in: [RuleEvaluatorSpec](#ruleevaluatorspec)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| alertmanagers | Alertmanagers contains endpoint configuration for designated Alertmanagers. | [][AlertmanagerEndpoints](#alertmanagerendpoints) | false |

[Back to TOC](#table-of-contents)

## AlertmanagerEndpoints

AlertmanagerEndpoints defines a selection of a single Endpoints object containing alertmanager IPs to fire alerts against.


<em>appears in: [AlertingSpec](#alertingspec)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| namespace | Namespace of Endpoints object. | string | true |
| name | Name of Endpoints object in Namespace. | string | true |
| port | Port the Alertmanager API is exposed on. | intstr.IntOrString | true |
| scheme | Scheme to use when firing alerts. | string | false |
| pathPrefix | Prefix for the HTTP path alerts are pushed to. | string | false |
| tls | TLS Config to use for alertmanager connection. | *[TLSConfig](#tlsconfig) | false |
| authorization | Authorization section for this alertmanager endpoint | *[Authorization](#authorization) | false |
| apiVersion | Version of the Alertmanager API that rule-evaluator uses to send alerts. It can be \"v1\" or \"v2\". | string | false |
| timeout | Timeout is a per-target Alertmanager timeout when pushing alerts. | string | false |

[Back to TOC](#table-of-contents)

## Authorization

Authorization specifies a subset of the Authorization struct, that is safe for use in Endpoints (no CredentialsFile field).


<em>appears in: [AlertmanagerEndpoints](#alertmanagerendpoints)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| type | Set the authentication type. Defaults to Bearer, Basic will cause an error | string | false |
| credentials | The secret's key that contains the credentials of the request | *[v1.SecretKeySelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#secretkeyselector-v1-core) | false |

[Back to TOC](#table-of-contents)

## ClusterPodMonitoring

ClusterPodMonitoring defines monitoring for a set of pods, scoped to all pods within the cluster.


<em>appears in: [ClusterPodMonitoringList](#clusterpodmonitoringlist)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#objectmeta-v1-meta) | false |
| spec | Specification of desired Pod selection for target discovery by Prometheus. | [ClusterPodMonitoringSpec](#clusterpodmonitoringspec) | true |
| status | Most recently observed status of the resource. | [PodMonitoringStatus](#podmonitoringstatus) | true |

[Back to TOC](#table-of-contents)

## ClusterPodMonitoringList

ClusterPodMonitoringList is a list of ClusterPodMonitorings.

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#listmeta-v1-meta) | false |
| items |  | [][ClusterPodMonitoring](#clusterpodmonitoring) | true |

[Back to TOC](#table-of-contents)

## ClusterPodMonitoringSpec

ClusterPodMonitoringSpec contains specification parameters for PodMonitoring.


<em>appears in: [ClusterPodMonitoring](#clusterpodmonitoring)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| selector | Label selector that specifies which pods are selected for this monitoring configuration. | [metav1.LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#labelselector-v1-meta) | true |
| endpoints | The endpoints to scrape on the selected pods. | [][ScrapeEndpoint](#scrapeendpoint) | true |
| targetLabels | Labels to add to the Prometheus target for discovered endpoints. The `instance` label is always set to `<pod_name>:<port>` or `<node_name>:<port>` if the scraped pod is controlled by a DaemonSet. | [TargetLabels](#targetlabels) | false |
| limits | Limits to apply at scrape time. | *[ScrapeLimits](#scrapelimits) | false |

[Back to TOC](#table-of-contents)

## ClusterRules

ClusterRules defines Prometheus alerting and recording rules that are scoped to the current cluster. Only metric data from the current cluster is processed and all rule results have their project_id and cluster label preserved for query processing. If the location label is not preserved by the rule, it defaults to the cluster's location.


<em>appears in: [ClusterRulesList](#clusterruleslist)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#objectmeta-v1-meta) | false |
| spec | Specification of rules to record and alert on. | [RulesSpec](#rulesspec) | true |
| status | Most recently observed status of the resource. | [RulesStatus](#rulesstatus) | true |

[Back to TOC](#table-of-contents)

## ClusterRulesList

ClusterRulesList is a list of ClusterRules.

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#listmeta-v1-meta) | false |
| items |  | [][ClusterRules](#clusterrules) | true |

[Back to TOC](#table-of-contents)

## CollectionSpec

CollectionSpec specifies how the operator configures collection of metric data.


<em>appears in: [OperatorConfig](#operatorconfig)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| externalLabels | ExternalLabels specifies external labels that are attached to all scraped data before being written to Cloud Monitoring. The precedence behavior matches that of Prometheus. | map[string]string | false |
| filter | Filter limits which metric data is sent to Cloud Monitoring. | [ExportFilters](#exportfilters) | false |
| credentials | A reference to GCP service account credentials with which Prometheus collectors are run. It needs to have metric write permissions for all project IDs to which data is written. Within GKE, this can typically be left empty if the compute default service account has the required permissions. | *[v1.SecretKeySelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#secretkeyselector-v1-core) | false |
| kubeletScraping | Configuration to scrape the metric endpoints of the Kubelets. | *[KubeletScraping](#kubeletscraping) | false |
| compression | Compression enables compression of metrics collection data | CompressionType | false |

[Back to TOC](#table-of-contents)

## ConfigSpec

ConfigSpec holds configurations for the Prometheus configuration.


<em>appears in: [OperatorFeatures](#operatorfeatures)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| compression | Compression enables compression of the config data propagated by the operator to collectors. It is recommended to use the gzip option when using a large number of ClusterPodMonitoring and/or PodMonitoring. | CompressionType | false |

[Back to TOC](#table-of-contents)

## ExportFilters

ExportFilters provides mechanisms to filter the scraped data that's sent to GMP.


<em>appears in: [CollectionSpec](#collectionspec)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| matchOneOf | A list Prometheus time series matchers. Every time series must match at least one of the matchers to be exported. This field can be used equivalently to the match[] parameter of the Prometheus federation endpoint to selectively export data. Example: `[\"{job!='foobar'}\", \"{__name__!~'container_foo.*\|container_bar.*'}\"]` | []string | false |

[Back to TOC](#table-of-contents)

## GlobalRules

GlobalRules defines Prometheus alerting and recording rules that are scoped to all data in the queried project. If the project_id or location labels are not preserved by the rule, they default to the values of the cluster.


<em>appears in: [GlobalRulesList](#globalruleslist)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#objectmeta-v1-meta) | false |
| spec | Specification of rules to record and alert on. | [RulesSpec](#rulesspec) | true |
| status | Most recently observed status of the resource. | [RulesStatus](#rulesstatus) | true |

[Back to TOC](#table-of-contents)

## GlobalRulesList

GlobalRulesList is a list of GlobalRules.

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#listmeta-v1-meta) | false |
| items |  | [][GlobalRules](#globalrules) | true |

[Back to TOC](#table-of-contents)

## HTTPClientConfig

HTTPClientConfig stores HTTP-client configurations.


<em>appears in: [ScrapeEndpoint](#scrapeendpoint)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| tls | Configures the scrape request's TLS settings. | *[TLS](#tls) | false |

[Back to TOC](#table-of-contents)

## KubeletScraping

KubeletScraping allows enabling scraping of the Kubelets' metric endpoints.


<em>appears in: [CollectionSpec](#collectionspec)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| interval | The interval at which the metric endpoints are scraped. | string | true |

[Back to TOC](#table-of-contents)

## LabelMapping

LabelMapping specifies how to transfer a label from a Kubernetes resource onto a Prometheus target.


<em>appears in: [TargetLabels](#targetlabels)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| from | Kubenetes resource label to remap. | string | true |
| to | Remapped Prometheus target label. Defaults to the same name as `From`. | string | false |

[Back to TOC](#table-of-contents)

## ManagedAlertmanagerSpec

ManagedAlertmanagerSpec holds configuration information for the managed Alertmanager instance.


<em>appears in: [OperatorConfig](#operatorconfig)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| configSecret | ConfigSecret refers to the name of a single-key Secret in the public namespace that holds the managed Alertmanager config file. | *[v1.SecretKeySelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#secretkeyselector-v1-core) | false |

[Back to TOC](#table-of-contents)

## MonitoringCondition

MonitoringCondition describes a condition of a PodMonitoring.


<em>appears in: [PodMonitoringStatus](#podmonitoringstatus)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| type |  | MonitoringConditionType | true |
| status | Status of the condition, one of True, False, Unknown. | corev1.ConditionStatus | true |
| lastUpdateTime | The last time this condition was updated. | metav1.Time | false |
| lastTransitionTime | Last time the condition transitioned from one status to another. | metav1.Time | false |
| reason | The reason for the condition's last transition. | string | false |
| message | A human-readable message indicating details about the transition. | string | false |

[Back to TOC](#table-of-contents)

## OperatorConfig

OperatorConfig defines configuration of the gmp-operator.


<em>appears in: [OperatorConfigList](#operatorconfiglist)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#objectmeta-v1-meta) | false |
| rules | Rules specifies how the operator configures and deployes rule-evaluator. | [RuleEvaluatorSpec](#ruleevaluatorspec) | false |
| collection | Collection specifies how the operator configures collection. | [CollectionSpec](#collectionspec) | false |
| managedAlertmanager | ManagedAlertmanager holds information for configuring the managed instance of Alertmanager. | *[ManagedAlertmanagerSpec](#managedalertmanagerspec) | false |
| features | Features holds configuration for optional managed-collection features. | [OperatorFeatures](#operatorfeatures) | false |

[Back to TOC](#table-of-contents)

## OperatorConfigList

OperatorConfigList is a list of OperatorConfigs.

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#listmeta-v1-meta) | false |
| items |  | [][OperatorConfig](#operatorconfig) | true |

[Back to TOC](#table-of-contents)

## OperatorFeatures

OperatorFeatures holds configuration for optional managed-collection features.


<em>appears in: [OperatorConfig](#operatorconfig)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| targetStatus | Configuration of target status reporting. | [TargetStatusSpec](#targetstatusspec) | false |
| config | Settings for the collector configuration propagation. | [ConfigSpec](#configspec) | false |

[Back to TOC](#table-of-contents)

## PodMonitoring

PodMonitoring defines monitoring for a set of pods, scoped to pods within the PodMonitoring's namespace.


<em>appears in: [PodMonitoringList](#podmonitoringlist)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#objectmeta-v1-meta) | false |
| spec | Specification of desired Pod selection for target discovery by Prometheus. | [PodMonitoringSpec](#podmonitoringspec) | true |
| status | Most recently observed status of the resource. | [PodMonitoringStatus](#podmonitoringstatus) | true |

[Back to TOC](#table-of-contents)

## PodMonitoringList

PodMonitoringList is a list of PodMonitorings.

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#listmeta-v1-meta) | false |
| items |  | [][PodMonitoring](#podmonitoring) | true |

[Back to TOC](#table-of-contents)

## PodMonitoringSpec

PodMonitoringSpec contains specification parameters for PodMonitoring.


<em>appears in: [PodMonitoring](#podmonitoring)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| selector | Label selector that specifies which pods are selected for this monitoring configuration. | [metav1.LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#labelselector-v1-meta) | true |
| endpoints | The endpoints to scrape on the selected pods. | [][ScrapeEndpoint](#scrapeendpoint) | true |
| targetLabels | Labels to add to the Prometheus target for discovered endpoints. The `instance` label is always set to `<pod_name>:<port>` or `<node_name>:<port>` if the scraped pod is controlled by a DaemonSet. | [TargetLabels](#targetlabels) | false |
| limits | Limits to apply at scrape time. | *[ScrapeLimits](#scrapelimits) | false |

[Back to TOC](#table-of-contents)

## PodMonitoringStatus

PodMonitoringStatus holds status information of a PodMonitoring resource.


<em>appears in: [ClusterPodMonitoring](#clusterpodmonitoring), [PodMonitoring](#podmonitoring)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| observedGeneration | The generation observed by the controller. | int64 | true |
| conditions | Represents the latest available observations of a podmonitor's current state. | [][MonitoringCondition](#monitoringcondition) | false |
| endpointStatuses | Represents the latest available observations of target state for each ScrapeEndpoint. | [][ScrapeEndpointStatus](#scrapeendpointstatus) | false |

[Back to TOC](#table-of-contents)

## RelabelingRule

RelabelingRule defines a single Prometheus relabeling rule.


<em>appears in: [ScrapeEndpoint](#scrapeendpoint)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| sourceLabels | The source labels select values from existing labels. Their content is concatenated using the configured separator and matched against the configured regular expression for the replace, keep, and drop actions. | []string | false |
| separator | Separator placed between concatenated source label values. Defaults to ';'. | string | false |
| targetLabel | Label to which the resulting value is written in a replace action. It is mandatory for replace actions. Regex capture groups are available. | string | false |
| regex | Regular expression against which the extracted value is matched. Defaults to '(.*)'. | string | false |
| modulus | Modulus to take of the hash of the source label values. | uint64 | false |
| replacement | Replacement value against which a regex replace is performed if the regular expression matches. Regex capture groups are available. Defaults to '$1'. | string | false |
| action | Action to perform based on regex matching. Defaults to 'replace'. | string | false |

[Back to TOC](#table-of-contents)

## Rule

Rule is a single rule in the Prometheus format: https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/


<em>appears in: [RuleGroup](#rulegroup)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| record | Record the result of the expression to this metric name. Only one of `record` and `alert` must be set. | string | false |
| alert | Name of the alert to evaluate the expression as. Only one of `record` and `alert` must be set. | string | false |
| expr | The PromQL expression to evaluate. | string | true |
| for | The duration to wait before a firing alert produced by this rule is sent to Alertmanager. Only valid if `alert` is set. | string | false |
| labels | A set of labels to attach to the result of the query expression. | map[string]string | false |
| annotations | A set of annotations to attach to alerts produced by the query expression. Only valid if `alert` is set. | map[string]string | false |

[Back to TOC](#table-of-contents)

## RuleEvaluatorSpec

RuleEvaluatorSpec defines configuration for deploying rule-evaluator.


<em>appears in: [OperatorConfig](#operatorconfig)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| externalLabels | ExternalLabels specifies external labels that are attached to any rule results and alerts produced by rules. The precedence behavior matches that of Prometheus. | map[string]string | false |
| queryProjectID | QueryProjectID is the GCP project ID to evaluate rules against. If left blank, the rule-evaluator will try attempt to infer the Project ID from the environment. | string | false |
| generatorUrl | The base URL used for the generator URL in the alert notification payload. Should point to an instance of a query frontend that gives access to queryProjectID. | string | false |
| alerting | Alerting contains how the rule-evaluator configures alerting. | [AlertingSpec](#alertingspec) | false |
| credentials | A reference to GCP service account credentials with which the rule evaluator container is run. It needs to have metric read permissions against queryProjectId and metric write permissions against all projects to which rule results are written. Within GKE, this can typically be left empty if the compute default service account has the required permissions. | *[v1.SecretKeySelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#secretkeyselector-v1-core) | false |

[Back to TOC](#table-of-contents)

## RuleGroup

RuleGroup declares rules in the Prometheus format: https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/


<em>appears in: [RulesSpec](#rulesspec)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| name | The name of the rule group. | string | true |
| interval | The interval at which to evaluate the rules. Must be a valid Prometheus duration. | string | true |
| rules | A list of rules that are executed sequentially as part of this group. | [][Rule](#rule) | true |

[Back to TOC](#table-of-contents)

## Rules

Rules defines Prometheus alerting and recording rules that are scoped to the namespace of the resource. Only metric data from this namespace is processed and all rule results have their project_id, cluster, and namespace label preserved for query processing. If the location label is not preserved by the rule, it defaults to the cluster's location.


<em>appears in: [RulesList](#ruleslist)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#objectmeta-v1-meta) | false |
| spec | Specification of rules to record and alert on. | [RulesSpec](#rulesspec) | true |
| status | Most recently observed status of the resource. | [RulesStatus](#rulesstatus) | true |

[Back to TOC](#table-of-contents)

## RulesList

RulesList is a list of Rules.

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#listmeta-v1-meta) | false |
| items |  | [][Rules](#rules) | true |

[Back to TOC](#table-of-contents)

## RulesSpec

RulesSpec contains specification parameters for a Rules resource.


<em>appears in: [ClusterRules](#clusterrules), [GlobalRules](#globalrules), [Rules](#rules)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| groups | A list of Prometheus rule groups. | [][RuleGroup](#rulegroup) | true |

[Back to TOC](#table-of-contents)

## SampleGroup




<em>appears in: [ScrapeEndpointStatus](#scrapeendpointstatus)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| sampleTargets | Targets emitting the error message. | [][SampleTarget](#sampletarget) | false |
| count | Total count of similar errors. | *int32 | false |

[Back to TOC](#table-of-contents)

## SampleTarget




<em>appears in: [SampleGroup](#samplegroup)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| labels | The label set, keys and values, of the target. | prommodel.LabelSet | false |
| lastError | Error message. | *string | false |
| lastScrapeDurationSeconds | Scrape duration in seconds. | string | false |
| health | Health status. | string | false |

[Back to TOC](#table-of-contents)

## ScrapeEndpoint

ScrapeEndpoint specifies a Prometheus metrics endpoint to scrape.


<em>appears in: [ClusterPodMonitoringSpec](#clusterpodmonitoringspec), [PodMonitoringSpec](#podmonitoringspec)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| port | Name or number of the port to scrape. The container metadata label is only populated if the port is referenced by name because port numbers are not unique across containers. | intstr.IntOrString | true |
| scheme | Protocol scheme to use to scrape. | string | false |
| path | HTTP path to scrape metrics from. Defaults to \"/metrics\". | string | false |
| params | HTTP GET params to use when scraping. | map[string][]string | false |
| proxyUrl | Proxy URL to scrape through. Encoded passwords are not supported. | string | false |
| interval | Interval at which to scrape metrics. Must be a valid Prometheus duration. | string | false |
| timeout | Timeout for metrics scrapes. Must be a valid Prometheus duration. Must not be larger then the scrape interval. | string | false |
| metricRelabeling | Relabeling rules for metrics scraped from this endpoint. Relabeling rules that override protected target labels (project_id, location, cluster, namespace, job, instance, or __address__) are not permitted. The labelmap action is not permitted in general. | [][RelabelingRule](#relabelingrule) | false |
| tls | Configures the scrape request's TLS settings. | *TLS | false |

[Back to TOC](#table-of-contents)

## ScrapeEndpointStatus




<em>appears in: [PodMonitoringStatus](#podmonitoringstatus)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| name | The name of the ScrapeEndpoint. | string | true |
| activeTargets | Total number of active targets. | int64 | false |
| unhealthyTargets | Total number of active, unhealthy targets. | int64 | false |
| lastUpdateTime | Last time this status was updated. | metav1.Time | false |
| sampleGroups | A fixed sample of targets grouped by error type. | [][SampleGroup](#samplegroup) | false |
| collectorsFraction | Fraction of collectors included in status, bounded [0,1]. Ideally, this should always be 1. Anything less can be considered a problem and should be investigated. | string | false |

[Back to TOC](#table-of-contents)

## ScrapeLimits

ScrapeLimits limits applied to scraped targets.


<em>appears in: [ClusterPodMonitoringSpec](#clusterpodmonitoringspec), [PodMonitoringSpec](#podmonitoringspec)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| samples | Maximum number of samples accepted within a single scrape. Uses Prometheus default if left unspecified. | uint64 | false |
| labels | Maximum number of labels accepted for a single sample. Uses Prometheus default if left unspecified. | uint64 | false |
| labelNameLength | Maximum label name length. Uses Prometheus default if left unspecified. | uint64 | false |
| labelValueLength | Maximum label value length. Uses Prometheus default if left unspecified. | uint64 | false |

[Back to TOC](#table-of-contents)

## SecretOrConfigMap

SecretOrConfigMap allows to specify data as a Secret or ConfigMap. Fields are mutually exclusive. Taking inspiration from prometheus-operator: https://github.com/prometheus-operator/prometheus-operator/blob/2c81b0cf6a5673e08057499a08ddce396b19dda4/Documentation/api.md#secretorconfigmap


<em>appears in: [TLSConfig](#tlsconfig)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| secret | Secret containing data to use for the targets. | *[v1.SecretKeySelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#secretkeyselector-v1-core) | false |
| configMap | ConfigMap containing data to use for the targets. | *v1.ConfigMapKeySelector | false |

[Back to TOC](#table-of-contents)

## TLS

TLS specifies TLS configuration parameters from Kubernetes resources.


<em>appears in: [HTTPClientConfig](#httpclientconfig)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| serverName | Used to verify the hostname for the targets. | string | false |
| insecureSkipVerify | Disable target certificate validation. | bool | false |
| minVersion | Minimum TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS 1.1), TLS12 (TLS 1.2), TLS13 (TLS 1.3). If unset, Prometheus will use Go default minimum version, which is TLS 1.2. See MinVersion in https://pkg.go.dev/crypto/tls#Config. | string | false |
| maxVersion | Maximum TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS 1.1), TLS12 (TLS 1.2), TLS13 (TLS 1.3). If unset, Prometheus will use Go default minimum version, which is TLS 1.2. See MinVersion in https://pkg.go.dev/crypto/tls#Config. | string | false |

[Back to TOC](#table-of-contents)

## TLSConfig

TLSConfig specifies TLS configuration parameters from Kubernetes resources.


<em>appears in: [AlertmanagerEndpoints](#alertmanagerendpoints)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| ca | Struct containing the CA cert to use for the targets. | *[SecretOrConfigMap](#secretorconfigmap) | false |
| cert | Struct containing the client cert file for the targets. | *[SecretOrConfigMap](#secretorconfigmap) | false |
| keySecret | Secret containing the client key file for the targets. | *[v1.SecretKeySelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#secretkeyselector-v1-core) | false |
| serverName | Used to verify the hostname for the targets. | string | false |
| insecureSkipVerify | Disable target certificate validation. | bool | false |
| minVersion | Minimum TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS 1.1), TLS12 (TLS 1.2), TLS13 (TLS 1.3). If unset, Prometheus will use Go default minimum version, which is TLS 1.2. See MinVersion in https://pkg.go.dev/crypto/tls#Config. | string | false |
| maxVersion | Maximum TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS 1.1), TLS12 (TLS 1.2), TLS13 (TLS 1.3). If unset, Prometheus will use Go default minimum version, which is TLS 1.2. See MinVersion in https://pkg.go.dev/crypto/tls#Config. | string | false |

[Back to TOC](#table-of-contents)

## TargetLabels

TargetLabels configures labels for the discovered Prometheus targets.


<em>appears in: [ClusterPodMonitoringSpec](#clusterpodmonitoringspec), [PodMonitoringSpec](#podmonitoringspec)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata | Pod metadata labels that are set on all scraped targets. Permitted keys are `pod`, `container`, and `node` for PodMonitoring and `pod`, `container`, `node`, and `namespace` for ClusterPodMonitoring. The `container` label is only populated if the scrape port is referenced by name. Defaults to [pod, container] for PodMonitoring and [namespace, pod, container] for ClusterPodMonitoring. If set to null, it will be interpreted as the empty list for PodMonitoring and to [namespace] for ClusterPodMonitoring. This is for backwards-compatibility only. | *[]string | false |
| fromPod | Labels to transfer from the Kubernetes Pod to Prometheus target labels. Mappings are applied in order. | [][LabelMapping](#labelmapping) | false |

[Back to TOC](#table-of-contents)

## TargetStatusSpec

TargetStatusSpec holds configuration for target status reporting.


<em>appears in: [OperatorFeatures](#operatorfeatures)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| enabled | Enable target status reporting. | bool | false |

[Back to TOC](#table-of-contents)
