## 0.18.3 (2021-03-04)

### New features

The `HighlightStyle.get` function can now be used to find the classes that the active highlighters assign to a given style tag.

## 0.18.1 (2021-03-04)

### Bug fixes

Fix a regression where the highlighter would walk the entire tree, not just the viewport, making editing of large documents slow.

## 0.18.0 (2021-03-03)

### Breaking changes

When multiple highlight styles are available in an editor, they will be combined, instead of the highest-precedence one overriding the others.

`HighlightStyle.define` now expects an array, not a variable list of arguments.

### New features

Highlight styles now have a `fallback` property that installs them as a fallback highlighter, which only takes effect if no other style is available.

It is now possible to map style tags to static class names in `HighlightStyle` definitions with the `class` property.

The new `classHighlightStyle` assigns a set of static classes to highlight tags, for use with external CSS.

Highlight styles can now be scoped per language.

## 0.17.3 (2021-02-25)

### New features

There is now a separate style tag for (XML-style) tag names (still a subtag of `typeName`).

## 0.17.2 (2021-01-28)

### Bug fixes

Fix an issue where the highlighter wouldn't re-highlight the code when the highlight style configuration changed.

## 0.17.1 (2021-01-06)

### New features

The package now also exports a CommonJS module.

## 0.17.0 (2020-12-29)

### Breaking changes

First numbered release.

