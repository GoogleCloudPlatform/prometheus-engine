## 0.18.1 (2021-03-31)

### Breaking changes

`EditorParseContext.getSkippingParser` now replaces `EditorParseContext.skippingParser` and allows you to provide a promise that'll cause parsing to start again. (The old property remains available until the next major release.)

### Bug fixes

Fix an issue where nested parsers could see past the end of the nested region.

## 0.18.0 (2021-03-03)

### Breaking changes

Update dependencies to 0.18.

### Breaking changes

The `Language` constructor takes an additional argument that provides the top node type.

### New features

`Language` instances now have a `topNode` property giving their top node type.

`TreeIndentContext` now has a `continue` method that allows an indenter to defer to the indentation of the parent nodes.

## 0.17.5 (2021-02-19)

### New features

This package now exports a `foldInside` helper function, a fold function that should work for most delimited node types.

## 0.17.4 (2021-01-15)

## 0.17.3 (2021-01-15)

### Bug fixes

Parse scheduling has been improved to reduce the likelyhood of the user looking at unparsed code in big documents.

Prevent parser from running too far past the current viewport in huge documents.

## 0.17.2 (2021-01-06)

### New features

The package now also exports a CommonJS module.

## 0.17.1 (2020-12-30)

### Bug fixes

Fix a bug where changing the editor configuration wouldn't update the language parser used.

## 0.17.0 (2020-12-29)

### Breaking changes

First numbered release.

