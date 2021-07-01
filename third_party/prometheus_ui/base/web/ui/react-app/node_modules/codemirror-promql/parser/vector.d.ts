import { EditorState } from '@codemirror/state';
import { SyntaxNode } from 'lezer-tree';
import { VectorMatching } from '../types/vector';
export declare function buildVectorMatching(state: EditorState, binaryNode: SyntaxNode): VectorMatching | null;
