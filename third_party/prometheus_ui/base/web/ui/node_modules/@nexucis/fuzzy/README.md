Fuzzy
=====
[![CircleCI](https://circleci.com/gh/Nexucis/fuzzy.svg?style=shield)](https://circleci.com/gh/Nexucis/fuzzy) [![codecov](https://codecov.io/gh/Nexucis/fuzzy/branch/master/graph/badge.svg)](https://codecov.io/gh/Nexucis/fuzzy) 
[![NPM version](https://img.shields.io/npm/v/@nexucis/fuzzy.svg)](https://www.npmjs.com/package/@nexucis/fuzzy) [![GitHub license](https://img.shields.io/badge/license-MIT-blue.svg)](./LICENSE)

This lib provides a fuzzy search. It's inspired from the work of [Matt York](https://github.com/mattyork) on the repository [fuzzy](https://github.com/mattyork/fuzzy)

## Installation

```bash
npm install @nexucis/fuzzy
```

## Getting started

1. Filter a simple list of string

```typescript
import Fuzzy from '@nexucis/fuzzy'

const fuz = new Fuzzy()
const list = ['lion', 'goat', 'mouse', 'dragon']

console.log(fuz.filter('li', list))
// [
//   {rendered: 'lion', index: 0, score: 4, original: 'lion'},
// ]
//
```

2. Wrap matching characters in each string for highlighting

```typescript
import Fuzzy from '@nexucis/fuzzy'

const fuz = new Fuzzy({pre:'<b>', post:'</b>'})
const list = ['lion', 'goat', 'mouse', 'dragon']

console.log(fuz.filter('li', list))
// [
//   {rendered: '<b>li</b>on', index: 0, score: 4, original: 'lion'},
// ]
//
```

3. Include the list of indices of the matched characters to make your own highlight

```typescript
import Fuzzy from '@nexucis/fuzzy'

const fuz = new Fuzzy({includeMatches: true})
const list = ['lion', 'goat', 'mouse', 'dragon']

console.log(fuz.filter('li', list))
// [
//   {rendered: 'lion', index: 0, score: 4, original: 'lion', intervales:[{from:0, to:2}]},
// ]
//
```

4. Override locally the global configuration

```typescript
import Fuzzy from '@nexucis/fuzzy'

const fuz = new Fuzzy({includeMatches: true})
const list = ['lion', 'goat', 'mouse', 'dragon']

console.log(fuz.filter('li', list), {includeMatches: false})
// [
//   {rendered: 'lion', index: 0, score: 4, original: 'lion'},
// ]
//
```

## Available Options

**Note**: each option can be passed to the constructor or/and in each method exposed. 
The options passed in the method take precedence over the one passed in the contructor.

### caseSensitive

* **Type**: `boolean`
* **Default**: `false`

Indicates whether comparisons should be case-sensitive.

### includeMatches

* **Type**: `boolean`
* **Default**: `false`

Whether the matches should be included in the result. When true, each record in the result set will include the indices of the matched characters. 
These can consequently be used for highlighting purposes.

### shouldSort

* **Type**: `boolean`
* **Default**: `false`

Whether the result should be sorted

### escapeHTML

* **Type**: `boolean`
* **Default**: `false`

Whether the filtering should escape the HTML characters that can be found in each record in the result

### pre

* **Type**: `string`
* **Default**: `''`

Should be used to prefix each matched characters. Can be useful for the highlighting.

### post

* **Type**: `string`
* **Default**: `''`

Should be used to suffix each matched characters. Can be useful for the highlighting.

## Contributions
Any contribution or suggestion would be really appreciated. Feel free to [file an issue](https://github.com/Nexucis/fuzzy/issues) or [send a pull request](https://github.com/Nexucis/fuzzy/pulls).

## License
[MIT](./LICENSE)
