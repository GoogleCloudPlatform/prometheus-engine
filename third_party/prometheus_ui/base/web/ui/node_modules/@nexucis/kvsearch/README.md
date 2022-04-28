KVSearch
========

## Installation

```bash
npm install @nexucis/kvsearch
```

## Usage

1. Filter a list of object using a list of index

```typescript
import { KVSearch } from '@nexucis/kvsearch';

const list = [
    {
        labels: { instance: 'demo.org', job: 'demo' },
        scrapePool: 'scrapePool demo'
    },
    {
        labels: { instance: 'k8s.org', job: 'constellation' },
        scrapePool: 'galaxy'
    }
]
const search = new KVSearch({
    indexedKeys: [
        'labels',
        'scrapePool',
    ],
});
search.filter('demo', list) // will match the first object
search.filter('constellation', list) // won't match any object present in the list, since the attribute `labels.jop` is not indexed
```

Here the indexed list says:

* Since `labels` value is an object, then check if the `pattern` is matching a key in the object return by `labels`
* Same thing for `scrapePool`, excepting it returns a string, so the code won't loop other a list of key, it will just
  check if the value of `scrapePool` is matching the `pattern`

Note that the matching is using the lib `@nexucis/fuzzy`, so it's not an exact match used.

2. Filter a list of object using a list of index, with some regexp

```typescript
import { KVSearch } from '@nexucis/kvsearch';

const list = [
    {
        labels: { instance: 'demo.org', job: 'demo' },
        scrapePool: 'scrapePool demo'
    },
    {
        labels: { instance: 'k8s.org', job: 'constellation' },
        scrapePool: 'galaxy'
    }
]
const search = new KVSearch({
    indexedKeys: [
        'labels',
        ['labels', /.*/],
        'scrapePool',
    ],
});
search.filter('constel', list) // will match only the 2nd object
```

The difference here is we indexed the attributes of the `labels` object. In this example, by using the Regexp `/.*/` we
indexed every attribute of the object `labels`. That's why the pattern `constellation` is matching the second object.
But we could also just index the field `job` of the `labels` like that `['labels', 'job']`. It would have worked as
well.

3. Filter a list of object using a specific query.

Using a list of index is simple, but it always used a fuzzy match. Probably sometimes you would like to do an exact
match or a negative match depending on the context.

You can do it by creating your own query like that:

```typescript
import { KVSearch } from '@nexucis/kvsearch';

const list = [
    {
        labels: { instance: 'demo.org', job: 'demo' },
        scrapePool: 'scrapePool demo'
    },
    {
        labels: { instance: 'k8s.org', job: 'constellation' },
        scrapePool: 'galaxy'
    }
]
const search = new KVSearch();
search.filterWithQuery({ keyPath: ['labels', /.*/], match: 'exact', pattern: 'constellation' })
```

4. Filter a list of object using a complex query.

It's possible to combine query together, so you can write multiple conditions.

```typescript
import { KVSearch } from '@nexucis/kvsearch';

const list = [
    {
        labels: { instance: 'demo.org', job: 'demo' },
        scrapePool: 'scrapePool demo'
    },
    {
        labels: { instance: 'k8s.org', job: 'constellation' },
        scrapePool: 'galaxy'
    },
    {
        labels: { instance: 'awx.com', job: 'constellation' },
        scrapePool: 'galaxy',
    }
]
const search = new KVSearch();
search.filterWithQuery({
    operator: 'and',
    left: {
        keyPath: ['scrapePool'],
        match: 'fuzzy',
        pattern: 'gal'
    },
    right: {
        keyPath: ['labels', 'instance'],
        match: 'exact',
        pattern: 'awx.com'
    }
}) // this query is matching the last element of the list.
```

Note as it can be painful to write the query himself, a support to write it with a string in the codemirror editor is
coming soon.

For the moment, the codemirror support doesn't handle the regexp.
