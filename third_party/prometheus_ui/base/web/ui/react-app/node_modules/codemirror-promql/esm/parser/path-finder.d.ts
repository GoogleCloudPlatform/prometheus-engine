import { SyntaxNode } from 'lezer-tree';
export declare function walkBackward(node: SyntaxNode, exit: number): SyntaxNode | null;
export declare function walkThrough(node: SyntaxNode, ...path: (number | string)[]): SyntaxNode | null;
export declare function containsAtLeastOneChild(node: SyntaxNode, ...child: (number | string)[]): boolean;
export declare function containsChild(node: SyntaxNode, ...child: (number | string)[]): boolean;
export declare function retrieveAllRecursiveNodes(parentNode: SyntaxNode | null, recursiveNode: number, leaf: number): SyntaxNode[];
