// The MIT License (MIT)
//
// Copyright (c) 2020 The Prometheus Authors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.
import { labelMatchersToString } from '../parser/matcher';
import LRUCache from 'lru-cache';
const apiPrefix = '/api/v1';
const labelsEndpoint = apiPrefix + '/labels';
const labelValuesEndpoint = apiPrefix + '/label/:name/values';
const seriesEndpoint = apiPrefix + '/series';
const metricMetadataEndpoint = apiPrefix + '/metadata';
// These are status codes where the Prometheus API still returns a valid JSON body,
// with an error encoded within the JSON.
const badRequest = 400;
const unprocessableEntity = 422;
const serviceUnavailable = 503;
// HTTPPrometheusClient is the HTTP client that should be used to get some information from the different endpoint provided by prometheus.
export class HTTPPrometheusClient {
    constructor(config) {
        this.lookbackInterval = 60 * 60 * 1000 * 12; //12 hours
        // For some reason, just assigning via "= fetch" here does not end up executing fetch correctly
        // when calling it, thus the indirection via another function wrapper.
        this.fetchFn = (input, init) => fetch(input, init);
        this.url = config.url;
        this.errorHandler = config.httpErrorHandler;
        if (config.lookbackInterval) {
            this.lookbackInterval = config.lookbackInterval;
        }
        if (config.fetchFn) {
            this.fetchFn = config.fetchFn;
        }
    }
    labelNames(metricName) {
        const end = new Date();
        const start = new Date(end.getTime() - this.lookbackInterval);
        if (metricName === undefined || metricName === '') {
            const params = new URLSearchParams({
                start: start.toISOString(),
                end: end.toISOString(),
            });
            // See https://prometheus.io/docs/prometheus/latest/querying/api/#getting-label-names
            return this.fetchAPI(`${labelsEndpoint}?${params}`).catch((error) => {
                if (this.errorHandler) {
                    this.errorHandler(error);
                }
                return [];
            });
        }
        return this.series(metricName).then((series) => {
            const labelNames = new Set();
            for (const labelSet of series) {
                for (const [key] of Object.entries(labelSet)) {
                    if (key === '__name__') {
                        continue;
                    }
                    labelNames.add(key);
                }
            }
            return Array.from(labelNames);
        });
    }
    // labelValues return a list of the value associated to the given labelName.
    // In case a metric is provided, then the list of values is then associated to the couple <MetricName, LabelName>
    labelValues(labelName, metricName, matchers) {
        const end = new Date();
        const start = new Date(end.getTime() - this.lookbackInterval);
        if (!metricName || metricName.length === 0) {
            const params = new URLSearchParams({
                start: start.toISOString(),
                end: end.toISOString(),
            });
            // See https://prometheus.io/docs/prometheus/latest/querying/api/#querying-label-values
            return this.fetchAPI(`${labelValuesEndpoint.replace(/:name/gi, labelName)}?${params}`).catch((error) => {
                if (this.errorHandler) {
                    this.errorHandler(error);
                }
                return [];
            });
        }
        return this.series(metricName, matchers, labelName).then((series) => {
            const labelValues = new Set();
            for (const labelSet of series) {
                for (const [key, value] of Object.entries(labelSet)) {
                    if (key === '__name__') {
                        continue;
                    }
                    if (key === labelName) {
                        labelValues.add(value);
                    }
                }
            }
            return Array.from(labelValues);
        });
    }
    metricMetadata() {
        return this.fetchAPI(metricMetadataEndpoint).catch((error) => {
            if (this.errorHandler) {
                this.errorHandler(error);
            }
            return {};
        });
    }
    series(metricName, matchers, labelName) {
        const end = new Date();
        const start = new Date(end.getTime() - this.lookbackInterval);
        const params = new URLSearchParams({
            start: start.toISOString(),
            end: end.toISOString(),
            'match[]': labelMatchersToString(metricName, matchers, labelName),
        });
        // See https://prometheus.io/docs/prometheus/latest/querying/api/#finding-series-by-label-matchers
        return this.fetchAPI(`${seriesEndpoint}?${params}`).catch((error) => {
            if (this.errorHandler) {
                this.errorHandler(error);
            }
            return [];
        });
    }
    metricNames() {
        return this.labelValues('__name__');
    }
    fetchAPI(resource) {
        return this.fetchFn(this.url + resource)
            .then((res) => {
            if (!res.ok && ![badRequest, unprocessableEntity, serviceUnavailable].includes(res.status)) {
                throw new Error(res.statusText);
            }
            return res;
        })
            .then((res) => res.json())
            .then((apiRes) => {
            if (apiRes.status === 'error') {
                throw new Error(apiRes.error !== undefined ? apiRes.error : 'missing "error" field in response JSON');
            }
            if (apiRes.data === undefined) {
                throw new Error('missing "data" field in response JSON');
            }
            return apiRes.data;
        });
    }
}
class Cache {
    constructor(maxAge) {
        this.completeAssociation = new LRUCache(maxAge);
        this.metricMetadata = {};
        this.labelValues = new LRUCache(maxAge);
        this.labelNames = [];
    }
    setAssociations(metricName, series) {
        series.forEach((labelSet) => {
            let currentAssociation = this.completeAssociation.get(metricName);
            if (!currentAssociation) {
                currentAssociation = new Map();
                this.completeAssociation.set(metricName, currentAssociation);
            }
            for (const [key, value] of Object.entries(labelSet)) {
                if (key === '__name__') {
                    continue;
                }
                const labelValues = currentAssociation.get(key);
                if (labelValues === undefined) {
                    currentAssociation.set(key, new Set([value]));
                }
                else {
                    labelValues.add(value);
                }
            }
        });
    }
    setMetricMetadata(metadata) {
        this.metricMetadata = metadata;
    }
    getMetricMetadata() {
        return this.metricMetadata;
    }
    setLabelNames(labelNames) {
        this.labelNames = labelNames;
    }
    getLabelNames(metricName) {
        if (!metricName || metricName.length === 0) {
            return this.labelNames;
        }
        const labelSet = this.completeAssociation.get(metricName);
        return labelSet ? Array.from(labelSet.keys()) : [];
    }
    setLabelValues(labelName, labelValues) {
        this.labelValues.set(labelName, labelValues);
    }
    getLabelValues(labelName, metricName) {
        if (!metricName || metricName.length === 0) {
            const result = this.labelValues.get(labelName);
            return result ? result : [];
        }
        const labelSet = this.completeAssociation.get(metricName);
        if (labelSet) {
            const labelValues = labelSet.get(labelName);
            return labelValues ? Array.from(labelValues) : [];
        }
        return [];
    }
}
export class CachedPrometheusClient {
    constructor(client, maxAge = 5 * 60 * 1000) {
        this.client = client;
        this.cache = new Cache(maxAge);
    }
    labelNames(metricName) {
        const cachedLabel = this.cache.getLabelNames(metricName);
        if (cachedLabel && cachedLabel.length > 0) {
            return Promise.resolve(cachedLabel);
        }
        if (metricName === undefined || metricName === '') {
            return this.client.labelNames().then((labelNames) => {
                this.cache.setLabelNames(labelNames);
                return labelNames;
            });
        }
        return this.series(metricName).then(() => {
            return this.cache.getLabelNames(metricName);
        });
    }
    labelValues(labelName, metricName) {
        const cachedLabel = this.cache.getLabelValues(labelName, metricName);
        if (cachedLabel && cachedLabel.length > 0) {
            return Promise.resolve(cachedLabel);
        }
        if (metricName === undefined || metricName === '') {
            return this.client.labelValues(labelName).then((labelValues) => {
                this.cache.setLabelValues(labelName, labelValues);
                return labelValues;
            });
        }
        return this.series(metricName).then(() => {
            return this.cache.getLabelValues(labelName, metricName);
        });
    }
    metricMetadata() {
        const cachedMetadata = this.cache.getMetricMetadata();
        if (cachedMetadata && Object.keys(cachedMetadata).length > 0) {
            return Promise.resolve(cachedMetadata);
        }
        return this.client.metricMetadata().then((metadata) => {
            this.cache.setMetricMetadata(metadata);
            return this.cache.getMetricMetadata();
        });
    }
    series(metricName) {
        return this.client.series(metricName).then((series) => {
            this.cache.setAssociations(metricName, series);
            return series;
        });
    }
    metricNames() {
        return this.labelValues('__name__');
    }
}
//# sourceMappingURL=prometheus.js.map