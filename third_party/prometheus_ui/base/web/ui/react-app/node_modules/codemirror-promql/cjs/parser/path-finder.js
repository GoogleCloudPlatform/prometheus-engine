"use strict";
// The MIT License (MIT)
//
// Copyright (c) 2020 The Prometheus Authors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.
Object.defineProperty(exports, "__esModule", { value: true });
exports.retrieveAllRecursiveNodes = exports.containsChild = exports.containsAtLeastOneChild = exports.walkThrough = exports.walkBackward = void 0;
// walkBackward will iterate other the tree from the leaf to the root until it founds the given `exit` node.
// It returns null if the exit is not found.
function walkBackward(node, exit) {
    var cursor = node.cursor;
    var cursorIsMoving = true;
    while (cursorIsMoving && cursor.type.id !== exit) {
        cursorIsMoving = cursor.parent();
    }
    return cursor.type.id === exit ? cursor.node : null;
}
exports.walkBackward = walkBackward;
// walkThrough is going to follow the path passed in parameter.
// If it succeeds to reach the last id/name of the path, then it will return the corresponding Subtree.
// Otherwise if it's not possible to reach the last id/name of the path, it will return `null`
// Note: the way followed during the iteration of the tree to find the given path, is only from the root to the leaf.
function walkThrough(node) {
    var path = [];
    for (var _i = 1; _i < arguments.length; _i++) {
        path[_i - 1] = arguments[_i];
    }
    var cursor = node.cursor;
    var i = 0;
    var cursorIsMoving = true;
    path.unshift(cursor.type.id);
    while (i < path.length && cursorIsMoving) {
        if (cursor.type.id === path[i] || cursor.type.name === path[i]) {
            i++;
            if (i < path.length) {
                cursorIsMoving = cursor.next();
            }
        }
        else {
            cursorIsMoving = cursor.nextSibling();
        }
    }
    if (i >= path.length) {
        return cursor.node;
    }
    return null;
}
exports.walkThrough = walkThrough;
function containsAtLeastOneChild(node) {
    var child = [];
    for (var _i = 1; _i < arguments.length; _i++) {
        child[_i - 1] = arguments[_i];
    }
    var cursor = node.cursor;
    if (!cursor.next()) {
        // let's try to move directly to the children level and
        // return false immediately if the current node doesn't have any child
        return false;
    }
    var result = false;
    do {
        result = child.some(function (n) { return cursor.type.id === n || cursor.type.name === n; });
    } while (!result && cursor.nextSibling());
    return result;
}
exports.containsAtLeastOneChild = containsAtLeastOneChild;
function containsChild(node) {
    var child = [];
    for (var _i = 1; _i < arguments.length; _i++) {
        child[_i - 1] = arguments[_i];
    }
    var cursor = node.cursor;
    if (!cursor.next()) {
        // let's try to move directly to the children level and
        // return false immediately if the current node doesn't have any child
        return false;
    }
    var i = 0;
    do {
        if (cursor.type.id === child[i] || cursor.type.name === child[i]) {
            i++;
        }
    } while (i < child.length && cursor.nextSibling());
    return i >= child.length;
}
exports.containsChild = containsChild;
function retrieveAllRecursiveNodes(parentNode, recursiveNode, leaf) {
    var nodes = [];
    function recursiveRetrieveNode(node, nodes) {
        var subNode = node === null || node === void 0 ? void 0 : node.getChild(recursiveNode);
        var le = node === null || node === void 0 ? void 0 : node.lastChild;
        if (subNode && subNode.type.id === recursiveNode) {
            recursiveRetrieveNode(subNode, nodes);
        }
        if (le && le.type.id === leaf) {
            nodes.push(le);
        }
    }
    recursiveRetrieveNode(parentNode, nodes);
    return nodes;
}
exports.retrieveAllRecursiveNodes = retrieveAllRecursiveNodes;
//# sourceMappingURL=path-finder.js.map