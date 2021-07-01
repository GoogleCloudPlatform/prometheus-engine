export declare const DefaultBufferLength = 1024;
export declare class NodeProp<T> {
    deserialize: (str: string) => T;
    constructor({ deserialize }?: {
        deserialize?: (str: string) => T;
    });
    static string(): NodeProp<string>;
    static number(): NodeProp<number>;
    static flag(): NodeProp<boolean>;
    set(propObj: {
        [prop: number]: any;
    }, value: T): {
        [prop: number]: any;
    };
    add(match: {
        [selector: string]: T;
    } | ((type: NodeType) => T | undefined)): NodePropSource;
    static closedBy: NodeProp<readonly string[]>;
    static openedBy: NodeProp<readonly string[]>;
    static group: NodeProp<readonly string[]>;
}
export declare type NodePropSource = (type: NodeType) => null | [NodeProp<any>, any];
export declare class NodeType {
    readonly name: string;
    readonly id: number;
    static define(spec: {
        id: number;
        name?: string;
        props?: readonly ([NodeProp<any>, any] | NodePropSource)[];
        top?: boolean;
        error?: boolean;
        skipped?: boolean;
    }): NodeType;
    prop<T>(prop: NodeProp<T>): T | undefined;
    get isTop(): boolean;
    get isSkipped(): boolean;
    get isError(): boolean;
    get isAnonymous(): boolean;
    is(name: string | number): boolean;
    static none: NodeType;
    static match<T>(map: {
        [selector: string]: T;
    }): (node: NodeType) => T | undefined;
}
export declare class NodeSet {
    readonly types: readonly NodeType[];
    constructor(types: readonly NodeType[]);
    extend(...props: NodePropSource[]): NodeSet;
}
export declare class Tree {
    readonly type: NodeType;
    readonly children: readonly (Tree | TreeBuffer)[];
    readonly positions: readonly number[];
    readonly length: number;
    constructor(type: NodeType, children: readonly (Tree | TreeBuffer)[], positions: readonly number[], length: number);
    static empty: Tree;
    cursor(pos?: number, side?: -1 | 0 | 1): TreeCursor;
    fullCursor(): TreeCursor;
    get topNode(): SyntaxNode;
    resolve(pos: number, side?: -1 | 0 | 1): SyntaxNode;
    iterate(spec: {
        enter(type: NodeType, from: number, to: number): false | void;
        leave?(type: NodeType, from: number, to: number): void;
        from?: number;
        to?: number;
    }): void;
    balance(maxBufferLength?: number): Tree;
    static build(data: BuildData): Tree;
}
declare type BuildData = {
    buffer: BufferCursor | readonly number[];
    nodeSet: NodeSet;
    topID?: number;
    start?: number;
    length?: number;
    maxBufferLength?: number;
    reused?: (Tree | TreeBuffer)[];
    minRepeatType?: number;
};
export declare class TreeBuffer {
    readonly length: number;
    readonly type: NodeType;
}
export interface SyntaxNode {
    type: NodeType;
    name: string;
    from: number;
    to: number;
    parent: SyntaxNode | null;
    firstChild: SyntaxNode | null;
    lastChild: SyntaxNode | null;
    childAfter(pos: number): SyntaxNode | null;
    childBefore(pos: number): SyntaxNode | null;
    nextSibling: SyntaxNode | null;
    prevSibling: SyntaxNode | null;
    cursor: TreeCursor;
    resolve(pos: number, side?: -1 | 0 | 1): SyntaxNode;
    getChild(type: string | number, before?: string | number | null, after?: string | number | null): SyntaxNode | null;
    getChildren(type: string | number, before?: string | number | null, after?: string | number | null): SyntaxNode[];
}
export declare class TreeCursor {
    readonly full: boolean;
    type: NodeType;
    get name(): string;
    from: number;
    to: number;
    private buffer;
    private stack;
    private index;
    private bufferNode;
    private yieldNode;
    private yieldBuf;
    private yield;
    firstChild(): boolean;
    lastChild(): boolean;
    childAfter(pos: number): boolean;
    childBefore(pos: number): boolean;
    parent(): boolean;
    nextSibling(): boolean;
    prevSibling(): boolean;
    private atLastNode;
    private move;
    next(): boolean;
    prev(): boolean;
    moveTo(pos: number, side?: -1 | 0 | 1): this;
    get node(): SyntaxNode;
    get tree(): Tree | null;
}
export interface BufferCursor {
    pos: number;
    id: number;
    start: number;
    end: number;
    size: number;
    next(): void;
    fork(): BufferCursor;
}
export interface ChangedRange {
    fromA: number;
    toA: number;
    fromB: number;
    toB: number;
}
export declare class TreeFragment {
    readonly from: number;
    readonly to: number;
    readonly tree: Tree;
    readonly offset: number;
    private open;
    constructor(from: number, to: number, tree: Tree, offset: number, open: number);
    get openStart(): boolean;
    get openEnd(): boolean;
    static applyChanges(fragments: readonly TreeFragment[], changes: readonly ChangedRange[], minGap?: number): readonly TreeFragment[];
    static addTree(tree: Tree, fragments?: readonly TreeFragment[], partial?: boolean): TreeFragment[];
}
export interface PartialParse {
    advance(): Tree | null;
    pos: number;
    forceFinish(): Tree;
}
export interface ParseContext {
    fragments?: readonly TreeFragment[];
}
export interface Input {
    length: number;
    get(pos: number): number;
    lineAfter(pos: number): string;
    read(from: number, to: number): string;
    clip(at: number): Input;
}
export declare function stringInput(input: string): Input;
export {};
