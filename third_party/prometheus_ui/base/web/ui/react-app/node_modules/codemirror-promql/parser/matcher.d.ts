import { SyntaxNode } from 'lezer-tree';
import { EditorState } from '@codemirror/state';
import { Matcher } from '../types/matcher';
export declare function buildLabelMatchers(labelMatchers: SyntaxNode[], state: EditorState): Matcher[];
export declare function labelMatchersToString(metricName: string, matchers?: Matcher[], labelName?: string): string;
