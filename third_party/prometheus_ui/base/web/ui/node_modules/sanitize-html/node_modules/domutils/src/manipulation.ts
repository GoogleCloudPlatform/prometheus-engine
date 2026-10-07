import type { ChildNode, ParentNode } from "domhandler";

/**
 * Remove an element from the dom
 *
 * @category Manipulation
 * @param element The element to be removed.
 */
export function removeElement(element: ChildNode): void {
    if (element.prev) element.prev.next = element.next;
    if (element.next) element.next.prev = element.prev;

    if (element.parent) {
        const childs = element.parent.children;
        const childsIndex = childs.lastIndexOf(element);
        if (childsIndex !== -1) {
            childs.splice(childsIndex, 1);
        }
    }
    element.next = null;
    element.prev = null;
    element.parent = null;
}

/**
 * Replace an element in the dom
 *
 * @category Manipulation
 * @param element The element to be replaced.
 * @param replacement The element to be added
 */
export function replaceElement(
    element: ChildNode,
    replacement: ChildNode,
): void {
    replacement.prev = element.prev;
    if (replacement.prev) {
        replacement.prev.next = replacement;
    }

    replacement.next = element.next;
    if (replacement.next) {
        replacement.next.prev = replacement;
    }

    replacement.parent = element.parent;
    if (replacement.parent) {
        const { children } = replacement.parent;
        const elementIndex = children.lastIndexOf(element);
        if (elementIndex === -1) {
            return;
        }

        children[elementIndex] = replacement;
        element.parent = null;
    }
}

/**
 * Append a child to an element.
 *
 * @category Manipulation
 * @param parent The element to append to.
 * @param child The element to be added as a child.
 */
export function appendChild(parent: ParentNode, child: ChildNode): void {
    removeElement(child);

    child.next = null;
    child.parent = parent;

    if (parent.children.push(child) > 1) {
        const sibling = parent.children[parent.children.length - 2];
        sibling.next = child;
        child.prev = sibling;
    } else {
        child.prev = null;
    }
}

/**
 * Append an element after another.
 *
 * @category Manipulation
 * @param element The element to append after.
 * @param next The element be added.
 */
export function append(element: ChildNode, next: ChildNode): void {
    removeElement(next);

    const { parent } = element;
    const currentNext = element.next;

    next.next = currentNext;
    next.prev = element;
    element.next = next;
    next.parent = parent;

    if (currentNext) {
        currentNext.prev = next;
        if (parent) {
            const childs = parent.children;
            childs.splice(childs.lastIndexOf(currentNext), 0, next);
        }
    } else if (parent) {
        parent.children.push(next);
    }
}

/**
 * Prepend a child to an element.
 *
 * @category Manipulation
 * @param parent The element to prepend before.
 * @param child The element to be added as a child.
 */
export function prependChild(parent: ParentNode, child: ChildNode): void {
    removeElement(child);

    child.parent = parent;
    child.prev = null;

    if (parent.children.unshift(child) === 1) {
        child.next = null;
    } else {
        const sibling = parent.children[1];
        sibling.prev = child;
        child.next = sibling;
    }
}

/**
 * Prepend an element before another.
 *
 * @category Manipulation
 * @param element The element to prepend before.
 * @param previous The element to be added.
 */
export function prepend(element: ChildNode, previous: ChildNode): void {
    removeElement(previous);

    const { parent } = element;
    if (parent) {
        const childs = parent.children;
        childs.splice(childs.indexOf(element), 0, previous);
    }

    if (element.prev) {
        element.prev.next = previous;
    }

    previous.parent = parent;
    previous.prev = element.prev;
    previous.next = element;
    element.prev = previous;
}
