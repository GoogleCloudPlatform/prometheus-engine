import { type AnyNode, type ChildNode, type Element, type ParentNode } from "domhandler";
/**
 * Get a node's children.
 *
 * @category Traversal
 * @param element Node to get the children of.
 * @returns `element`'s children, or an empty array.
 */
export declare function getChildren(element: AnyNode): ChildNode[];
/**
 * Get a node's parent.
 *
 * @category Traversal
 * @param element Node to get the parent of.
 * @returns `element`'s parent node, or `null` if `element` is a root node.
 */
export declare function getParent(element: AnyNode): ParentNode | null;
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
export declare function getSiblings(element: AnyNode): AnyNode[];
/**
 * Gets an attribute from an element.
 *
 * @category Traversal
 * @param element Element to check.
 * @param name Attribute name to retrieve.
 * @returns The element's attribute value, or `undefined`.
 */
export declare function getAttributeValue(element: Element, name: string): string | undefined;
/**
 * Checks whether an element has an attribute.
 *
 * @category Traversal
 * @param element Element to check.
 * @param name Attribute name to look for.
 * @returns Returns whether `element` has the attribute `name`.
 */
export declare function hasAttrib(element: Element, name: string): boolean;
/**
 * Get the tag name of an element.
 *
 * @category Traversal
 * @param element The element to get the name for.
 * @returns The tag name of `element`.
 */
export declare function getName(element: Element): string;
/**
 * Returns the next element sibling of a node.
 *
 * @category Traversal
 * @param element The element to get the next sibling of.
 * @returns `element`'s next sibling that is a tag, or `null` if there is no next
 * sibling.
 */
export declare function nextElementSibling(element: AnyNode): Element | null;
/**
 * Returns the previous element sibling of a node.
 *
 * @category Traversal
 * @param element The element to get the previous sibling of.
 * @returns `element`'s previous sibling that is a tag, or `null` if there is no
 * previous sibling.
 */
export declare function prevElementSibling(element: AnyNode): Element | null;
//# sourceMappingURL=traversal.d.ts.map