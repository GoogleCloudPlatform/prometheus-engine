import type { ChildNode, ParentNode } from "domhandler";
/**
 * Remove an element from the dom
 *
 * @category Manipulation
 * @param element The element to be removed.
 */
export declare function removeElement(element: ChildNode): void;
/**
 * Replace an element in the dom
 *
 * @category Manipulation
 * @param element The element to be replaced.
 * @param replacement The element to be added
 */
export declare function replaceElement(element: ChildNode, replacement: ChildNode): void;
/**
 * Append a child to an element.
 *
 * @category Manipulation
 * @param parent The element to append to.
 * @param child The element to be added as a child.
 */
export declare function appendChild(parent: ParentNode, child: ChildNode): void;
/**
 * Append an element after another.
 *
 * @category Manipulation
 * @param element The element to append after.
 * @param next The element be added.
 */
export declare function append(element: ChildNode, next: ChildNode): void;
/**
 * Prepend a child to an element.
 *
 * @category Manipulation
 * @param parent The element to prepend before.
 * @param child The element to be added as a child.
 */
export declare function prependChild(parent: ParentNode, child: ChildNode): void;
/**
 * Prepend an element before another.
 *
 * @category Manipulation
 * @param element The element to prepend before.
 * @param previous The element to be added.
 */
export declare function prepend(element: ChildNode, previous: ChildNode): void;
//# sourceMappingURL=manipulation.d.ts.map