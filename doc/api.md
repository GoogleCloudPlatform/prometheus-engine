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
* [Rule](#rule)
* [RuleEvaluatorSpec](#ruleevaluatorspec)
* [RuleGroup](#rulegroup)
* [Rules](#rules)
* [RulesList](#ruleslist)
* [RulesSpec](#rulesspec)
* [ScrapeEndpoint](#scrapeendpoint)
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

## CollectionSpec

CollectionSpec specifies how the operator configures collection of metric data.


<em>appears in: [OperatorConfig](#operatorconfig)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| externalLabels | ExternalLabels specifies external labels that are attached to all scraped data before being written to Cloud Monitoring. The precedence behavior matches that of Prometheus. | map[string]string | false |
| filter | Filter limits which metric data is sent to Cloud Monitoring. | [ExportFilters](#exportfilters) | false |

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
| selector |  | [metav1.LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.17/#labelselector-v1-meta) | true |
| endpoints |  | [][ScrapeEndpoint](#scrapeendpoint) | true |
| targetLabels |  | [TargetLabels](#targetlabels) | false |

[Back to TOC](#table-of-contents)

## PodMonitoringStatus

PodMonitoringStatus holds status information of a PodMonitoring resource.


<em>appears in: [PodMonitoring](#podmonitoring)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| observedGeneration | The generation observed by the controller. | int64 | true |
| conditions | Represents the latest available observations of a podmonitor's current state. | [][MonitoringCondition](#monitoringcondition) | false |

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

Rules defines Prometheus alerting and recording rules.


<em>appears in: [RulesList](#ruleslist)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.17/#objectmeta-v1-meta) | false |
| spec | Specification of desired Pod selection for target discovery by Prometheus. | [RulesSpec](#rulesspec) | true |
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


<em>appears in: [Rules](#rules)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| scope | The scope at which to evaluate rules. Must be \"Cluster\" or \"Namespace\". It acts as safety mechanism against unintentionally having rules query more data than intended without requiring adjusting all selectors of the PromQL expression.\n\nAt the Cluster scope only metrics with target labels \"project_id\" and \"cluster\" matching the current one are used as input to rules. At the Namespace scope they are further restricted by the namespace the Rules resource is in. | Scope | true |
| groups | A list of Prometheus rule groups. | [][RuleGroup](#rulegroup) | true |

[Back to TOC](#table-of-contents)

## ScrapeEndpoint

ScrapeEndpoint specifies a Prometheus metrics endpoint to scrape.


<em>appears in: [PodMonitoringSpec](#podmonitoringspec)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| port | Name or number of the port to scrape. | intstr.IntOrString | false |
| path | HTTP path to scrape metrics from. Defaults to \"/metrics\". | string | false |
| interval | Interval at which to scrape metrics. Must be a valid Prometheus duration. | string | false |
| timeout | Timeout for metrics scrapes. Must be a valid Prometheus duration. Must not be larger then the scrape interval. | string | false |

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
| ca | Struct containing the CA cert to use for the targets. | [SecretOrConfigMap](#secretorconfigmap) | false |
| cert | Struct containing the client cert file for the targets. | [SecretOrConfigMap](#secretorconfigmap) | false |
| keySecret | Secret containing the client key file for the targets. | *[v1.SecretKeySelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.17/#secretkeyselector-v1-core) | false |
| serverName | Used to verify the hostname for the targets. | string | false |
| insecureSkipVerify | Disable target certificate validation. | bool | false |

[Back to TOC](#table-of-contents)

## TargetLabels

TargetLabels groups label mappings by Kubernetes resource.


<em>appears in: [PodMonitoringSpec](#podmonitoringspec)</em>

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| fromPod | Labels to transfer from the Kubernetes Pod to Prometheus target labels. In the case of a label mapping conflict: - Mappings at the end of the array take precedence. | [][LabelMapping](#labelmapping) | false |

[Back to TOC](#table-of-contents)
