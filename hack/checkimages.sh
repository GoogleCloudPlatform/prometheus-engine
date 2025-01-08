#!/bin/bash
set -o errexit
set -o nounset
set -o pipefail

source .bingo/variables.env

set -o xtrace

ALERTMANAGER_IMAGE=$(${YQ} '.images.alertmanager.image' charts/values.global.yaml)
ALERTMANAGER_TAG=$(${YQ} '.images.alertmanager.tag' charts/values.global.yaml)
docker manifest inspect "${ALERTMANAGER_IMAGE}:${ALERTMANAGER_TAG}" > /dev/null

BASH_IMAGE=$(${YQ} '.images.bash.image' charts/values.global.yaml)
BASH_TAG=$(${YQ} '.images.bash.tag' charts/values.global.yaml)
docker manifest inspect "${BASH_IMAGE}:${BASH_TAG}" > /dev/null

CONFIG_RELOADER_IMAGE=$(${YQ} '.images.configReloader.image' charts/values.global.yaml)
CONFIG_RELOADER_TAG=$(${YQ} '.images.configReloader.tag' charts/values.global.yaml)
docker manifest inspect "${CONFIG_RELOADER_IMAGE}:${CONFIG_RELOADER_TAG}" > /dev/null

DATASOURCE_SYNCER_IMAGE=$(${YQ} '.images.datasourceSyncer.image' charts/values.global.yaml)
DATASOURCE_SYNCER_TAG=$(${YQ} '.images.datasourceSyncer.tag' charts/values.global.yaml)
docker manifest inspect "${DATASOURCE_SYNCER_IMAGE}:${DATASOURCE_SYNCER_TAG}" > /dev/null

OPERATOR_IMAGE=$(${YQ} '.images.operator.image' charts/values.global.yaml)
OPERATOR_TAG=$(${YQ} '.images.operator.tag' charts/values.global.yaml)
docker manifest inspect "${OPERATOR_IMAGE}:${OPERATOR_TAG}" > /dev/null

PROMETHEUS_IMAGE=$(${YQ} '.images.prometheus.image' charts/values.global.yaml)
PROMETHEUS_TAG=$(${YQ} '.images.prometheus.tag' charts/values.global.yaml)
docker manifest inspect "${PROMETHEUS_IMAGE}:${PROMETHEUS_TAG}" > /dev/null

RULE_EVALUATOR_IMAGE=$(${YQ} '.images.ruleEvaluator.image' charts/values.global.yaml)
RULE_EVALUATOR_TAG=$(${YQ} '.images.ruleEvaluator.tag' charts/values.global.yaml)
docker manifest inspect "${RULE_EVALUATOR_IMAGE}:${RULE_EVALUATOR_TAG}" > /dev/null
