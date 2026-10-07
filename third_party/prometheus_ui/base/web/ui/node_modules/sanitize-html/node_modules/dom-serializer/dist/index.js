/*
 * Module dependencies
 */
import * as ElementType from "domelementtype";
import { encodeXML, escapeAttribute, escapeText } from "entities";
/**
 * Mixed-case SVG and MathML tags & attributes
 * recognized by the HTML parser.
 * @see https://html.spec.whatwg.org/multipage/parsing.html#parsing-main-inforeign
 */
import { attributeNames, elementNames } from "./foreign-names.js";
const unencodedElements = new Set([
    "style",
    "script",
    "xmp",
    "iframe",
    "noembed",
    "noframes",
    "plaintext",
    "noscript",
]);
function replaceQuotes(value) {
    return value.replace(/"/g, "&quot;");
}
/**
 * Format attributes
 * @param attributes Attribute map to serialize.
 * @param options Options that control this operation.
 */
function formatAttributes(attributes, options) {
    if (!attributes)
        return;
    const encode = (options.encodeEntities ?? options.decodeEntities) === false
        ? replaceQuotes
        : !!options.xmlMode || options.encodeEntities !== "utf8"
            ? encodeXML
            : escapeAttribute;
    return Object.keys(attributes)
        .map((key) => {
        const value = attributes[key];
        const normalizedValue = value == null ? "" : String(value);
        if (options.xmlMode === "foreign") {
            /* Fix up mixed-case attribute names */
            key = attributeNames.get(key) ?? key;
        }
        if (!(options.emptyAttrs || options.xmlMode) && normalizedValue === "") {
            return key;
        }
        return `${key}="${encode(normalizedValue)}"`;
    })
        .join(" ");
}
/**
 * Self-enclosing tags
 */
const singleTag = new Set([
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
/**
 * Renders a DOM node or an array of DOM nodes to a string.
 *
 * Can be thought of as the equivalent of the `outerHTML` of the passed node(s).
 * @param node Node to be rendered.
 * @param options Changes serialization behavior
 */
export function render(node, options = {}) {
    const nodes = "length" in node ? node : [node];
    let output = "";
    let index = 0;
    while (index < nodes.length) {
        output += renderNode(nodes[index], options);
        index++;
    }
    return output;
}
export default render;
function renderNode(node, options) {
    switch (node.type) {
        case ElementType.Root: {
            return render(node.children, options);
        }
        // @ts-expect-error We don't use `Doctype` yet
        case ElementType.Doctype:
        case ElementType.Directive: {
            return renderDirective(node);
        }
        case ElementType.Comment: {
            return renderComment(node);
        }
        case ElementType.CDATA: {
            return renderCdata(node);
        }
        case ElementType.Script:
        case ElementType.Style:
        case ElementType.Tag: {
            return renderTag(node, options);
        }
        case ElementType.Text: {
            return renderText(node, options);
        }
    }
}
const foreignModeIntegrationPoints = new Set([
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
const foreignElements = new Set(["svg", "math"]);
function renderTag(element, options) {
    // Handle SVG / MathML in HTML
    if (options.xmlMode === "foreign") {
        /* Fix up mixed-case element names */
        element.name = elementNames.get(element.name) ?? element.name;
        /* Exit foreign mode at integration points */
        if (element.parent &&
            foreignModeIntegrationPoints.has(element.parent.name)) {
            options = { ...options, xmlMode: false };
        }
    }
    if (!options.xmlMode && foreignElements.has(element.name)) {
        options = { ...options, xmlMode: "foreign" };
    }
    let tag = `<${element.name}`;
    const attribs = formatAttributes(element.attribs, options);
    if (attribs) {
        tag += ` ${attribs}`;
    }
    if (element.children.length === 0 &&
        (options.xmlMode
            ? // In XML mode or foreign mode, and user hasn't explicitly turned off self-closing tags
                options.selfClosingTags !== false
            : // User explicitly asked for self-closing tags, even in HTML mode
                options.selfClosingTags && singleTag.has(element.name))) {
        if (!options.xmlMode)
            tag += " ";
        tag += "/>";
    }
    else {
        tag += ">";
        if (element.children.length > 0) {
            tag += render(element.children, options);
        }
        if (!!options.xmlMode || !singleTag.has(element.name)) {
            tag += `</${element.name}>`;
        }
    }
    return tag;
}
function renderDirective(element) {
    return `<${element.data}>`;
}
function renderText(element, options) {
    let data = element.data || "";
    // If entities weren't decoded, no need to encode them back
    if ((options.encodeEntities ?? options.decodeEntities) !== false &&
        !(!options.xmlMode &&
            element.parent &&
            unencodedElements.has(element.parent.name))) {
        data =
            !!options.xmlMode || options.encodeEntities !== "utf8"
                ? encodeXML(data)
                : escapeText(data);
    }
    return data;
}
function renderCdata(element) {
    return `<![CDATA[${element.children[0].data}]]>`;
}
function renderComment(element) {
    return `<!--${element.data}-->`;
}
