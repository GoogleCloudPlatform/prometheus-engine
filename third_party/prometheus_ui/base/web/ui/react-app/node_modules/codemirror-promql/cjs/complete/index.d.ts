import { PrometheusClient, PrometheusConfig } from '../client/prometheus';
import { CompletionContext, CompletionResult } from '@codemirror/autocomplete';
export interface CompleteStrategy {
    promQL(context: CompletionContext): Promise<CompletionResult | null> | CompletionResult | null;
}
export interface CompleteConfiguration {
    remote?: PrometheusConfig | PrometheusClient;
    maxMetricsMetadata?: number;
    completeStrategy?: CompleteStrategy;
}
export declare function newCompleteStrategy(conf?: CompleteConfiguration): CompleteStrategy;
