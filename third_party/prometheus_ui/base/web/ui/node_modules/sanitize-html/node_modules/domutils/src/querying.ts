import {
    type AnyNode,
    type Element,
    hasChildren,
    isTag,
    type ParentNode,
} from "domhandler";

/**
 * Search a node and its children for nodes passing a test function. If `node` is not an array, it will be wrapped in one.
 *
 * @category Querying
 * @param test Function to test nodes on.
 * @param node Node to search. Will be included in the result set if it matches.
 * @param recurse Also consider child nodes.
 * @param limit Maximum number of nodes to return.
 * @returns All nodes passing `test`.
 */
export function filter(
    test: (element: AnyNode) => boolean,
    node: AnyNode | AnyNode[],
    recurse = true,
    limit: number = Number.POSITIVE_INFINITY,
): AnyNode[] {
    return find(test, Array.isArray(node) ? node : [node], recurse, limit);
}

/**
 * Search an array of nodes and their children for nodes passing a test function.
 *
 * @category Querying
 * @param test Function to test nodes on.
 * @param nodes Array of nodes to search.
 * @param recurse Also consider child nodes.
 * @param limit Maximum number of nodes to return.
 * @returns All nodes passing `test`.
 */
export function find(
    test: (element: AnyNode) => boolean,
    nodes: AnyNode[] | ParentNode,
    recurse: boolean,
    limit: number,
): AnyNode[] {
    const result: AnyNode[] = [];
    /** Stack of the arrays we are looking at. */
    const nodeStack: AnyNode[][] = [Array.isArray(nodes) ? nodes : [nodes]];
    /** Stack of the indices within the arrays. */
    const indexStack = [0];

    for (;;) {
        // First, check if the current array has any more elements to look at.
        if (indexStack[0] >= nodeStack[0].length) {
            // If we have no more arrays to look at, we are done.
            if (indexStack.length === 1) {
                return result;
            }

            // Otherwise, remove the current array from the stack.
            nodeStack.shift();
            indexStack.shift();

            // Loop back to the start to continue with the next array.
            continue;
        }

        const element = nodeStack[0][indexStack[0]++];

        if (test(element)) {
            result.push(element);
            if (--limit <= 0) return result;
        }

        if (recurse && hasChildren(element) && element.children.length > 0) {
            /*
             * Add the children to the stack. We are depth-first, so this is
             * the next array we look at.
             */
            indexStack.unshift(0);
            nodeStack.unshift(element.children);
        }
    }
}

/**
 * Finds one element in a tree that passes a test.
 *
 * @category Querying
 * @param test Function to test nodes on.
 * @param nodes Node or array of nodes to search.
 * @param recurse Also consider child nodes.
 * @returns The first node that passes `test`.
 */
export function findOne(
    test: (element: Element) => boolean,
    nodes: AnyNode[] | ParentNode,
    recurse = true,
): Element | null {
    const searchedNodes = Array.isArray(nodes) ? nodes : [nodes];
    for (const node of searchedNodes) {
        if (isTag(node) && test(node)) {
            return node;
        }
        if (recurse && hasChildren(node) && node.children.length > 0) {
            const found = findOne(test, node.children, true);
            if (found) return found;
        }
    }

    return null;
}

/**
 * Checks if a tree of nodes contains at least one node passing a test.
 *
 * @category Querying
 * @param test Function to test nodes on.
 * @param nodes Array of nodes to search.
 * @returns Whether a tree of nodes contains at least one node passing the test.
 */
export function existsOne(
    test: (element: Element) => boolean,
    nodes: AnyNode[] | ParentNode,
): boolean {
    return (Array.isArray(nodes) ? nodes : [nodes]).some(
        (node) =>
            (isTag(node) && test(node)) ||
            (hasChildren(node) && existsOne(test, node.children)),
    );
}

/**
 * Search an array of nodes and their children for elements passing a test function.
 *
 * Same as `find`, but limited to elements and with less options, leading to reduced complexity.
 *
 * @category Querying
 * @param test Function to test nodes on.
 * @param nodes Array of nodes to search.
 * @returns All nodes passing `test`.
 */
export function findAll(
    test: (element: Element) => boolean,
    nodes: AnyNode[] | ParentNode,
): Element[] {
    const result = [];
    const nodeStack = [Array.isArray(nodes) ? nodes : [nodes]];
    const indexStack = [0];

    for (;;) {
        if (indexStack[0] >= nodeStack[0].length) {
            if (nodeStack.length === 1) {
                return result;
            }

            // Otherwise, remove the current array from the stack.
            nodeStack.shift();
            indexStack.shift();

            // Loop back to the start to continue with the next array.
            continue;
        }

        const element = nodeStack[0][indexStack[0]++];

        if (isTag(element) && test(element)) result.push(element);

        if (hasChildren(element) && element.children.length > 0) {
            indexStack.unshift(0);
            nodeStack.unshift(element.children);
        }
    }
}
