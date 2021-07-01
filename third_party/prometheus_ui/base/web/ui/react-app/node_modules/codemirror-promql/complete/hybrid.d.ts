import { CompleteStrategy } from './index';
import { SyntaxNode } from 'lezer-tree';
import { PrometheusClient } from '../client';
import { CompletionContext, CompletionResult } from '@codemirror/autocomplete';
import { EditorState } from '@codemirror/state';
import { Matcher } from '../types/matcher';
export declare enum ContextKind {
    MetricName = 0,
    LabelName = 1,
    LabelValue = 2,
    Function = 3,
    Aggregation = 4,
    BinOpModifier = 5,
    BinOp = 6,
    MatchOp = 7,
    AggregateOpModifier = 8,
    Duration = 9,
    Offset = 10,
    Bool = 11,
    AtModifiers = 12
}
export interface Context {
    kind: ContextKind;
    metricName?: string;
    labelName?: string;
    matchers?: Matcher[];
}
export declare function computeStartCompletePosition(node: SyntaxNode, pos: number): number;
export declare function analyzeCompletion(state: EditorState, node: SyntaxNode): Context[];
export declare class HybridComplete implements CompleteStrategy {
    private readonly prometheusClient;
    private readonly maxMetricsMetadata;
    constructor(prometheusClient?: PrometheusClient, maxMetricsMetadata?: number);
    promQL(context: CompletionContext): Promise<CompletionResult | null> | CompletionResult | null;
    private autocompleteMetricName;
    private autocompleteLabelName;
    private autocompleteLabelValue;
}
