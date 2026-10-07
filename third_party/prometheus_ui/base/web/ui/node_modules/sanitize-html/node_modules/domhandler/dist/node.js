import { ElementType, isTag as isTagRaw } from "domelementtype";
/**
 * This object will be used as the prototype for Nodes when creating a
 * DOM-Level-1-compliant structure.
 */
export class Node {
    /** Parent of the node */
    parent = null;
    /** Previous sibling */
    prev = null;
    /** Next sibling */
    next = null;
    /** The start index of the node. Requires `withStartIndices` on the handler to be `true. */
    startIndex = null;
    /** The end index of the node. Requires `withEndIndices` on the handler to be `true. */
    endIndex = null;
    // Read-write aliases for properties
    /**
     * Same as {@link parent}.
     * [DOM spec](https://dom.spec.whatwg.org)-compatible alias.
     */
    get parentNode() {
        return this.parent;
    }
    set parentNode(parent) {
        this.parent = parent;
    }
    /**
     * Same as {@link prev}.
     * [DOM spec](https://dom.spec.whatwg.org)-compatible alias.
     */
    get previousSibling() {
        return this.prev;
    }
    set previousSibling(previous) {
        this.prev = previous;
    }
    /**
     * Same as {@link next}.
     * [DOM spec](https://dom.spec.whatwg.org)-compatible alias.
     */
    get nextSibling() {
        return this.next;
    }
    set nextSibling(next) {
        this.next = next;
    }
    /**
     * Clone this node, and optionally its children.
     * @param recursive Clone child nodes as well.
     * @returns A clone of the node.
     */
    cloneNode(recursive = false) {
        return cloneNode(this, recursive);
    }
}
/**
 * A node that contains some data.
 */
export class DataNode extends Node {
    data;
    /**
     * @param data The content of the data node
     */
    constructor(data) {
        super();
        this.data = data;
    }
    /**
     * Same as {@link data}.
     * [DOM spec](https://dom.spec.whatwg.org)-compatible alias.
     */
    get nodeValue() {
        return this.data;
    }
    set nodeValue(data) {
        this.data = data;
    }
}
/**
 * Text within the document.
 */
export class Text extends DataNode {
    type = ElementType.Text;
    get nodeType() {
        return 3;
    }
}
/**
 * Comments within the document.
 */
export class Comment extends DataNode {
    type = ElementType.Comment;
    get nodeType() {
        return 8;
    }
}
/**
 * Processing instructions, including doc types.
 */
export class ProcessingInstruction extends DataNode {
    type = ElementType.Directive;
    name;
    constructor(name, data) {
        super(data);
        this.name = name;
    }
    get nodeType() {
        return 1;
    }
    /** If this is a doctype, the document type name (parse5 only). */
    "x-name";
    /** If this is a doctype, the document type public identifier (parse5 only). */
    "x-publicId";
    /** If this is a doctype, the document type system identifier (parse5 only). */
    "x-systemId";
}
/**
 * A node that can have children.
 */
export class NodeWithChildren extends Node {
    children;
    /**
     * @param children Children of the node. Only certain node types can have children.
     */
    constructor(children) {
        super();
        this.children = children;
    }
    // Aliases
    /** First child of the node. */
    get firstChild() {
        return this.children[0] ?? null;
    }
    /** Last child of the node. */
    get lastChild() {
        return this.children.length > 0
            ? this.children[this.children.length - 1]
            : null;
    }
    /**
     * Same as {@link children}.
     * [DOM spec](https://dom.spec.whatwg.org)-compatible alias.
     */
    get childNodes() {
        return this.children;
    }
    set childNodes(children) {
        this.children = children;
    }
}
/**
 * CDATA nodes.
 */
export class CDATA extends NodeWithChildren {
    type = ElementType.CDATA;
    get nodeType() {
        return 4;
    }
}
/**
 * The root node of the document.
 */
export class Document extends NodeWithChildren {
    type = ElementType.Root;
    get nodeType() {
        return 9;
    }
}
/**
 * An element within the DOM.
 */
