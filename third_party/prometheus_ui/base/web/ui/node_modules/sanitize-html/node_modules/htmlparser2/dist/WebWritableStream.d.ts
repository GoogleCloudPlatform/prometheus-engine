import { type Handler, type ParserOptions } from "./Parser.js";
/**
 * WebWritableStream makes the `Parser` interface available as a Web Streams API WritableStream.
 *
 * This is useful for piping `fetch()` response bodies directly into the parser.
 * @see Parser
 * @example
 * ```typescript
 * import { WebWritableStream } from "htmlparser2/WebWritableStream";
 *
 * const stream = new WebWritableStream({
 *     onopentag(name, attribs) {
 *         console.log("Opened:", name);
 *     },
 * });
 *
 * const response = await fetch("https://example.com");
 * await response.body.pipeTo(stream);
 * ```
 */
export declare class WebWritableStream extends WritableStream<string | Uint8Array> {
    constructor(cbs: Partial<Handler>, options?: ParserOptions);
}
//# sourceMappingURL=WebWritableStream.d.ts.map