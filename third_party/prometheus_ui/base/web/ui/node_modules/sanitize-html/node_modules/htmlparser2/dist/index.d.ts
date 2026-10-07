import { Parser, type ParserOptions } from "./Parser.js";
export type { Handler, ParserOptions } from "./Parser.js";
export { Parser } from "./Parser.js";
import { type Document, type DomHandlerOptions, type Element } from "domhandler";
export { DomHandler, DomHandler as DefaultHandler, type DomHandlerOptions, } from "domhandler";
/**
 * Combined parser and handler options.
 */
export type Options = ParserOptions & DomHandlerOptions;
/**
 * Parses the data, returns the resulting document.
 * @param data The data that should be parsed.
 * @param options Optional options for the parser and DOM handler.
 */
export declare function parseDocument(data: string, options?: Options): Document;
/**
 * Creates a parser instance, with an attached DOM handler.
 * @param callback A callback that will be called once parsing has been completed, with the resulting document.
 * @param options Optional options for the parser and DOM handler.
 * @param elementCallback An optional callback that will be called every time a tag has been completed inside of the DOM.
 */
export declare function createDocumentStream(callback: (error: Error | null, document: Document) => void, options?: Options, elementCallback?: (element: Element) => void): Parser;
export * as ElementType from "domelementtype";
export { type Callbacks as TokenizerCallbacks, default as Tokenizer, QuoteType, } from "./Tokenizer.js";
import { type Feed } from "domutils";
export { type Feed, getFeed } from "domutils";
/**
 * Parse a feed.
 * @param feed The feed that should be parsed, as a string.
 * @param options Optionally, options for parsing. When using this, you should set `xmlMode` to `true`.
 */
export declare function parseFeed(feed: string, options?: Options): Feed | null;
export * as DomUtils from "domutils";
//# sourceMappingURL=index.d.ts.map