export class Element extends NodeWithChildren {
    name;
    attribs;
    type;
    /**
     * @param name Name of the tag, eg. `div`, `span`.
     * @param attribs Object mapping attribute names to attribute values.
     * @param children Children of the node.
     * @param type Node type used for the new node instance.
     */
    constructor(name, attribs, children = [], type = name === "script"
        ? ElementType.Script
        : name === "style"
            ? ElementType.Style
            : ElementType.Tag) {
        super(children);
        this.name = name;
        this.attribs = attribs;
        this.type = type;
    }
    get nodeType() {
        return 1;
    }
    // DOM Level 1 aliases
    /**
     * Same as {@link name}.
     * [DOM spec](https://dom.spec.whatwg.org)-compatible alias.
     */
    get tagName() {
        return this.name;
    }
    set tagName(name) {
        this.name = name;
    }
    get attributes() {
        return Object.keys(this.attribs).map((name) => ({
            name,
            value: this.attribs[name],
            namespace: this["x-attribsNamespace"]?.[name],
            prefix: this["x-attribsPrefix"]?.[name],
        }));
    }
    /** Element namespace (parse5 only). */
    namespace;
    /** Element attribute namespaces (parse5 only). */
    "x-attribsNamespace";
    /** Element attribute namespace-related prefixes (parse5 only). */
    "x-attribsPrefix";
}
/**
 * Checks if `node` is an element node.
 * @param node Node to check.
 * @returns `true` if the node is an element node.
 */
export function isTag(node) {
    return isTagRaw(node);
}
/**
 * Checks if `node` is a CDATA node.
 * @param node Node to check.
 * @returns `true` if the node is a CDATA node.
 */
export function isCDATA(node) {
    return node.type === ElementType.CDATA;
}
/**
 * Checks if `node` is a text node.
 * @param node Node to check.
 * @returns `true` if the node is a text node.
 */
export function isText(node) {
    return node.type === ElementType.Text;
}
/**
 * Checks if `node` is a comment node.
 * @param node Node to check.
 * @returns `true` if the node is a comment node.
 */
export function isComment(node) {
    return node.type === ElementType.Comment;
}
/**
 * Checks if `node` is a directive node.
 * @param node Node to check.
 * @returns `true` if the node is a directive node.
 */
export function isDirective(node) {
    return node.type === ElementType.Directive;
}
/**
 * Checks if `node` is a document node.
 * @param node Node to check.
 * @returns `true` if the node is a document node.
 */
export function isDocument(node) {
    return node.type === ElementType.Root;
}
/**
 * Checks if `node` has children.
 * @param node Node to check.
 * @returns `true` if the node has children.
 */
export function hasChildren(node) {
    return Object.hasOwn(node, "children");
}
/**
 * Clone a node, and optionally its children.
 * @param node Node to clone.
 * @param recursive Clone child nodes as well.
 * @returns A clone of the node.
 */
export function cloneNode(node, recursive = false) {
    let result;
    if (isText(node)) {
        result = new Text(node.data);
    }
    else if (isComment(node)) {
        result = new Comment(node.data);
    }
    else if (isTag(node)) {
        const children = recursive ? cloneChildren(node.children) : [];
        const clone = new Element(node.name, { ...node.attribs }, children);
        for (const child of children) {
            child.parent = clone;
        }
        if (node.namespace != null) {
            clone.namespace = node.namespace;
        }
        if (node["x-attribsNamespace"]) {
            clone["x-attribsNamespace"] = { ...node["x-attribsNamespace"] };
        }
        if (node["x-attribsPrefix"]) {
            clone["x-attribsPrefix"] = { ...node["x-attribsPrefix"] };
        }
        result = clone;
    }
    else if (isCDATA(node)) {
        const children = recursive ? cloneChildren(node.children) : [];
        const clone = new CDATA(children);
        for (const child of children) {
            child.parent = clone;
        }
        result = clone;
    }
    else if (isDocument(node)) {
        const children = recursive ? cloneChildren(node.children) : [];
        const clone = new Document(children);
        for (const child of children) {
            child.parent = clone;
        }
        if (node["x-mode"]) {
            clone["x-mode"] = node["x-mode"];
        }
        result = clone;
    }
    else if (isDirective(node)) {
        const instruction = new ProcessingInstruction(node.name, node.data);
        if (node["x-name"] != null) {
            instruction["x-name"] = node["x-name"];
            instruction["x-publicId"] = node["x-publicId"];
            instruction["x-systemId"] = node["x-systemId"];
        }
        result = instruction;
    }
    else {
        throw new Error(`Not implemented yet: ${node.type}`);
    }
    result.startIndex = node.startIndex;
    result.endIndex = node.endIndex;
    if (node.sourceCodeLocation != null) {
        result.sourceCodeLocation = node.sourceCodeLocation;
    }
    return result;
}
/**
 * Clone a list of child nodes.
 * @param childs The child nodes to clone.
 * @returns A list of cloned child nodes.
 */
function cloneChildren(childs) {
    const children = childs.map((child) => cloneNode(child, true));
    for (let index = 1; index < children.length; index++) {
        children[index].prev = children[index - 1];
        children[index - 1].next = children[index];
    }
    return children;
}
