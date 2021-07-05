import { EditorView, Command, KeyBinding } from '@codemirror/view';
import { EditorState, TransactionSpec, Extension } from '@codemirror/state';

/**
Describes a problem or hint for a piece of code.
*/
interface Diagnostic {
    /**
    The start position of the relevant text.
    */
    from: number;
    /**
    The end position. May be equal to `from`, though actually
    covering text is preferable.
    */
    to: number;
    /**
    The severity of the problem. This will influence how it is
    displayed.
    */
    severity: "info" | "warning" | "error";
    /**
    An optional source string indicating where the diagnostic is
    coming from. You can put the name of your linter here, if
    applicable.
    */
    source?: string;
    /**
    The message associated with this diagnostic.
    */
    message: string;
    /**
    An optional array of actions that can be taken on this
    diagnostic.
    */
    actions?: readonly Action[];
}
/**
An action associated with a diagnostic.
*/
interface Action {
    /**
    The label to show to the user. Should be relatively short.
    */
    name: string;
    /**
    The function to call when the user activates this action. Is
    given the diagnostic's _current_ position, which may have
    changed since the creation of the diagnostic due to editing.
    */
    apply: (view: EditorView, from: number, to: number) => void;
}
/**
Returns a transaction spec which updates the current set of
diagnostics.
*/
declare function setDiagnostics(state: EditorState, diagnostics: readonly Diagnostic[]): TransactionSpec;
/**
Command to open and focus the lint panel.
*/
declare const openLintPanel: Command;
/**
Command to close the lint panel, when open.
*/
declare const closeLintPanel: Command;
/**
Move the selection to the next diagnostic.
*/
declare const nextDiagnostic: Command;
/**
A set of default key bindings for the lint functionality.

- Ctrl-Shift-m (Cmd-Shift-m on macOS): [`openLintPanel`](https://codemirror.net/6/docs/ref/#lint.openLintPanel)
- F8: [`nextDiagnostic`](https://codemirror.net/6/docs/ref/#lint.nextDiagnostic)
*/
declare const lintKeymap: readonly KeyBinding[];
/**
Given a diagnostic source, this function returns an extension that
enables linting with that source. It will be called whenever the
editor is idle (after its content changed).
*/
declare function linter(source: (view: EditorView) => readonly Diagnostic[] | Promise<readonly Diagnostic[]>, config?: {
    /**
    Time to wait (in milliseconds) after a change before running
    the linter. Defaults to 750ms.
    */
    delay?: number;
}): Extension;

export { Action, Diagnostic, closeLintPanel, lintKeymap, linter, nextDiagnostic, openLintPanel, setDiagnostics };
