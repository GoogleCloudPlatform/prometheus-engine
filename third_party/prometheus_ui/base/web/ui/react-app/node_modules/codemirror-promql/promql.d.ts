import { Extension } from '@codemirror/state';
import { CompleteConfiguration } from './complete';
import { LezerLanguage } from '@codemirror/language';
export declare const promQLLanguage: LezerLanguage;
/**
 * This class holds the state of the completion extension for CodeMirror and allow hot-swapping the complete strategy.
 */
export declare class PromQLExtension {
    private complete;
    private readonly lint;
    private enableCompletion;
    private enableLinter;
    constructor();
    setComplete(conf?: CompleteConfiguration): PromQLExtension;
    activateCompletion(activate: boolean): PromQLExtension;
    activateLinter(activate: boolean): PromQLExtension;
    asExtension(): Extension;
}
