const { fromCodePoint } = String;
import Tokenizer, { QuoteType } from "./Tokenizer.js";
const formTags = new Set([
    "input",
    "option",
    "optgroup",
    "select",
    "button",
    "datalist",
    "textarea",
]);
const pTag = new Set(["p"]);
const headingTags = new Set(["h1", "h2", "h3", "h4", "h5", "h6", "p"]);
const tableSectionTags = new Set(["thead", "tbody"]);
const ddtTags = new Set(["dd", "dt"]);
const rtpTags = new Set(["rt", "rp"]);
const openImpliesClose = new Map([
    ["tr", new Set(["tr", "th", "td"])],
    ["th", new Set(["th"])],
    ["td", new Set(["thead", "th", "td"])],
    ["body", new Set(["head", "link", "script"])],
    ["a", new Set(["a"])],
    ["li", new Set(["li"])],
    ["p", pTag],
    ["h1", headingTags],
    ["h2", headingTags],
    ["h3", headingTags],
    ["h4", headingTags],
    ["h5", headingTags],
    ["h6", headingTags],
    ["select", formTags],
    ["input", formTags],
    ["output", formTags],
    ["button", formTags],
    ["datalist", formTags],
    ["textarea", formTags],
    ["option", new Set(["option"])],
    ["optgroup", new Set(["optgroup", "option"])],
    ["dd", ddtTags],
    ["dt", ddtTags],
    ["address", pTag],
    ["article", pTag],
    ["aside", pTag],
    ["blockquote", pTag],
    ["details", pTag],
    ["div", pTag],
    ["dl", pTag],
    ["fieldset", pTag],
    ["figcaption", pTag],
    ["figure", pTag],
    ["footer", pTag],
    ["form", pTag],
    ["header", pTag],
    ["hr", pTag],
    ["main", pTag],
    ["nav", pTag],
    ["ol", pTag],
    ["pre", pTag],
    ["section", pTag],
    ["table", pTag],
    ["ul", pTag],
    ["rt", rtpTags],
    ["rp", rtpTags],
    ["tbody", tableSectionTags],
    ["tfoot", tableSectionTags],
]);
const DOCUMENT_TYPE = "doctype";
const voidElements = new Set([
    "area",
    "base",
    "basefont",
    "br",
    "col",
    "command",
    "embed",
    "frame",
    "hr",
    "img",
    "input",
    "isindex",
    "keygen",
    "link",
    "meta",
    "param",
    "source",
    "track",
    "wbr",
]);
const foreignContextElements = new Set(["math", "svg"]);
/**
 * Elements that can be used to integrate HTML content within foreign namespaces (e.g., SVG or MathML).
 *
 * Entries must use the SVG-adjusted casing (e.g. "foreignObject" not
 * "foreignobject") since they are compared against adjusted tag names.
 */
const htmlIntegrationElements = new Set([
    "mi",
    "mo",
    "mn",
    "ms",
    "mtext",
    "annotation-xml",
    "foreignObject",
    "desc",
    "title",
]);
const svgTagNameAdjustments = new Map([
    ["altglyph", "altGlyph"],
    ["altglyphdef", "altGlyphDef"],
    ["altglyphitem", "altGlyphItem"],
    ["animatecolor", "animateColor"],
    ["animatemotion", "animateMotion"],
    ["animatetransform", "animateTransform"],
    ["clippath", "clipPath"],
    ["feblend", "feBlend"],
    ["fecolormatrix", "feColorMatrix"],
    ["fecomponenttransfer", "feComponentTransfer"],
    ["fecomposite", "feComposite"],
    ["feconvolvematrix", "feConvolveMatrix"],
    ["fediffuselighting", "feDiffuseLighting"],
    ["fedisplacementmap", "feDisplacementMap"],
    ["fedistantlight", "feDistantLight"],
    ["fedropshadow", "feDropShadow"],
    ["feflood", "feFlood"],
    ["fefunca", "feFuncA"],
    ["fefuncb", "feFuncB"],
    ["fefuncg", "feFuncG"],
    ["fefuncr", "feFuncR"],
    ["fegaussianblur", "feGaussianBlur"],
    ["feimage", "feImage"],
    ["femerge", "feMerge"],
    ["femergenode", "feMergeNode"],
    ["femorphology", "feMorphology"],
    ["feoffset", "feOffset"],
    ["fepointlight", "fePointLight"],
    ["fespecularlighting", "feSpecularLighting"],
    ["fespotlight", "feSpotLight"],
    ["fetile", "feTile"],
    ["feturbulence", "feTurbulence"],
    ["foreignobject", "foreignObject"],
    ["glyphref", "glyphRef"],
    ["lineargradient", "linearGradient"],
    ["radialgradient", "radialGradient"],
    ["textpath", "textPath"],
]);
var ForeignContext;
(function (ForeignContext) {
    ForeignContext[ForeignContext["None"] = 0] = "None";
    ForeignContext[ForeignContext["Svg"] = 1] = "Svg";
    ForeignContext[ForeignContext["MathML"] = 2] = "MathML";
})(ForeignContext || (ForeignContext = {}));
const reNameEnd = /\s|\//;
/**
 * Incremental parser implementation.
 */
