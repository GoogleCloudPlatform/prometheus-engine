"use strict";
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
var _a;
Object.defineProperty(exports, "__esModule", { value: true });
exports.getFunction = exports.ValueType = void 0;
var lezer_promql_1 = require("lezer-promql");
var ValueType;
(function (ValueType) {
    ValueType["none"] = "none";
    ValueType["vector"] = "vector";
    ValueType["scalar"] = "scalar";
    ValueType["matrix"] = "matrix";
    ValueType["string"] = "string";
})(ValueType = exports.ValueType || (exports.ValueType = {}));
// promqlFunctions is a list of all functions supported by PromQL, including their types.
// Based on https://github.com/prometheus/prometheus/blob/master/promql/parser/functions.go#L26
var promqlFunctions = (_a = {},
    _a[lezer_promql_1.Abs] = {
        name: 'abs',
        argTypes: [ValueType.vector],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.Absent] = {
        name: 'absent',
        argTypes: [ValueType.vector],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.AbsentOverTime] = {
        name: 'absent_over_time',
        argTypes: [ValueType.matrix],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.AvgOverTime] = {
        name: 'avg_over_time',
        argTypes: [ValueType.matrix],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.Ceil] = {
        name: 'ceil',
        argTypes: [ValueType.vector],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.Changes] = {
        name: 'changes',
        argTypes: [ValueType.matrix],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.Clamp] = {
        name: 'clamp',
        argTypes: [ValueType.vector, ValueType.scalar, ValueType.scalar],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.ClampMax] = {
        name: 'clamp_max',
        argTypes: [ValueType.vector, ValueType.scalar],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.ClampMin] = {
        name: 'clamp_min',
        argTypes: [ValueType.vector, ValueType.scalar],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.CountOverTime] = {
        name: 'count_over_time',
        argTypes: [ValueType.matrix],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.DaysInMonth] = {
        name: 'days_in_month',
        argTypes: [ValueType.vector],
        variadic: 1,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.DayOfMonth] = {
        name: 'day_of_month',
        argTypes: [ValueType.vector],
        variadic: 1,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.DayOfWeek] = {
        name: 'day_of_week',
        argTypes: [ValueType.vector],
        variadic: 1,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.Delta] = {
        name: 'delta',
        argTypes: [ValueType.matrix],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.Deriv] = {
        name: 'deriv',
        argTypes: [ValueType.matrix],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.Exp] = {
        name: 'exp',
        argTypes: [ValueType.vector],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.Floor] = {
        name: 'floor',
        argTypes: [ValueType.vector],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.HistogramQuantile] = {
        name: 'histogram_quantile',
        argTypes: [ValueType.scalar, ValueType.vector],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.HoltWinters] = {
        name: 'holt_winters',
        argTypes: [ValueType.matrix, ValueType.scalar, ValueType.scalar],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.Hour] = {
        name: 'hour',
        argTypes: [ValueType.vector],
        variadic: 1,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.Idelta] = {
        name: 'idelta',
        argTypes: [ValueType.matrix],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.Increase] = {
        name: 'increase',
        argTypes: [ValueType.matrix],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.Irate] = {
        name: 'irate',
        argTypes: [ValueType.matrix],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.LabelReplace] = {
        name: 'label_replace',
        argTypes: [ValueType.vector, ValueType.string, ValueType.string, ValueType.string, ValueType.string],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.LabelJoin] = {
        name: 'label_join',
        argTypes: [ValueType.vector, ValueType.string, ValueType.string, ValueType.string],
        variadic: -1,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.LastOverTime] = {
        name: 'last_over_time',
        argTypes: [ValueType.matrix],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.Ln] = {
        name: 'ln',
        argTypes: [ValueType.vector],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.Log10] = {
        name: 'log10',
        argTypes: [ValueType.vector],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.Log2] = {
        name: 'log2',
        argTypes: [ValueType.vector],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.MaxOverTime] = {
        name: 'max_over_time',
        argTypes: [ValueType.matrix],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.MinOverTime] = {
        name: 'min_over_time',
        argTypes: [ValueType.matrix],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.Minute] = {
        name: 'minute',
        argTypes: [ValueType.vector],
        variadic: 1,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.Month] = {
        name: 'month',
        argTypes: [ValueType.vector],
        variadic: 1,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.PredictLinear] = {
        name: 'predict_linear',
        argTypes: [ValueType.matrix, ValueType.scalar],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.QuantileOverTime] = {
        name: 'quantile_over_time',
        argTypes: [ValueType.scalar, ValueType.matrix],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.Rate] = {
        name: 'rate',
        argTypes: [ValueType.matrix],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.Resets] = {
        name: 'resets',
        argTypes: [ValueType.matrix],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.Round] = {
        name: 'round',
        argTypes: [ValueType.vector, ValueType.scalar],
        variadic: 1,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.Scalar] = {
        name: 'scalar',
        argTypes: [ValueType.vector],
        variadic: 0,
        returnType: ValueType.scalar,
    },
    _a[lezer_promql_1.Sgn] = {
        name: 'sgn',
        argTypes: [ValueType.vector],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.Sort] = {
        name: 'sort',
        argTypes: [ValueType.vector],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.SortDesc] = {
        name: 'sort_desc',
        argTypes: [ValueType.vector],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.Sqrt] = {
        name: 'sqrt',
        argTypes: [ValueType.vector],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.StddevOverTime] = {
        name: 'stddev_over_time',
        argTypes: [ValueType.matrix],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.StdvarOverTime] = {
        name: 'stdvar_over_time',
        argTypes: [ValueType.matrix],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.SumOverTime] = {
        name: 'sum_over_time',
        argTypes: [ValueType.matrix],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.Time] = {
        name: 'time',
        argTypes: [],
        variadic: 0,
        returnType: ValueType.scalar,
    },
    _a[lezer_promql_1.Timestamp] = {
        name: 'timestamp',
        argTypes: [ValueType.vector],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.Vector] = {
        name: 'vector',
        argTypes: [ValueType.scalar],
        variadic: 0,
        returnType: ValueType.vector,
    },
    _a[lezer_promql_1.Year] = {
        name: 'year',
        argTypes: [ValueType.vector],
        variadic: 1,
        returnType: ValueType.vector,
    },
    _a);
function getFunction(id) {
    return promqlFunctions[id];
}
exports.getFunction = getFunction;
//# sourceMappingURL=function.js.map