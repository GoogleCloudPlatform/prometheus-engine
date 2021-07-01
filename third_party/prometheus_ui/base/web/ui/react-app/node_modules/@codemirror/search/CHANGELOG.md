## 0.18.2 (2021-03-19)

### Bug fixes

The search interface and cursor will no longer include overlapping matches (aligning with what all other editors are doing).

### New features

The package now exports a `RegExpCursor` which is a search cursor that matches regular expression patterns.

The search/replace interface now allows the user to use regular expressions.

The `SearchCursor` class now has a `nextOverlapping` method that includes matches that start inside the previous match.

Basic backslash escapes (\n, \r, \t, and \\) are now accepted in string search patterns in the UI.

## 0.18.1 (2021-03-15)

### Bug fixes

Fix an issue where entering an invalid input in the goto-line dialog would submit a form and reload the page.

## 0.18.0 (2021-03-03)

### Breaking changes

Update dependencies to 0.18.

## 0.17.1 (2021-01-06)

### New features

The package now also exports a CommonJS module.

## 0.17.0 (2020-12-29)

### Breaking changes

First numbered release.

