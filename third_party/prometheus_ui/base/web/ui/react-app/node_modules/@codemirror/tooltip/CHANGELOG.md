## 0.18.4 (2021-03-15)

### Breaking changes

It is no longer necessary to use the `tooltips` extension when using this package—just providing values through `showTooltip` will implicitly enable the necessary extensions.

Tooltips no longer use the `class` property on the spec object (just apply the class yourself when creating the DOM element).

### Bug fixes

Tooltips in a dark theme that doesn't explicitly style them no longer use the light theme defaults.

### New features

`showTooltip` now accepts null as input value, which doesn't produce a tooltip.

## 0.18.3 (2021-03-14)

### Bug fixes

Fix a crash in tooltip creation.

## 0.18.2 (2021-03-14)

### Bug fixes

Fix an issue where tooltips created in an out-of-view editor show up in the wrong place.

## 0.18.1 (2021-03-04)

### New features

The source callback to `hoverTooltip` may now return a promise.

## 0.18.0 (2021-03-03)

### Breaking changes

Extra CSS classes for tooltips must now be specified with the `class` option. The `style` option no longer exists.

## 0.17.2 (2021-01-14)

### Bug fixes

Fix tooltip positioning on iOS, which still handles position: fixed strangely.

## 0.17.1 (2021-01-06)

### New features

The package now also exports a CommonJS module.

## 0.17.0 (2020-12-29)

### Breaking changes

First numbered release.

