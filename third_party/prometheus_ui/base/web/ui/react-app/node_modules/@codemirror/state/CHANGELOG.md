## 0.18.7 (2021-05-04)

### Bug fixes

Fix an issue where state fields might be initialized with a state that they aren't actually part of during reconfiguration.

## 0.18.6 (2021-04-12)

### New features

The new `EditorState.wordAt` method finds the word at a given position.

## 0.18.5 (2021-04-08)

### Bug fixes

Fix an issue in the compiled output that would break the code when minified with terser.

## 0.18.4 (2021-04-06)

### New features

The new `Transaction.remote` annotation can be used to mark and recognize transactions created by other actors.

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

