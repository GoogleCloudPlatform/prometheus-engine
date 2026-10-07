import { hasChildren, isTag, } from "domhandler";
/**
 * Get a node's children.
 *
 * @category Traversal
 * @param element Node to get the children of.
 * @returns `element`'s children, or an empty array.
 */
export function getChildren(element) {
    return hasChildren(element) ? element.children : [];
}
export function getParent(element) {
    return element.parent || null;
}
/**
 * Gets an elements siblings, including the element itself.
 *
 * Attempts to get the children through the element's parent first. If we don't
 * have a parent (the element is a root node), we walk the element's `prev` &
 * `next` to get all remaining nodes.
 *
 * @category Traversal
 * @param element Element to get the siblings of.
 * @returns `element`'s siblings, including `element`.
 */
export function getSiblings(element) {
    const parent = getParent(element);
    if (parent != null)
        return getChildren(parent);
    const siblings = [element];
    let { prev, next } = element;
    while (prev != null) {
        siblings.unshift(prev);
        ({ prev } = prev);
    }
    while (next != null) {
        siblings.push(next);
        ({ next } = next);
    }
    return siblings;
}
/**
 * Gets an attribute from an element.
 *
 * @category Traversal
 * @param element Element to check.
 * @param name Attribute name to retrieve.
 * @returns The element's attribute value, or `undefined`.
 */
export function getAttributeValue(element, name) {
    const { attribs } = element;
    return attribs?.[name];
}
/**
 * Checks whether an element has an attribute.
 *
 * @category Traversal
 * @param element Element to check.
 * @param name Attribute name to look for.
 * @returns Returns whether `element` has the attribute `name`.
 */
export function hasAttrib(element, name) {
    const { attribs } = element;
    return (attribs != null && Object.hasOwn(attribs, name) && attribs[name] != null);
}
/**
 * Get the tag name of an element.
 *
 * @category Traversal
 * @param element The element to get the name for.
 * @returns The tag name of `element`.
 */
export function getName(element) {
    return element.name;
}
/**
 * Returns the next element sibling of a node.
 *
 * @category Traversal
 * @param element The element to get the next sibling of.
 * @returns `element`'s next sibling that is a tag, or `null` if there is no next
 * sibling.
 */
export function nextElementSibling(element) {
    let { next } = element;
    while (next !== null && !isTag(next))
        ({ next } = next);
    return next;
}
/**
 * Returns the previous element sibling of a node.
 *
 * @category Traversal
 * @param element The element to get the previous sibling of.
 * @returns `element`'s previous sibling that is a tag, or `null` if there is no
 * previous sibling.
 */
// eslint-disable-next-line unicorn/prevent-abbreviations -- Keep public API name for backwards compatibility.
export function prevElementSibling(element) {
    let { prev } = element;
    while (prev !== null && !isTag(prev))
        ({ prev } = prev);
    return prev;
}
//# sourceMappingURL=traversal.js.map