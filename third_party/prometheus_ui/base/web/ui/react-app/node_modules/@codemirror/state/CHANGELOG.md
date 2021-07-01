## 0.18.3 (2021-03-23)

### New features

The `ChangeDesc` class now has `toJSON` and `fromJSON` methods.

## 0.18.2 (2021-03-14)

### Bug fixes

Fix unintended ES2020 output (the package contains ES6 code again).

## 0.18.1 (2021-03-10)

### New features

The new `Compartment.get` method can be used to get the content of a compartment in a given state.

## 0.18.0 (2021-03-03)

### Breaking changes

`tagExtension` and the `reconfigure` transaction spec property have been replaced with the concept of configuration compartments and reconfiguration effects (see `Compartment`, `StateEffect.reconfigure`, and `StateEffect.appendConfig`).

## 0.17.2 (2021-02-19)

### New features

`EditorSelection.map` and `SelectionRange.map` now take an optional second argument to indicate which direction to map to.

## 0.17.1 (2021-01-06)

### New features

The package now also exports a CommonJS module.

## 0.17.0 (2020-12-29)

### Breaking changes

First numbered release.

