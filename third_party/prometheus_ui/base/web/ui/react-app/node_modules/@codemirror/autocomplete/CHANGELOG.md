## 0.18.3 (2021-03-15)

### Bug fixes

Adjust to updated @codemirror/tooltip interface.

## 0.18.2 (2021-03-14)

### Bug fixes

Fix unintended ES2020 output (the package contains ES6 code again).

## 0.18.1 (2021-03-11)

### Bug fixes

Stop active completion when all sources resolve without producing any matches.

### New features

`Completion.info` may now return a promise.

## 0.18.0 (2021-03-03)

### Bug fixes

Only preserve selected option across updates when it isn't the first option.

## 0.17.4 (2021-01-18)

### Bug fixes

Fix a styling issue where the selection had become invisible inside snippet fields (when using `drawSelection`).

### New features

Snippet fields can now be selected with the pointing device (so that they are usable on touch devices).

## 0.17.3 (2021-01-18)

### Bug fixes

Fix a bug where uppercase completions would be incorrectly matched against the typed input.

## 0.17.2 (2021-01-12)

### Bug fixes

Don't bind Cmd-Space on macOS, since that already has a system default binding. Use Ctrl-Space for autocompletion.

## 0.17.1 (2021-01-06)

### New features

The package now also exports a CommonJS module.

## 0.17.0 (2020-12-29)

### Breaking changes

First numbered release.

