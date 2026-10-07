/**
 * Quote style used for parsed attributes.
 */
export declare enum QuoteType {
    NoValue = 0,
    Unquoted = 1,
    Single = 2,
    Double = 3
}
/**
 * Low-level tokenizer callback interface.
 */
export interface Callbacks {
    onattribdata(start: number, endIndex: number): void;
    onattribentity(codepoint: number): void;
    onattribend(quote: QuoteType, endIndex: number): void;
    onattribname(start: number, endIndex: number): void;
    oncdata(start: number, endIndex: number, endOffset: number): void;
    onclosetag(start: number, endIndex: number): void;
    oncomment(start: number, endIndex: number, endOffset: number): void;
    ondeclaration(start: number, endIndex: number): void;
    onend(): void;
    onopentagend(endIndex: number): void;
    onopentagname(start: number, endIndex: number): void;
    onprocessinginstruction(start: number, endIndex: number): void;
    onselfclosingtag(endIndex: number): void;
    ontext(start: number, endIndex: number): void;
    ontextentity(codepoint: number, endIndex: number): void;
    isInForeignContext?(): boolean;
}
/**
 * Tokenizer implementation used by `Parser`.
 */
export default class Tokenizer {
    private readonly cbs;
    /** The current state the tokenizer is in. */
    private state;
    /** The read buffer. */
    private buffer;
    /** The beginning of the section that is currently being read. */
    private sectionStart;
    /** The index within the buffer that we are currently looking at. */
    private index;
    /** The start of the last entity. */
    private entityStart;
    /** Some behavior, eg. when decoding entities, is done while we are in another state. This keeps track of the other state type. */
    private baseState;
    /** For special parsing behavior inside of script and style tags. */
    private isSpecial;
    /** Indicates whether the tokenizer has been paused. */
    running: boolean;
    /** The offset of the current buffer. */
    private offset;
    private readonly xmlMode;
    private readonly decodeEntities;
    private readonly recognizeSelfClosing;
    private readonly entityDecoder;
    constructor({ xmlMode, decodeEntities, recognizeSelfClosing, }: {
        xmlMode?: boolean;
        decodeEntities?: boolean;
        recognizeSelfClosing?: boolean;
    }, cbs: Callbacks);
    reset(): void;
    write(chunk: string): void;
    end(): void;
    pause(): void;
    resume(): void;
    private stateText;
    private currentSequence;
    private sequenceIndex;
    private enterTagBody;
    /**
     * Match the opening tag name against an HTML text-only tag sequence.
     *
     * Some tags share an initial prefix (`script`/`style`, `title`/`textarea`,
     * `noembed`/`noframes`), so we may switch to an alternate sequence at the
     * first distinguishing byte.  On a successful full match we fall back to
     * the normal tag-name state; a later `>` will enter raw-text, RCDATA, or
     * plaintext mode based on `currentSequence` / `isSpecial`.
     * @param c Current character code point.
     */
    private stateSpecialStartSequence;
    private stateCDATASequence;
    /**
     * When we wait for one specific character, we can speed things up
     * by skipping through the buffer until we find it.
     * @param c Current character code point.
     * @returns Whether the character was found.
     */
    private fastForwardTo;
    /**
     * Emit a comment token and return to the text state.
     * @param offset Number of characters in the end sequence that have already been matched.
     */
    private emitComment;
    /**
     * Comments and CDATA end with `-->` and `]]>`.
     *
     * Their common qualities are:
     * - Their end sequences have a distinct character they start with.
     * - That character is then repeated, so we have to check multiple repeats.
     * - All characters but the start character of the sequence can be skipped.
     * @param c Current character code point.
     */
    private stateInCommentLike;
    /**
     * HTML only allows ASCII alpha characters (a-z and A-Z) at the beginning of a tag name.
     *
     * XML allows a lot more characters here (@see https://www.w3.org/TR/REC-xml/#NT-NameStartChar).
     * We allow anything that wouldn't end the tag.
     * @param c Current character code point.
     */
    private isTagStartChar;
    /**
     * Scan raw-text / RCDATA content for the matching end tag.
     *
     * For RCDATA tags (`<title>`, `<textarea>`) entities are decoded inline.
     * For raw-text tags (`<script>`, `<style>`, etc.) we fast-forward to `<`.
     * @param c Current character code point.
     */
    private stateInSpecialTag;
    private stateBeforeTagName;
    private stateInTagName;
    private stateBeforeClosingTagName;
    private stateInClosingTagName;
    private stateAfterClosingTagName;
    private stateBeforeAttributeName;
    /**
     * Handle `/` before `>` in an opening tag.
     *
     * In HTML mode, text-only tags ignore the self-closing flag and still enter
     * their raw-text/RCDATA/plaintext state unless self-closing tags are being
     * recognized. In XML mode, or for ordinary tags, the tokenizer returns to
     * regular text parsing after emitting the self-closing callback.
     * @param c Current character code point.
     */
    private stateInSelfClosingTag;
    private stateInAttributeName;
    private stateAfterAttributeName;
    private stateBeforeAttributeValue;
    private handleInAttributeValue;
    private stateInAttributeValueDoubleQuotes;
    private stateInAttributeValueSingleQuotes;
    private stateInAttributeValueNoQuotes;
    /**
     * Distinguish between CDATA, declarations, HTML comments, and HTML bogus
     * comments after `<!`.
     *
     * In HTML mode, only real comments and doctypes stay on declaration paths;
     * everything else becomes a bogus comment terminated by the next `>`.
     * @param c Current character code point.
     */
    private stateBeforeDeclaration;
    /**
     * Continue matching `doctype` after `<!d`.
     *
     * A full `doctype` match stays on the declaration path; any other name falls
     * back to an HTML bogus comment, which matches browser behavior for
     * non-doctype `<!...>` constructs.
     * @param c Current character code point.
     */
    private stateDeclarationSequence;
    private stateInDeclaration;
    /**
     * XML processing instructions (`<?...?>`).
     *
     * In HTML mode `<?` is routed to `InSpecialComment` instead, so this
     * state is only reachable in XML mode.
     * @param c Current character code point.
     */
    private stateInProcessingInstruction;
    private stateBeforeComment;
    private stateInSpecialComment;
    private startEntity;
    private stateInEntity;
    /**
     * Remove data that has already been consumed from the buffer.
     */
    private cleanup;
    private shouldContinue;
    /**
     * Iterates through the buffer, calling the function corresponding to the current state.
     *
     * States that are more likely to be hit are higher up, as a performance improvement.
     */
    private parse;
    private finish;
    private handleTrailingCommentLikeData;
    private handleTrailingMarkupDeclaration;
    /** Handle any trailing data. */
    private handleTrailingData;
    private emitCodePoint;
}
//# sourceMappingURL=Tokenizer.d.ts.map