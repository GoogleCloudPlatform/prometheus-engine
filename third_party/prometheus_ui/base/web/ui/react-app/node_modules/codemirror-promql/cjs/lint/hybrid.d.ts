import { LintStrategy } from './index';
import { EditorView } from '@codemirror/view';
import { Diagnostic } from '@codemirror/lint';
export declare class HybridLint implements LintStrategy {
    promQL(this: HybridLint): (view: EditorView) => readonly Diagnostic[];
}
