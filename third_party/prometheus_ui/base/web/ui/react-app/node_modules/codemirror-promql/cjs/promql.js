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
Object.defineProperty(exports, "__esModule", { value: true });
exports.PromQLExtension = exports.promQLLanguage = void 0;
var lezer_promql_1 = require("lezer-promql");
var highlight_1 = require("@codemirror/highlight");
var complete_1 = require("./complete");
var lint_1 = require("./lint");
var language_1 = require("@codemirror/language");
exports.promQLLanguage = language_1.LezerLanguage.define({
    parser: lezer_promql_1.parser.configure({
        props: [
            highlight_1.styleTags({
                LineComment: highlight_1.tags.comment,
                LabelName: highlight_1.tags.labelName,
                StringLiteral: highlight_1.tags.string,
                NumberLiteral: highlight_1.tags.number,
                Duration: highlight_1.tags.number,
                'Abs Absent AbsentOverTime AvgOverTime Ceil Changes Clamp ClampMax ClampMin CountOverTime DaysInMonth DayOfMonth DayOfWeek Delta Deriv Exp Floor HistogramQuantile HoltWinters Hour Idelta Increase Irate LabelReplace LabelJoin LastOverTime Ln Log10 Log2 MaxOverTime MinOverTime Minute Month PredictLinear QuantileOverTime Rate Resets Round Scalar Sgn Sort SortDesc Sqrt StddevOverTime StdvarOverTime SumOverTime Time Timestamp Vector Year': highlight_1.tags.function(highlight_1.tags.variableName),
                'Avg Bottomk Count Count_values Group Max Min Quantile Stddev Stdvar Sum Topk': highlight_1.tags.operatorKeyword,
                'By Without Bool On Ignoring GroupLeft GroupRight Offset Start End': highlight_1.tags.modifier,
                'And Unless Or': highlight_1.tags.logicOperator,
                'Sub Add Mul Mod Div Eql Neq Lte Lss Gte Gtr EqlRegex EqlSingle NeqRegex Pow At': highlight_1.tags.operator,
                UnaryOp: highlight_1.tags.arithmeticOperator,
                '( )': highlight_1.tags.paren,
                '[ ]': highlight_1.tags.squareBracket,
                '{ }': highlight_1.tags.brace,
                '⚠': highlight_1.tags.invalid,
            }),
        ],
    }),
    languageData: {
        closeBrackets: { brackets: ['(', '[', '{', "'", '"', '`'] },
        commentTokens: { line: '#' },
    },
});
/**
 * This class holds the state of the completion extension for CodeMirror and allow hot-swapping the complete strategy.
 */
var PromQLExtension = /** @class */ (function () {
    function PromQLExtension() {
        this.complete = complete_1.newCompleteStrategy();
        this.lint = lint_1.newLintStrategy();
        this.enableLinter = true;
        this.enableCompletion = true;
    }
    PromQLExtension.prototype.setComplete = function (conf) {
        this.complete = complete_1.newCompleteStrategy(conf);
        return this;
    };
    PromQLExtension.prototype.getComplete = function () {
        return this.complete;
    };
    PromQLExtension.prototype.activateCompletion = function (activate) {
        this.enableCompletion = activate;
        return this;
    };
    PromQLExtension.prototype.setLinter = function (linter) {
        this.lint = linter;
        return this;
    };
    PromQLExtension.prototype.getLinter = function () {
        return this.lint;
    };
    PromQLExtension.prototype.activateLinter = function (activate) {
        this.enableLinter = activate;
        return this;
    };
    PromQLExtension.prototype.asExtension = function () {
        var _this = this;
        var extension = [exports.promQLLanguage];
        if (this.enableCompletion) {
            var completion = exports.promQLLanguage.data.of({
                autocomplete: function (context) {
                    return _this.complete.promQL(context);
                },
            });
            extension = extension.concat(completion);
        }
        if (this.enableLinter) {
            extension = extension.concat(lint_1.promQLLinter(this.lint.promQL, this.lint));
        }
        return extension;
    };
    return PromQLExtension;
}());
exports.PromQLExtension = PromQLExtension;
//# sourceMappingURL=promql.js.map