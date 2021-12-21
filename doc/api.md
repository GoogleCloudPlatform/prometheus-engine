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
* [ClusterRules](#clusterrules)
* [ClusterRulesList](#clusterruleslist)
* [CollectionSpec](#collectionspec)
* [ExportFilters](#exportfilters)
* [LabelMapping](#labelmapping)
* [MonitoringCondition](#monitoringcondition)
* [OperatorConfig](#operatorconfig)
* [OperatorConfigList](#operatorconfiglist)
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
* [ScrapeEndpoint](#scrapeendpoint)
* [ScrapeLimits](#scrapelimits)
* [SecretOrConfigMap](#secretorconfigmap)
* [TLSConfig](#tlsconfig)
* [TargetLabels](#targetlabels)

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
| credentials | The secret's key that contains the credentials of the request | *[v1.SecretKeySelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.17/#secretkeyselector-v1-core) | false |

[Back to TOC](#table-of-contents)

## ClusterRules

ClusterRules defines Prometheus alerting and recording rules that are scoped to the current cluster. Only metric data from the current cluster is processed and all rule results have their project_id and cluster label preserved for query processing. The location, if not preserved by the rule, is set to the cluster's location


<em>appears in: [ClusterRulesList](#clusterruleslist)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.17/#objectmeta-v1-meta) | false |
| spec | Specification of rules to record and alert on. | [RulesSpec](#rulesspec) | true |
| status | Most recently observed status of the resource. | [RulesStatus](#rulesstatus) | true |

[Back to TOC](#table-of-contents)

## ClusterRulesList

ClusterRulesList is a list of ClusterRules.

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.17/#listmeta-v1-meta) | false |
| items |  | [][ClusterRules](#clusterrules) | true |

[Back to TOC](#table-of-contents)

## CollectionSpec

CollectionSpec specifies how the operator configures collection of metric data.


<em>appears in: [OperatorConfig](#operatorconfig)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| externalLabels | ExternalLabels specifies external labels that are attached to all scraped data before being written to Cloud Monitoring. The precedence behavior matches that of Prometheus. | map[string]string | false |
| filter | Filter limits which metric data is sent to Cloud Monitoring. | [ExportFilters](#exportfilters) | false |
| credentials | A reference to GCP service account credentials with which Prometheus collectors are run. It needs to have metric write permissions for all project IDs to which data is written. Within GKE, this can typically be left empty if the compute default service account has the required permissions. | *[v1.SecretKeySelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.17/#secretkeyselector-v1-core) | false |

[Back to TOC](#table-of-contents)

## ExportFilters

ExportFilters provides mechanisms to filter the scraped data that's sent to GMP.


<em>appears in: [CollectionSpec](#collectionspec)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| matchOneOf | A list Prometheus time series matchers. Every time series must match at least one of the matchers to be exported. This field can be used equivalently to the match[] parameter of the Prometheus federation endpoint to selectively export data.\n\nExample: `[\"{job='prometheus'}\", \"{__name__=~'job:.*'}\"]` | []string | false |

[Back to TOC](#table-of-contents)

## LabelMapping

LabelMapping specifies how to transfer a label from a Kubernetes resource onto a Prometheus target.


<em>appears in: [TargetLabels](#targetlabels)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| from | Kubenetes resource label to remap. | string | true |
| to | Remapped Prometheus target label. Defaults to the same name as `From`. | string | false |

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
| metadata |  | [metav1.ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.17/#objectmeta-v1-meta) | false |
| rules | Rules specifies how the operator configures and deployes rule-evaluator. | [RuleEvaluatorSpec](#ruleevaluatorspec) | false |
| collection | Collection specifies how the operator configures collection. | [CollectionSpec](#collectionspec) | false |

[Back to TOC](#table-of-contents)

## OperatorConfigList

OperatorConfigList is a list of OperatorConfigs.

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.17/#listmeta-v1-meta) | false |
| items |  | [][OperatorConfig](#operatorconfig) | true |

[Back to TOC](#table-of-contents)

## PodMonitoring

PodMonitoring defines monitoring for a set of pods.


<em>appears in: [PodMonitoringList](#podmonitoringlist)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.17/#objectmeta-v1-meta) | false |
| spec | Specification of desired Pod selection for target discovery by Prometheus. | [PodMonitoringSpec](#podmonitoringspec) | true |
| status | Most recently observed status of the resource. | [PodMonitoringStatus](#podmonitoringstatus) | true |

[Back to TOC](#table-of-contents)

## PodMonitoringList

PodMonitoringList is a list of PodMonitorings.

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.17/#listmeta-v1-meta) | false |
| items |  | [][PodMonitoring](#podmonitoring) | true |

[Back to TOC](#table-of-contents)

## PodMonitoringSpec

PodMonitoringSpec contains specification parameters for PodMonitoring.


<em>appears in: [PodMonitoring](#podmonitoring)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| selector | Label selector that specifies which pods are selected for this monitoring configuration. | [metav1.LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.17/#labelselector-v1-meta) | true |
| endpoints | The endpoints to scrape on the selected pods. | [][ScrapeEndpoint](#scrapeendpoint) | true |
| targetLabels | Label to add to the Prometheus target for discovered endpoints. | [TargetLabels](#targetlabels) | false |
| limits | Limits to apply at scrape time. | *[ScrapeLimits](#scrapelimits) | false |

[Back to TOC](#table-of-contents)

## PodMonitoringStatus

PodMonitoringStatus holds status information of a PodMonitoring resource.


<em>appears in: [PodMonitoring](#podmonitoring)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| observedGeneration | The generation observed by the controller. | int64 | true |
| conditions | Represents the latest available observations of a podmonitor's current state. | [][MonitoringCondition](#monitoringcondition) | false |

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
| alerting | Alerting contains how the rule-evaluator configures alerting. | [AlertingSpec](#alertingspec) | false |
| credentials | A reference to GCP service account credentials with which the rule evaluator container is run. It needs to have metric read permissions against queryProjectId and metric write permissions against all projects to which rule results are written. Within GKE, this can typically be left empty if the compute default service account has the required permissions. | *[v1.SecretKeySelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.17/#secretkeyselector-v1-core) | false |

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

Rules defines Prometheus alerting and recording rules that are scoped to the namespace of the resource. Only metric data from this namespace is processed and all rule results have their project_id, cluster, and namespace label preserved for query processing. The location, if not preserved by the rule, is set to the cluster's location.


<em>appears in: [RulesList](#ruleslist)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.17/#objectmeta-v1-meta) | false |
| spec | Specification of rules to record and alert on. | [RulesSpec](#rulesspec) | true |
| status | Most recently observed status of the resource. | [RulesStatus](#rulesstatus) | true |

[Back to TOC](#table-of-contents)

## RulesList

RulesList is a list of Rules.

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.17/#listmeta-v1-meta) | false |
| items |  | [][Rules](#rules) | true |

[Back to TOC](#table-of-contents)

## RulesSpec

RulesSpec contains specification parameters for a Rules resource.


<em>appears in: [ClusterRules](#clusterrules), [Rules](#rules)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| groups | A list of Prometheus rule groups. | [][RuleGroup](#rulegroup) | true |

[Back to TOC](#table-of-contents)

## ScrapeEndpoint

ScrapeEndpoint specifies a Prometheus metrics endpoint to scrape.


<em>appears in: [PodMonitoringSpec](#podmonitoringspec)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| port | Name or number of the port to scrape. | intstr.IntOrString | false |
| scheme | Protocol scheme to use to scrape. | string | false |
| path | HTTP path to scrape metrics from. Defaults to \"/metrics\". | string | false |
| params | HTTP GET params to use when scraping. | map[string][]string | false |
| proxyUrl | Proxy URL to scrape through. Encoded passwords are not supported. | string | false |
| interval | Interval at which to scrape metrics. Must be a valid Prometheus duration. | string | false |
| timeout | Timeout for metrics scrapes. Must be a valid Prometheus duration. Must not be larger then the scrape interval. | string | false |
| metricRelabeling | Relabeling rules for metrics scraped from this endpoint. Relabeling rules that override protected target labels (project_id, location, cluster, namespace, job, instance, or __address__) are not permitted. The labelmap action is not permitted in general. | [][RelabelingRule](#relabelingrule) | false |

[Back to TOC](#table-of-contents)

## ScrapeLimits

ScrapeLimits limits applied to scraped targets.


<em>appears in: [PodMonitoringSpec](#podmonitoringspec)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| samples | Maximum number of samples accepted within a single scrape. | uint64 | false |
| labels | Maximum number of labels accepted for a single sample. | uint64 | false |
| labelNameLength | Maximum label name length. | uint64 | false |
| labelValueLength | Maximum label value length. | uint64 | false |

[Back to TOC](#table-of-contents)

## SecretOrConfigMap

SecretOrConfigMap allows to specify data as a Secret or ConfigMap. Fields are mutually exclusive. Taking inspiration from prometheus-operator: https://github.com/prometheus-operator/prometheus-operator/blob/2c81b0cf6a5673e08057499a08ddce396b19dda4/Documentation/api.md#secretorconfigmap


<em>appears in: [TLSConfig](#tlsconfig)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| secret | Secret containing data to use for the targets. | *[v1.SecretKeySelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.17/#secretkeyselector-v1-core) | false |
| configMap | ConfigMap containing data to use for the targets. | *v1.ConfigMapKeySelector | false |

[Back to TOC](#table-of-contents)

## TLSConfig

SafeTLSConfig specifies TLS configuration parameters from Kubernetes resources.


<em>appears in: [AlertmanagerEndpoints](#alertmanagerendpoints)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| ca | Struct containing the CA cert to use for the targets. | *[SecretOrConfigMap](#secretorconfigmap) | false |
| cert | Struct containing the client cert file for the targets. | *[SecretOrConfigMap](#secretorconfigmap) | false |
| keySecret | Secret containing the client key file for the targets. | *[v1.SecretKeySelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.17/#secretkeyselector-v1-core) | false |
| serverName | Used to verify the hostname for the targets. | string | false |
| insecureSkipVerify | Disable target certificate validation. | bool | false |

[Back to TOC](#table-of-contents)

## TargetLabels

TargetLabels configures labels for the discovered Prometheus targets.


<em>appears in: [PodMonitoringSpec](#podmonitoringspec)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| fromPod | Labels to transfer from the Kubernetes Pod to Prometheus target labels. In the case of a label mapping conflict: - Mappings at the end of the array take precedence. | [][LabelMapping](#labelmapping) | false |

[Back to TOC](#table-of-contents)
