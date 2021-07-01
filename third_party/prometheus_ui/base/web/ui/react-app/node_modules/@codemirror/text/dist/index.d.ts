declare function findClusterBreak(str: string, pos: number, forward?: boolean): number;
declare function codePointAt(str: string, pos: number): number;
declare function fromCodePoint(code: number): string;
declare function codePointSize(code: number): 1 | 2;

declare function countColumn(string: string, n: number, tabSize: number): number;
declare function findColumn(string: string, n: number, col: number, tabSize: number): {
    offset: number;
    leftOver: number;
};

interface TextIterator extends Iterator<string> {
    next(skip?: number): this;
    value: string;
    done: boolean;
    lineBreak: boolean;
}
declare abstract class Text implements Iterable<string> {
    abstract readonly length: number;
    abstract readonly lines: number;
    [Symbol.iterator]: () => Iterator<string>;
    lineAt(pos: number): Line;
    line(n: number): Line;
    replace(from: number, to: number, text: Text): Text;
    append(other: Text): Text;
    slice(from: number, to?: number): Text;
    abstract sliceString(from: number, to?: number, lineSep?: string): string;
    eq(other: Text): boolean;
    iter(dir?: 1 | -1): TextIterator;
    iterRange(from: number, to?: number): TextIterator;
    toJSON(): string[];
    static of(text: readonly string[]): Text;
    static empty: Text;
}
declare class Line {
    readonly from: number;
    readonly to: number;
    readonly number: number;
    readonly text: string;
    get length(): number;
}

export { Line, Text, TextIterator, codePointAt, codePointSize, countColumn, findClusterBreak, findColumn, fromCodePoint };
