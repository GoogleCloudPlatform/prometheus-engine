import { type AnyNode, type Element, type ParentNode } from "domhandler";
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
export declare function filter(test: (element: AnyNode) => boolean, node: AnyNode | AnyNode[], recurse?: boolean, limit?: number): AnyNode[];
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
export declare function find(test: (element: AnyNode) => boolean, nodes: AnyNode[] | ParentNode, recurse: boolean, limit: number): AnyNode[];
/**
 * Finds one element in a tree that passes a test.
 *
 * @category Querying
 * @param test Function to test nodes on.
 * @param nodes Node or array of nodes to search.
 * @param recurse Also consider child nodes.
 * @returns The first node that passes `test`.
 */
export declare function findOne(test: (element: Element) => boolean, nodes: AnyNode[] | ParentNode, recurse?: boolean): Element | null;
/**
 * Checks if a tree of nodes contains at least one node passing a test.
 *
 * @category Querying
 * @param test Function to test nodes on.
 * @param nodes Array of nodes to search.
 * @returns Whether a tree of nodes contains at least one node passing the test.
 */
export declare function existsOne(test: (element: Element) => boolean, nodes: AnyNode[] | ParentNode): boolean;
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
export declare function findAll(test: (element: Element) => boolean, nodes: AnyNode[] | ParentNode): Element[];
//# sourceMappingURL=querying.d.ts.map