## 0.18.3 (2021-03-19)

### Breaking changes

The outer DOM element now has class `cm-editor` instead of `cm-wrap` (`cm-wrap` will be present as well until 0.19).

### Bug fixes

Improve behavior of `posAtCoords` when the position is near text but not in any character's actual box.

## 0.18.2 (2021-03-19)

### Bug fixes

Triple-clicking now selects the line break after the clicked line (if any).

Fix an issue where the `drawSelection` plugin would fail to draw the top line of the selection when it started in an empty line.

Fix an issue where, at the end of a specific type of composition on iOS, the editor read the DOM before the browser was done updating it.

## 0.18.1 (2021-03-05)

### Bug fixes

Fix an issue where, on iOS, some types of IME would cause the composed content to be deleted when confirming a composition.

## 0.18.0 (2021-03-03)

### Breaking changes

The `themeClass` function and ``-style selectors in themes are no longer supported (prefixing with `cm-` should be done manually now).

Themes must now use `&` (instead of an extra `$`) to target the editor wrapper element.

The editor no longer adds `cm-light` or `cm-dark` classes. Targeting light or dark configurations in base themes should now be done by using a `&light` or `&dark` top-level selector.

## 0.17.13 (2021-03-03)

### Bug fixes

Work around a Firefox bug where it won't draw the cursor when it is between uneditable elements.

Fix a bug that broke built-in mouse event handling.

## 0.17.12 (2021-03-02)

### Bug fixes

Avoid interfering with touch events, to allow native selection behavior.

Fix a bug that broke sub-selectors with multiple `&` placeholders in themes.

## 0.17.11 (2021-02-25)

### Bug fixes

Fix vertical cursor motion on Safari with a larger line-height.

Fix incorrect selection drawing (with `drawSelection`) when the selection spans to just after a soft wrap point.

Fix an issue where compositions on Safari were inappropriately aborted in some circumstances.

The view will now redraw when the `EditorView.phrases` facet changes, to make sure translated text is properly updated.

## 0.17.10 (2021-02-22)

### Bug fixes

Long words without spaces, when line-wrapping is enabled, are now properly broken.

Fix the horizontal position of selections drawn by `drawSelection` in right-to-left editors with a scrollbar.

## 0.17.9 (2021-02-18)

### Bug fixes

Fix an issue where pasting linewise at the start of a line left the cursor before the inserted content.

## 0.17.8 (2021-02-16)

### Bug fixes

Fix a problem where the DOM selection and the editor state could get out of sync in non-editable mode.

Fix a crash when the editor was hidden on Safari, due to `getClientRects` returning an empty list.

Prevent Firefox from making the scrollable element keyboard-focusable.

## 0.17.7 (2021-01-25)

### New features

Add an `EditorView.announce` state effect that can be used to conveniently provide screen reader announcements.

## 0.17.6 (2021-01-22)

### Bug fixes

Avoid creating very high scroll containers for large documents so that we don't overflow the DOM's fixed-precision numbers.

## 0.17.5 (2021-01-15)

### Bug fixes

Fix a bug that would create space-filling placeholders with incorrect height when document is very large.

## 0.17.4 (2021-01-14)

### Bug fixes

The `drawSelection` extension will now reuse cursor DOM nodes when the number of cursors stays the same, allowing some degree of cursor transition animation.

Makes highlighted special characters styleable (``) and fix their default look in dark themes to have appropriate contrast.

### New features

Adds a new `MatchDecorator` helper class which can be used to easily maintain decorations on content that matches a regular expression.

## 0.17.3 (2021-01-06)

### New features

The package now also exports a CommonJS module.

## 0.17.2 (2021-01-04)

### Bug fixes

Work around Chrome problem where the native shift-enter behavior inserts two line breaks.

Make bracket closing and bracket pair removing more reliable on Android.

Fix bad cursor position and superfluous change transactions after pressing enter when in a composition on Android.

Fix issue where the wrong character was deleted when backspacing out a character before an identical copy of that character on Android.

## 0.17.1 (2020-12-30)

### Bug fixes

Fix a bug that prevented `ViewUpdate.focusChanged` from ever being true.

## 0.17.0 (2020-12-29)

### Breaking changes

First numbered release.