export class Parser {
    options;
    /** The start index of the last event. */
    startIndex = 0;
    /** The end index of the last event. */
    endIndex = 0;
    /**
     * Store the start index of the current open tag,
     * so we can update the start index for attributes.
     */
    openTagStart = 0;
    tagname = "";
    attribname = "";
    attribvalue = "";
    attribs = null;
    stack = [];
    foreignContext;
    cbs;
    lowerCaseTagNames;
    lowerCaseAttributeNames;
    recognizeSelfClosing;
    /** We are parsing HTML. Inverse of the `xmlMode` option. */
    htmlMode;
    tokenizer;
    buffers = [];
    bufferOffset = 0;
    /** The index of the last written buffer. Used when resuming after a `pause()`. */
    writeIndex = 0;
    /** Indicates whether the parser has finished running / `.end` has been called. */
    ended = false;
    constructor(cbs, options = {}) {
        this.options = options;
        this.cbs = cbs ?? {};
        this.htmlMode = !this.options.xmlMode;
        this.lowerCaseTagNames = options.lowerCaseTags ?? this.htmlMode;
        this.lowerCaseAttributeNames =
            options.lowerCaseAttributeNames ?? this.htmlMode;
        this.recognizeSelfClosing =
            options.recognizeSelfClosing ?? !this.htmlMode;
        this.tokenizer = new (options.Tokenizer ?? Tokenizer)(this.options, this);
        this.foreignContext = [ForeignContext.None];
        this.cbs.onparserinit?.(this);
    }
    // Tokenizer event handlers
    /**
     * @param start Start index for the current parser event.
     * @param endIndex End index for the current parser event.
     * @internal
     */
    ontext(start, endIndex) {
        const data = this.getSlice(start, endIndex);
        this.endIndex = endIndex - 1;
        this.cbs.ontext?.(data);
        this.startIndex = endIndex;
    }
    /**
     * @param cp Current Unicode code point.
     * @param endIndex End index for the current parser event.
     * @internal
     */
    ontextentity(cp, endIndex) {
        this.endIndex = endIndex - 1;
        this.cbs.ontext?.(fromCodePoint(cp));
        this.startIndex = endIndex;
    }
    /** @internal */
    isInForeignContext() {
        return this.foreignContext[0] !== ForeignContext.None;
    }
    /**
     * Checks if the current tag is a void element. Override this if you want
     * to specify your own additional void elements.
     * @param name Name of the pseudo selector.
     */
    isVoidElement(name) {
        return this.htmlMode && voidElements.has(name);
    }
    /**
     * Read a tag name from the buffer.
     *
     * When `lowerCaseTagNames` is enabled (the default in HTML mode), the name
     * is lowercased and may be adjusted for SVG casing or the `image` → `img`
     * alias.
     * @param start Start index of the tag name in the buffer.
     * @param endIndex End index of the tag name in the buffer.
     */
    readTagName(start, endIndex) {
        const name = this.lowerCaseTagNames
            ? this.getSlice(start, endIndex).toLowerCase()
            : this.getSlice(start, endIndex);
        if (!(this.lowerCaseTagNames && this.htmlMode)) {
            return name;
        }
        if (this.foreignContext[0] === ForeignContext.Svg) {
            return svgTagNameAdjustments.get(name) ?? name;
        }
        /*
         * Closing tags for SVG elements inside HTML integration points
         * (e.g. </foreignObject> while inside its own content) need case
         * adjustment so the name matches what was pushed to the stack.
         * `foreignContext.length > 1` means a foreign ancestor exists —
         * the base [None] entry plus at least one pushed context.
         */
        if (this.foreignContext.length > 1) {
            const adjusted = svgTagNameAdjustments.get(name);
            if (adjusted !== undefined && this.stack.includes(adjusted)) {
                return adjusted;
            }
        }
        if (!this.isInForeignContext()) {
            return name === "image" ? "img" : name;
        }
        return name;
    }
    /**
     * @param start Start index for the current parser event.
     * @param endIndex End index for the current parser event.
     * @internal
     */
    onopentagname(start, endIndex) {
        this.endIndex = endIndex;
        this.emitOpenTag(this.readTagName(start, endIndex));
    }
    emitOpenTag(name) {
        this.openTagStart = this.startIndex;
        this.tagname = name;
        /*
         * The spec ignores a second <form> when one is already open.
         * Setting tagname to "" suppresses all downstream effects: attribs
         * stays null so endOpenTag is a no-op, and closeCurrentTag can't
         * match "" on the stack.
         */
        if (this.htmlMode && name === "form" && this.stack.includes("form")) {
            this.tagname = "";
            return;
        }
        const impliesClose = this.htmlMode && openImpliesClose.get(name);
        if (impliesClose) {
            while (this.stack.length > 0 && impliesClose.has(this.stack[0])) {
                this.popElement(true);
            }
        }
        if (!this.isVoidElement(name)) {
            this.stack.unshift(name);
            if (this.htmlMode) {
                if (name === "svg") {
                    this.foreignContext.unshift(ForeignContext.Svg);
                }
                else if (name === "math") {
                    this.foreignContext.unshift(ForeignContext.MathML);
                }
                else if (htmlIntegrationElements.has(name)) {
                    this.foreignContext.unshift(ForeignContext.None);
                }
            }
        }
        this.cbs.onopentagname?.(name);
        if (this.cbs.onopentag)
            this.attribs = {};
    }
    endOpenTag(isImplied) {
        this.startIndex = this.openTagStart;
        if (this.attribs) {
            this.cbs.onopentag?.(this.tagname, this.attribs, isImplied);
            this.attribs = null;
        }
        if (this.cbs.onclosetag && this.isVoidElement(this.tagname)) {
            this.cbs.onclosetag(this.tagname, true);
        }
        this.tagname = "";
    }
    /**
     * @param endIndex End index for the current parser event.
     * @internal
     */
    onopentagend(endIndex) {
        this.endIndex = endIndex;
        this.endOpenTag(false);
        // Set `startIndex` for next node
        this.startIndex = endIndex + 1;
    }
    /**
     * @param start Start index for the current parser event.
     * @param endIndex End index for the current parser event.
     * @internal
     */
    onclosetag(start, endIndex) {
        this.endIndex = endIndex;
        const name = this.readTagName(start, endIndex);
        if (!this.isVoidElement(name)) {
            const pos = this.stack.indexOf(name);
            if (pos !== -1) {
                for (let index = 0; index < pos; index++) {
                    this.popElement(true);
                }
                this.popElement(false);
            }
            else if (this.htmlMode && name === "p") {
                // Implicit open before close
                this.emitOpenTag("p");
                this.closeCurrentTag(true);
            }
        }
        else if (this.htmlMode && name === "br") {
            // We can't use `emitOpenTag` for implicit open, as `br` would be implicitly closed.
            this.cbs.onopentagname?.("br");
            this.cbs.onopentag?.("br", {}, true);
            this.cbs.onclosetag?.("br", false);
        }
        // Set `startIndex` for next node
        this.startIndex = endIndex + 1;
    }
    /**
     * @param endIndex End index for the current parser event.
     * @internal
     */
    onselfclosingtag(endIndex) {
        this.endIndex = endIndex;
        if (this.recognizeSelfClosing || this.isInForeignContext()) {
            this.closeCurrentTag(false);
            // Set `startIndex` for next node
            this.startIndex = endIndex + 1;
        }
        else {
            // Ignore the fact that the tag is self-closing.
            this.onopentagend(endIndex);
        }
    }
    /**
     * Pop the top element off the stack, emit a close event, and maintain
     * the foreign context stack.
     * @param implied Whether this close is implied (not from an explicit end tag).
     */
    popElement(implied) {
        // biome-ignore lint/style/noNonNullAssertion: The element is guaranteed to exist.
        const element = this.stack.shift();
        if (this.htmlMode &&
            (foreignContextElements.has(element) ||
                htmlIntegrationElements.has(element))) {
            this.foreignContext.shift();
        }
        this.cbs.onclosetag?.(element, implied);
    }
    closeCurrentTag(isOpenImplied) {
        const name = this.tagname;
        this.endOpenTag(isOpenImplied);
        // Self-closing tags will be on the top of the stack
        if (this.stack[0] === name) {
            this.popElement(!isOpenImplied);
        }
    }
    /**
     * @param start Start index for the current parser event.
     * @param endIndex End index for the current parser event.
     * @internal
     */
    onattribname(start, endIndex) {
        this.startIndex = start;
        const name = this.getSlice(start, endIndex);
        this.attribname = this.lowerCaseAttributeNames
            ? name.toLowerCase()
            : name;
    }
    /**
     * @param start Start index for the current parser event.
     * @param endIndex End index for the current parser event.
     * @internal
     */
    onattribdata(start, endIndex) {
        this.attribvalue += this.getSlice(start, endIndex);
    }
    /**
     * @param cp Current Unicode code point.
     * @internal
     */
    onattribentity(cp) {
        this.attribvalue += fromCodePoint(cp);
    }
    /**
     * @param quote Quote type used for the current attribute.
     * @param endIndex End index for the current parser event.
     * @internal
     */
    onattribend(quote, endIndex) {
        this.endIndex = endIndex;
        this.cbs.onattribute?.(this.attribname, this.attribvalue, quote === QuoteType.Double
            ? '"'
            : quote === QuoteType.Single
                ? "'"
                : quote === QuoteType.NoValue
                    ? undefined
                    : null);
        if (this.attribs && !Object.hasOwn(this.attribs, this.attribname)) {
            this.attribs[this.attribname] = this.attribvalue;
        }
        this.attribvalue = "";
    }
    getInstructionName(value) {
        const index = value.search(reNameEnd);
        let name = index < 0 ? value : value.substr(0, index);
        if (this.lowerCaseTagNames) {
            name = name.toLowerCase();
        }
        return name;
    }
    /**
     * @param start Start index for the current parser event.
     * @param endIndex End index for the current parser event.
     * @internal
     */
    ondeclaration(start, endIndex) {
        this.endIndex = endIndex;
        const value = this.getSlice(start, endIndex);
        if (this.cbs.onprocessinginstruction) {
            /*
             * In HTML mode, ondeclaration is only reached for DOCTYPE
             * (the tokenizer routes everything else to bogus comments).
             */
            const name = this.htmlMode
                ? this.lowerCaseTagNames
                    ? DOCUMENT_TYPE
                    : value.slice(0, DOCUMENT_TYPE.length)
                : this.getInstructionName(value);
            this.cbs.onprocessinginstruction(`!${name}`, `!${value}`);
        }
        // Set `startIndex` for next node
        this.startIndex = endIndex + 1;
    }
    /**
     * @param start Start index for the current parser event.
     * @param endIndex End index for the current parser event.
     * @internal
     */
    onprocessinginstruction(start, endIndex) {
        this.endIndex = endIndex;
        const value = this.getSlice(start, endIndex);
        if (this.cbs.onprocessinginstruction) {
            const name = this.getInstructionName(value);
            this.cbs.onprocessinginstruction(`?${name}`, `?${value}`);
        }
        // Set `startIndex` for next node
        this.startIndex = endIndex + 1;
    }
    /**
     * @param start Start index for the current parser event.
     * @param endIndex End index for the current parser event.
     * @param offset Offset applied when computing parser indices.
     * @internal
     */
    oncomment(start, endIndex, offset) {
        this.endIndex = endIndex;
        this.cbs.oncomment?.(this.getSlice(start, endIndex - offset));
        this.cbs.oncommentend?.();
        // Set `startIndex` for next node
        this.startIndex = endIndex + 1;
    }
    /**
     * @param start Start index for the current parser event.
     * @param endIndex End index for the current parser event.
     * @param offset Offset applied when computing parser indices.
     * @internal
     */
    oncdata(start, endIndex, offset) {
        this.endIndex = endIndex;
        const value = this.getSlice(start, endIndex - offset);
        if (!this.htmlMode || this.options.recognizeCDATA) {
            this.cbs.oncdatastart?.();
            this.cbs.ontext?.(value);
            this.cbs.oncdataend?.();
        }
        else if (this.isInForeignContext()) {
            this.cbs.ontext?.(value);
        }
        else {
            this.cbs.oncomment?.(`[CDATA[${value}]]`);
            this.cbs.oncommentend?.();
        }
        // Set `startIndex` for next node
        this.startIndex = endIndex + 1;
    }
    /** @internal */
    onend() {
        if (this.cbs.onclosetag) {
            // Set the end index for all remaining tags
            this.endIndex = this.startIndex;
            for (let index = 0; index < this.stack.length; index++) {
                this.cbs.onclosetag(this.stack[index], true);
            }
        }
        this.cbs.onend?.();
    }
    /**
     * Resets the parser to a blank state, ready to parse a new HTML document
     */
    reset() {
        this.cbs.onreset?.();
        this.tokenizer.reset();
        this.tagname = "";
        this.attribname = "";
        this.attribvalue = "";
        this.attribs = null;
        this.stack.length = 0;
        this.startIndex = 0;
        this.endIndex = 0;
        this.cbs.onparserinit?.(this);
        this.buffers.length = 0;
        this.foreignContext.length = 0;
        this.foreignContext.unshift(ForeignContext.None);
        this.bufferOffset = 0;
        this.writeIndex = 0;
        this.ended = false;
    }
    /**
     * Resets the parser, then parses a complete document and
     * pushes it to the handler.
     * @param data Document to parse.
     */
    parseComplete(data) {
        this.reset();
        this.end(data);
    }
    getSlice(start, end) {
        if (start === end) {
            return "";
        }
        while (start - this.bufferOffset >= this.buffers[0].length) {
            this.shiftBuffer();
        }
        let slice = this.buffers[0].slice(start - this.bufferOffset, end - this.bufferOffset);
        while (end - this.bufferOffset > this.buffers[0].length) {
            this.shiftBuffer();
            slice += this.buffers[0].slice(0, end - this.bufferOffset);
        }
        return slice;
    }
    shiftBuffer() {
        this.bufferOffset += this.buffers[0].length;
        this.writeIndex--;
        this.buffers.shift();
    }
    /**
     * Parses a chunk of data and calls the corresponding callbacks.
     * @param chunk Chunk to parse.
     */
    write(chunk) {
        if (this.ended) {
            this.cbs.onerror?.(new Error(".write() after done!"));
            return;
        }
        this.buffers.push(chunk);
        if (this.tokenizer.running) {
            this.tokenizer.write(chunk);
            this.writeIndex++;
        }
    }
    /**
     * Parses the end of the buffer and clears the stack, calls onend.
     * @param chunk Optional final chunk to parse.
     */
    end(chunk) {
        if (this.ended) {
            this.cbs.onerror?.(new Error(".end() after done!"));
            return;
        }
        if (chunk)
            this.write(chunk);
        this.ended = true;
        this.tokenizer.end();
    }
    /**
     * Pauses parsing. The parser won't emit events until `resume` is called.
     */
    pause() {
        this.tokenizer.pause();
    }
    /**
     * Resumes parsing after `pause` was called.
     */
    resume() {
        this.tokenizer.resume();
        while (this.tokenizer.running &&
            this.writeIndex < this.buffers.length) {
            this.tokenizer.write(this.buffers[this.writeIndex++]);
        }
        if (this.ended)
            this.tokenizer.end();
    }
}
//# sourceMappingURL=Parser.js.map