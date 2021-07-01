0.14.0 / 2021-03-26
===================

* **[Feature]**: Through the update of [lezer-promql](https://github.com/promlabs/lezer-promql/releases/tag/0.18.0)
  support negative offset
* **[Enhancement]**: Add snippet to ease the usage of the aggregation `topk`, `bottomk` and `count_value`
* **[Enhancement]**: Autocomplete the 2nd hard of subquery time selector

0.13.0 / 2021-03-22
===================
* **[Feature]**: Linter and Autocompletion support 3 new PromQL functions: `clamp` , `last_over_time`, `sgn`
* **[Feature]**: Linter and Autocompletion support the `@` expression.
* **[Enhancement]**: Signature of `CompleteStrategy.promQL` has been updated to support the type `Promise<null>`
* **[BreakingChange]**: Support last version of Codemirror.next (v0.18.0)
* **[BreakingChange]**: Remove the function `enricher`

0.12.0 / 2021-01-12
===================

* **[Enhancement]**: Improve the parsing of `BinExpr` thanks to the changes provided by lezer-promql (v0.15.0)
* **[BreakingChange]**: Support the new version of codemirror v0.17.x

0.11.0 / 2020-12-08
===================

* **[Feature]**: Add the completion of the keyword `bool`. (#89)
* **[Feature]**: Add a function `enricher` that can be used to enrich the completion with a custom one.
* **[Feature]**: Add a LRU caching system. (#71)
* **[Feature]**: You can now configure the maximum number of metrics in Prometheus for which metadata is fetched.
* **[Feature]**: Allow the possibility to inject a custom `CompleteStrategy`. (#83)
* **[Feature]**: Provide the Matchers in the PrometheusClient for the method `labelValues` and `series`. (#84)
* **[Feature]**: Add the method `metricName` in the PrometheusClient that supports a prefix of the metric searched. (#84)
* **[Enhancement]**: Caching mechanism and PrometheusClient are splitted. (#71)
* **[Enhancement]**: Optimize the code of the PrometheusClient when no cache is used.
* **[Enhancement]**: General improvement of the code thanks to Codemirror.next v0.14.0 (for the new tree management) and v0.15.0 (for the new tags/highlight management)
* **[Enhancement]**: Improve the code coverage of the parser concerning the parsing of the function / aggregation.
* **[BugFix]**: In certain case, the linter didn't ignore the comments. (#78)
* **[BreakingChange]**: Use an object instead of a map when querying the metrics metadata.
* **[BreakingChange]**: Support last version of Codemirror.next (v0.15.0).
* **[BreakingChange]**: Change the way the completion configuration is structured.

0.10.2 / 2020-10-18
===================

* **[BugFix]**: Fixed missing autocompletion of binary operators after aggregations

0.10.1 / 2020-10-16
===================

* **[Enhancement]**: Caching of series label names and values for autocompletion is now optimized to be much faster
* **[BugFix]**: Fixed incorrect linter errors around binary operator arguments not separated from the operator by a space

0.10.0 / 2020-10-14
===================

* **[Enhancement]**: The Linter is now checking operation many-to-many, one-to-one, many-to-one and one-to-many
* **[Enhancement]**: The autocompletion is now showing the type of the metric if the type is same for every possible definition of the same metric
* **[Enhancement]**: The autocompletion is supporting the completion of the duration
* **[Enhancement]**: Descriptions have been added for the snippet, the binary operator modifier and the aggregation operator modifier
* **[Enhancement]**: Coverage of the code has been increased (a lot).
* **[BreakingChange]**: Removing LSP support
