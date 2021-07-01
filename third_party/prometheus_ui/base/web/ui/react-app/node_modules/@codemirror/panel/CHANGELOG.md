## 0.18.1 (2021-03-15)

### Breaking changes

Panels no longer use the `class` property on the spec object (just apply the class yourself when creating the DOM element).

### New features

It is no longer necessary to call `panels()` to use the panel extension (`showPanel` automatically enables it).

`showPanel` now accepts null as input value, which doesn't produce a panel.

## 0.18.0 (2021-03-03)

### Breaking changes

Extra CSS classes for panels must now be specified with the `class` option. The `style` option no longer exists.

## 0.17.1 (2021-01-06)

### New features

The package now also exports a CommonJS module.

## 0.17.0 (2020-12-29)

### Breaking changes

First numbered release.

