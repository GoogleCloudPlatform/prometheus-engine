import { Extension } from '@codemirror/state';
import { CompleteConfiguration, CompleteStrategy } from './complete';
import { LintStrategy } from './lint';
import { LezerLanguage } from '@codemirror/language';
export declare const promQLLanguage: LezerLanguage;
/**
 * This class holds the state of the completion extension for CodeMirror and allow hot-swapping the complete strategy.
 */
export declare class PromQLExtension {
    private complete;
    private lint;
    private enableCompletion;
    private enableLinter;
    constructor();
    setComplete(conf?: CompleteConfiguration): PromQLExtension;
    getComplete(): CompleteStrategy;
    activateCompletion(activate: boolean): PromQLExtension;
    setLinter(linter: LintStrategy): PromQLExtension;
    getLinter(): LintStrategy;
    activateLinter(activate: boolean): PromQLExtension;
    asExtension(): Extension;
}
