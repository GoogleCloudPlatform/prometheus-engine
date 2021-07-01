(function (global, factory) {
  typeof exports === 'object' && typeof module !== 'undefined' ? factory(exports, require('react')) :
  typeof define === 'function' && define.amd ? define(['exports', 'react'], factory) :
  (global = global || self, factory(global.Downshift = {}, global.React));
}(this, (function (exports, react) { 'use strict';

  function _objectWithoutPropertiesLoose(source, excluded) {
    if (source == null) return {};
    var target = {};
    var sourceKeys = Object.keys(source);
    var key, i;

    for (i = 0; i < sourceKeys.length; i++) {
      key = sourceKeys[i];
      if (excluded.indexOf(key) >= 0) continue;
      target[key] = source[key];
    }

    return target;
  }

  function _extends() {
    _extends = Object.assign || function (target) {
      for (var i = 1; i < arguments.length; i++) {
        var source = arguments[i];

        for (var key in source) {
          if (Object.prototype.hasOwnProperty.call(source, key)) {
            target[key] = source[key];
          }
        }
      }

      return target;
    };

    return _extends.apply(this, arguments);
  }

  function _assertThisInitialized(self) {
    if (self === void 0) {
      throw new ReferenceError("this hasn't been initialised - super() hasn't been called");
    }

    return self;
  }

  function _inheritsLoose(subClass, superClass) {
    subClass.prototype = Object.create(superClass.prototype);
    subClass.prototype.constructor = subClass;
    subClass.__proto__ = superClass;
  }

  function unwrapExports (x) {
  	return x && x.__esModule && Object.prototype.hasOwnProperty.call(x, 'default') ? x['default'] : x;
  }

  function createCommonjsModule(fn, module) {
  	return module = { exports: {} }, fn(module, module.exports), module.exports;
  }

  var reactIs_production_min = createCommonjsModule(function (module, exports) {

    Object.defineProperty(exports, "__esModule", {
      value: !0
    });
    var b = "function" === typeof Symbol && Symbol.for,
        c = b ? Symbol.for("react.element") : 60103,
        d = b ? Symbol.for("react.portal") : 60106,
        e = b ? Symbol.for("react.fragment") : 60107,
        f = b ? Symbol.for("react.strict_mode") : 60108,
        g = b ? Symbol.for("react.profiler") : 60114,
        h = b ? Symbol.for("react.provider") : 60109,
        k = b ? Symbol.for("react.context") : 60110,
        l = b ? Symbol.for("react.async_mode") : 60111,
        m = b ? Symbol.for("react.concurrent_mode") : 60111,
        n = b ? Symbol.for("react.forward_ref") : 60112,
        p = b ? Symbol.for("react.suspense") : 60113,
        q = b ? Symbol.for("react.suspense_list") : 60120,
        r = b ? Symbol.for("react.memo") : 60115,
        t = b ? Symbol.for("react.lazy") : 60116,
        v = b ? Symbol.for("react.fundamental") : 60117,
        w = b ? Symbol.for("react.responder") : 60118;

    function x(a) {
      if ("object" === typeof a && null !== a) {
        var u = a.$$typeof;

        switch (u) {
          case c:
            switch (a = a.type, a) {
              case l:
              case m:
              case e:
              case g:
              case f:
              case p:
                return a;

              default:
                switch (a = a && a.$$typeof, a) {
                  case k:
                  case n:
                  case h:
                    return a;

                  default:
                    return u;
                }

            }

          case t:
          case r:
          case d:
            return u;
        }
      }
    }

    function y(a) {
      return x(a) === m;
    }

    exports.typeOf = x;
    exports.AsyncMode = l;
    exports.ConcurrentMode = m;
    exports.ContextConsumer = k;
    exports.ContextProvider = h;
    exports.Element = c;
    exports.ForwardRef = n;
    exports.Fragment = e;
    exports.Lazy = t;
    exports.Memo = r;
    exports.Portal = d;
    exports.Profiler = g;
    exports.StrictMode = f;
    exports.Suspense = p;

    exports.isValidElementType = function (a) {
      return "string" === typeof a || "function" === typeof a || a === e || a === m || a === g || a === f || a === p || a === q || "object" === typeof a && null !== a && (a.$$typeof === t || a.$$typeof === r || a.$$typeof === h || a.$$typeof === k || a.$$typeof === n || a.$$typeof === v || a.$$typeof === w);
    };

    exports.isAsyncMode = function (a) {
      return y(a) || x(a) === l;
    };

    exports.isConcurrentMode = y;

    exports.isContextConsumer = function (a) {
      return x(a) === k;
    };

    exports.isContextProvider = function (a) {
      return x(a) === h;
    };

    exports.isElement = function (a) {
      return "object" === typeof a && null !== a && a.$$typeof === c;
    };

    exports.isForwardRef = function (a) {
      return x(a) === n;
    };

    exports.isFragment = function (a) {
      return x(a) === e;
    };

    exports.isLazy = function (a) {
      return x(a) === t;
    };

    exports.isMemo = function (a) {
      return x(a) === r;
    };

    exports.isPortal = function (a) {
      return x(a) === d;
    };

    exports.isProfiler = function (a) {
      return x(a) === g;
    };

    exports.isStrictMode = function (a) {
      return x(a) === f;
    };

    exports.isSuspense = function (a) {
      return x(a) === p;
    };
  });
  unwrapExports(reactIs_production_min);
  var reactIs_production_min_1 = reactIs_production_min.typeOf;
  var reactIs_production_min_2 = reactIs_production_min.AsyncMode;
  var reactIs_production_min_3 = reactIs_production_min.ConcurrentMode;
  var reactIs_production_min_4 = reactIs_production_min.ContextConsumer;
  var reactIs_production_min_5 = reactIs_production_min.ContextProvider;
  var reactIs_production_min_6 = reactIs_production_min.Element;
  var reactIs_production_min_7 = reactIs_production_min.ForwardRef;
  var reactIs_production_min_8 = reactIs_production_min.Fragment;
  var reactIs_production_min_9 = reactIs_production_min.Lazy;
  var reactIs_production_min_10 = reactIs_production_min.Memo;
  var reactIs_production_min_11 = reactIs_production_min.Portal;
  var reactIs_production_min_12 = reactIs_production_min.Profiler;
  var reactIs_production_min_13 = reactIs_production_min.StrictMode;
  var reactIs_production_min_14 = reactIs_production_min.Suspense;
  var reactIs_production_min_15 = reactIs_production_min.isValidElementType;
  var reactIs_production_min_16 = reactIs_production_min.isAsyncMode;
  var reactIs_production_min_17 = reactIs_production_min.isConcurrentMode;
  var reactIs_production_min_18 = reactIs_production_min.isContextConsumer;
  var reactIs_production_min_19 = reactIs_production_min.isContextProvider;
  var reactIs_production_min_20 = reactIs_production_min.isElement;
  var reactIs_production_min_21 = reactIs_production_min.isForwardRef;
  var reactIs_production_min_22 = reactIs_production_min.isFragment;
  var reactIs_production_min_23 = reactIs_production_min.isLazy;
  var reactIs_production_min_24 = reactIs_production_min.isMemo;
  var reactIs_production_min_25 = reactIs_production_min.isPortal;
  var reactIs_production_min_26 = reactIs_production_min.isProfiler;
  var reactIs_production_min_27 = reactIs_production_min.isStrictMode;
  var reactIs_production_min_28 = reactIs_production_min.isSuspense;

  var reactIs_development = createCommonjsModule(function (module, exports) {

    {
      (function () {

        Object.defineProperty(exports, '__esModule', {
          value: true
        }); // The Symbol used to tag the ReactElement-like types. If there is no native Symbol
        // nor polyfill, then a plain number is used for performance.

        var hasSymbol = typeof Symbol === 'function' && Symbol.for;
        var REACT_ELEMENT_TYPE = hasSymbol ? Symbol.for('react.element') : 0xeac7;
        var REACT_PORTAL_TYPE = hasSymbol ? Symbol.for('react.portal') : 0xeaca;
        var REACT_FRAGMENT_TYPE = hasSymbol ? Symbol.for('react.fragment') : 0xeacb;
        var REACT_STRICT_MODE_TYPE = hasSymbol ? Symbol.for('react.strict_mode') : 0xeacc;
        var REACT_PROFILER_TYPE = hasSymbol ? Symbol.for('react.profiler') : 0xead2;
        var REACT_PROVIDER_TYPE = hasSymbol ? Symbol.for('react.provider') : 0xeacd;
        var REACT_CONTEXT_TYPE = hasSymbol ? Symbol.for('react.context') : 0xeace; // TODO: We don't use AsyncMode or ConcurrentMode anymore. They were temporary
        // (unstable) APIs that have been removed. Can we remove the symbols?

        var REACT_ASYNC_MODE_TYPE = hasSymbol ? Symbol.for('react.async_mode') : 0xeacf;
        var REACT_CONCURRENT_MODE_TYPE = hasSymbol ? Symbol.for('react.concurrent_mode') : 0xeacf;
        var REACT_FORWARD_REF_TYPE = hasSymbol ? Symbol.for('react.forward_ref') : 0xead0;
        var REACT_SUSPENSE_TYPE = hasSymbol ? Symbol.for('react.suspense') : 0xead1;
        var REACT_SUSPENSE_LIST_TYPE = hasSymbol ? Symbol.for('react.suspense_list') : 0xead8;
        var REACT_MEMO_TYPE = hasSymbol ? Symbol.for('react.memo') : 0xead3;
        var REACT_LAZY_TYPE = hasSymbol ? Symbol.for('react.lazy') : 0xead4;
        var REACT_FUNDAMENTAL_TYPE = hasSymbol ? Symbol.for('react.fundamental') : 0xead5;
        var REACT_RESPONDER_TYPE = hasSymbol ? Symbol.for('react.responder') : 0xead6;

        function isValidElementType(type) {
          return typeof type === 'string' || typeof type === 'function' || // Note: its typeof might be other than 'symbol' or 'number' if it's a polyfill.
          type === REACT_FRAGMENT_TYPE || type === REACT_CONCURRENT_MODE_TYPE || type === REACT_PROFILER_TYPE || type === REACT_STRICT_MODE_TYPE || type === REACT_SUSPENSE_TYPE || type === REACT_SUSPENSE_LIST_TYPE || typeof type === 'object' && type !== null && (type.$$typeof === REACT_LAZY_TYPE || type.$$typeof === REACT_MEMO_TYPE || type.$$typeof === REACT_PROVIDER_TYPE || type.$$typeof === REACT_CONTEXT_TYPE || type.$$typeof === REACT_FORWARD_REF_TYPE || type.$$typeof === REACT_FUNDAMENTAL_TYPE || type.$$typeof === REACT_RESPONDER_TYPE);
        }
        /**
         * Forked from fbjs/warning:
         * https://github.com/facebook/fbjs/blob/e66ba20ad5be433eb54423f2b097d829324d9de6/packages/fbjs/src/__forks__/warning.js
         *
         * Only change is we use console.warn instead of console.error,
         * and do nothing when 'console' is not supported.
         * This really simplifies the code.
         * ---
         * Similar to invariant but only logs a warning if the condition is not met.
         * This can be used to log issues in development environments in critical
         * paths. Removing the logging code for production environments will keep the
         * same logic and follow the same code paths.
         */


        var lowPriorityWarning = function () {};

        {
          var printWarning = function (format) {
            for (var _len = arguments.length, args = Array(_len > 1 ? _len - 1 : 0), _key = 1; _key < _len; _key++) {
              args[_key - 1] = arguments[_key];
            }

            var argIndex = 0;
            var message = 'Warning: ' + format.replace(/%s/g, function () {
              return args[argIndex++];
            });

            if (typeof console !== 'undefined') {
              console.warn(message);
            }

            try {
              // --- Welcome to debugging React ---
              // This error was thrown as a convenience so that you can use this stack
              // to find the callsite that caused this warning to fire.
              throw new Error(message);
            } catch (x) {}
          };

          lowPriorityWarning = function (condition, format) {
            if (format === undefined) {
              throw new Error('`lowPriorityWarning(condition, format, ...args)` requires a warning ' + 'message argument');
            }

            if (!condition) {
              for (var _len2 = arguments.length, args = Array(_len2 > 2 ? _len2 - 2 : 0), _key2 = 2; _key2 < _len2; _key2++) {
                args[_key2 - 2] = arguments[_key2];
              }

              printWarning.apply(undefined, [format].concat(args));
            }
          };
        }
        var lowPriorityWarning$1 = lowPriorityWarning;

        function typeOf(object) {
          if (typeof object === 'object' && object !== null) {
            var $$typeof = object.$$typeof;

            switch ($$typeof) {
              case REACT_ELEMENT_TYPE:
                var type = object.type;

                switch (type) {
                  case REACT_ASYNC_MODE_TYPE:
                  case REACT_CONCURRENT_MODE_TYPE:
                  case REACT_FRAGMENT_TYPE:
                  case REACT_PROFILER_TYPE:
                  case REACT_STRICT_MODE_TYPE:
                  case REACT_SUSPENSE_TYPE:
                    return type;

                  default:
                    var $$typeofType = type && type.$$typeof;

                    switch ($$typeofType) {
                      case REACT_CONTEXT_TYPE:
                      case REACT_FORWARD_REF_TYPE:
                      case REACT_PROVIDER_TYPE:
                        return $$typeofType;

                      default:
                        return $$typeof;
                    }

                }

              case REACT_LAZY_TYPE:
              case REACT_MEMO_TYPE:
              case REACT_PORTAL_TYPE:
                return $$typeof;
            }
          }

          return undefined;
        } // AsyncMode is deprecated along with isAsyncMode


        var AsyncMode = REACT_ASYNC_MODE_TYPE;
        var ConcurrentMode = REACT_CONCURRENT_MODE_TYPE;
        var ContextConsumer = REACT_CONTEXT_TYPE;
        var ContextProvider = REACT_PROVIDER_TYPE;
        var Element = REACT_ELEMENT_TYPE;
        var ForwardRef = REACT_FORWARD_REF_TYPE;
        var Fragment = REACT_FRAGMENT_TYPE;
        var Lazy = REACT_LAZY_TYPE;
        var Memo = REACT_MEMO_TYPE;
        var Portal = REACT_PORTAL_TYPE;
        var Profiler = REACT_PROFILER_TYPE;
        var StrictMode = REACT_STRICT_MODE_TYPE;
        var Suspense = REACT_SUSPENSE_TYPE;
        var hasWarnedAboutDeprecatedIsAsyncMode = false; // AsyncMode should be deprecated

        function isAsyncMode(object) {
          {
            if (!hasWarnedAboutDeprecatedIsAsyncMode) {
              hasWarnedAboutDeprecatedIsAsyncMode = true;
              lowPriorityWarning$1(false, 'The ReactIs.isAsyncMode() alias has been deprecated, ' + 'and will be removed in React 17+. Update your code to use ' + 'ReactIs.isConcurrentMode() instead. It has the exact same API.');
            }
          }
          return isConcurrentMode(object) || typeOf(object) === REACT_ASYNC_MODE_TYPE;
        }

        function isConcurrentMode(object) {
          return typeOf(object) === REACT_CONCURRENT_MODE_TYPE;
        }

        function isContextConsumer(object) {
          return typeOf(object) === REACT_CONTEXT_TYPE;
        }

        function isContextProvider(object) {
          return typeOf(object) === REACT_PROVIDER_TYPE;
        }

        function isElement(object) {
          return typeof object === 'object' && object !== null && object.$$typeof === REACT_ELEMENT_TYPE;
        }

        function isForwardRef(object) {
          return typeOf(object) === REACT_FORWARD_REF_TYPE;
        }

        function isFragment(object) {
          return typeOf(object) === REACT_FRAGMENT_TYPE;
        }

        function isLazy(object) {
          return typeOf(object) === REACT_LAZY_TYPE;
        }

        function isMemo(object) {
          return typeOf(object) === REACT_MEMO_TYPE;
        }

        function isPortal(object) {
          return typeOf(object) === REACT_PORTAL_TYPE;
        }

        function isProfiler(object) {
          return typeOf(object) === REACT_PROFILER_TYPE;
        }

        function isStrictMode(object) {
          return typeOf(object) === REACT_STRICT_MODE_TYPE;
        }

        function isSuspense(object) {
          return typeOf(object) === REACT_SUSPENSE_TYPE;
        }

        exports.typeOf = typeOf;
        exports.AsyncMode = AsyncMode;
        exports.ConcurrentMode = ConcurrentMode;
        exports.ContextConsumer = ContextConsumer;
        exports.ContextProvider = ContextProvider;
        exports.Element = Element;
        exports.ForwardRef = ForwardRef;
        exports.Fragment = Fragment;
        exports.Lazy = Lazy;
        exports.Memo = Memo;
        exports.Portal = Portal;
        exports.Profiler = Profiler;
        exports.StrictMode = StrictMode;
        exports.Suspense = Suspense;
        exports.isValidElementType = isValidElementType;
        exports.isAsyncMode = isAsyncMode;
        exports.isConcurrentMode = isConcurrentMode;
        exports.isContextConsumer = isContextConsumer;
        exports.isContextProvider = isContextProvider;
        exports.isElement = isElement;
        exports.isForwardRef = isForwardRef;
        exports.isFragment = isFragment;
        exports.isLazy = isLazy;
        exports.isMemo = isMemo;
        exports.isPortal = isPortal;
        exports.isProfiler = isProfiler;
        exports.isStrictMode = isStrictMode;
        exports.isSuspense = isSuspense;
      })();
    }
  });
  unwrapExports(reactIs_development);
  var reactIs_development_1 = reactIs_development.typeOf;
  var reactIs_development_2 = reactIs_development.AsyncMode;
  var reactIs_development_3 = reactIs_development.ConcurrentMode;
  var reactIs_development_4 = reactIs_development.ContextConsumer;
  var reactIs_development_5 = reactIs_development.ContextProvider;
  var reactIs_development_6 = reactIs_development.Element;
  var reactIs_development_7 = reactIs_development.ForwardRef;
  var reactIs_development_8 = reactIs_development.Fragment;
  var reactIs_development_9 = reactIs_development.Lazy;
  var reactIs_development_10 = reactIs_development.Memo;
  var reactIs_development_11 = reactIs_development.Portal;
  var reactIs_development_12 = reactIs_development.Profiler;
  var reactIs_development_13 = reactIs_development.StrictMode;
  var reactIs_development_14 = reactIs_development.Suspense;
  var reactIs_development_15 = reactIs_development.isValidElementType;
  var reactIs_development_16 = reactIs_development.isAsyncMode;
  var reactIs_development_17 = reactIs_development.isConcurrentMode;
  var reactIs_development_18 = reactIs_development.isContextConsumer;
  var reactIs_development_19 = reactIs_development.isContextProvider;
  var reactIs_development_20 = reactIs_development.isElement;
  var reactIs_development_21 = reactIs_development.isForwardRef;
  var reactIs_development_22 = reactIs_development.isFragment;
  var reactIs_development_23 = reactIs_development.isLazy;
  var reactIs_development_24 = reactIs_development.isMemo;
  var reactIs_development_25 = reactIs_development.isPortal;
  var reactIs_development_26 = reactIs_development.isProfiler;
  var reactIs_development_27 = reactIs_development.isStrictMode;
  var reactIs_development_28 = reactIs_development.isSuspense;

  var reactIs = createCommonjsModule(function (module) {

    {
      module.exports = reactIs_development;
    }
  });
  var reactIs_1 = reactIs.isForwardRef;

  /*
  object-assign
  (c) Sindre Sorhus
  @license MIT
  */
  /* eslint-disable no-unused-vars */

  var getOwnPropertySymbols = Object.getOwnPropertySymbols;
  var hasOwnProperty = Object.prototype.hasOwnProperty;
  var propIsEnumerable = Object.prototype.propertyIsEnumerable;

  function toObject(val) {
    if (val === null || val === undefined) {
      throw new TypeError('Object.assign cannot be called with null or undefined');
    }

    return Object(val);
  }

  function shouldUseNative() {
    try {
      if (!Object.assign) {
        return false;
      } // Detect buggy property enumeration order in older V8 versions.
      // https://bugs.chromium.org/p/v8/issues/detail?id=4118


      var test1 = new String('abc'); // eslint-disable-line no-new-wrappers

      test1[5] = 'de';

      if (Object.getOwnPropertyNames(test1)[0] === '5') {
        return false;
      } // https://bugs.chromium.org/p/v8/issues/detail?id=3056


      var test2 = {};

      for (var i = 0; i < 10; i++) {
        test2['_' + String.fromCharCode(i)] = i;
      }

      var order2 = Object.getOwnPropertyNames(test2).map(function (n) {
        return test2[n];
      });

      if (order2.join('') !== '0123456789') {
        return false;
      } // https://bugs.chromium.org/p/v8/issues/detail?id=3056


      var test3 = {};
      'abcdefghijklmnopqrst'.split('').forEach(function (letter) {
        test3[letter] = letter;
      });

      if (Object.keys(Object.assign({}, test3)).join('') !== 'abcdefghijklmnopqrst') {
        return false;
      }

      return true;
    } catch (err) {
      // We don't expect any of the above to throw, but better to be safe.
      return false;
    }
  }

  var objectAssign = shouldUseNative() ? Object.assign : function (target, source) {
    var from;
    var to = toObject(target);
    var symbols;

    for (var s = 1; s < arguments.length; s++) {
      from = Object(arguments[s]);

      for (var key in from) {
        if (hasOwnProperty.call(from, key)) {
          to[key] = from[key];
        }
      }

      if (getOwnPropertySymbols) {
        symbols = getOwnPropertySymbols(from);

        for (var i = 0; i < symbols.length; i++) {
          if (propIsEnumerable.call(from, symbols[i])) {
            to[symbols[i]] = from[symbols[i]];
          }
        }
      }
    }

    return to;
  };

  /**
   * Copyright (c) 2013-present, Facebook, Inc.
   *
   * This source code is licensed under the MIT license found in the
   * LICENSE file in the root directory of this source tree.
   */

  var ReactPropTypesSecret = 'SECRET_DO_NOT_PASS_THIS_OR_YOU_WILL_BE_FIRED';
  var ReactPropTypesSecret_1 = ReactPropTypesSecret;

  var printWarning = function () {};

  {
    var ReactPropTypesSecret$1 = ReactPropTypesSecret_1;
    var loggedTypeFailures = {};
    var has = Function.call.bind(Object.prototype.hasOwnProperty);

    printWarning = function (text) {
      var message = 'Warning: ' + text;

      if (typeof console !== 'undefined') {
        console.error(message);
      }

      try {
        // --- Welcome to debugging React ---
        // This error was thrown as a convenience so that you can use this stack
        // to find the callsite that caused this warning to fire.
        throw new Error(message);
      } catch (x) {}
    };
  }
  /**
   * Assert that the values match with the type specs.
   * Error messages are memorized and will only be shown once.
   *
   * @param {object} typeSpecs Map of name to a ReactPropType
   * @param {object} values Runtime values that need to be type-checked
   * @param {string} location e.g. "prop", "context", "child context"
   * @param {string} componentName Name of the component for error messages.
   * @param {?Function} getStack Returns the component stack.
   * @private
   */


  function checkPropTypes(typeSpecs, values, location, componentName, getStack) {
    {
      for (var typeSpecName in typeSpecs) {
        if (has(typeSpecs, typeSpecName)) {
          var error; // Prop type validation may throw. In case they do, we don't want to
          // fail the render phase where it didn't fail before. So we log it.
          // After these have been cleaned up, we'll let them throw.

          try {
            // This is intentionally an invariant that gets caught. It's the same
            // behavior as without this statement except with a better message.
            if (typeof typeSpecs[typeSpecName] !== 'function') {
              var err = Error((componentName || 'React class') + ': ' + location + ' type `' + typeSpecName + '` is invalid; ' + 'it must be a function, usually from the `prop-types` package, but received `' + typeof typeSpecs[typeSpecName] + '`.');
              err.name = 'Invariant Violation';
              throw err;
            }

            error = typeSpecs[typeSpecName](values, typeSpecName, componentName, location, null, ReactPropTypesSecret$1);
          } catch (ex) {
            error = ex;
          }

          if (error && !(error instanceof Error)) {
            printWarning((componentName || 'React class') + ': type specification of ' + location + ' `' + typeSpecName + '` is invalid; the type checker ' + 'function must return `null` or an `Error` but returned a ' + typeof error + '. ' + 'You may have forgotten to pass an argument to the type checker ' + 'creator (arrayOf, instanceOf, objectOf, oneOf, oneOfType, and ' + 'shape all require an argument).');
          }

          if (error instanceof Error && !(error.message in loggedTypeFailures)) {
            // Only monitor this failure once because there tends to be a lot of the
            // same error.
            loggedTypeFailures[error.message] = true;
            var stack = getStack ? getStack() : '';
            printWarning('Failed ' + location + ' type: ' + error.message + (stack != null ? stack : ''));
          }
        }
      }
    }
  }
  /**
   * Resets warning cache when testing.
   *
   * @private
   */


  checkPropTypes.resetWarningCache = function () {
    {
      loggedTypeFailures = {};
    }
  };

  var checkPropTypes_1 = checkPropTypes;

  var has$1 = Function.call.bind(Object.prototype.hasOwnProperty);

  var printWarning$1 = function () {};

  {
    printWarning$1 = function (text) {
      var message = 'Warning: ' + text;

      if (typeof console !== 'undefined') {
        console.error(message);
      }

      try {
        // --- Welcome to debugging React ---
        // This error was thrown as a convenience so that you can use this stack
        // to find the callsite that caused this warning to fire.
        throw new Error(message);
      } catch (x) {}
    };
  }

  function emptyFunctionThatReturnsNull() {
    return null;
  }

  var factoryWithTypeCheckers = function (isValidElement, throwOnDirectAccess) {
    /* global Symbol */
    var ITERATOR_SYMBOL = typeof Symbol === 'function' && Symbol.iterator;
    var FAUX_ITERATOR_SYMBOL = '@@iterator'; // Before Symbol spec.

    /**
     * Returns the iterator method function contained on the iterable object.
     *
     * Be sure to invoke the function with the iterable as context:
     *
     *     var iteratorFn = getIteratorFn(myIterable);
     *     if (iteratorFn) {
     *       var iterator = iteratorFn.call(myIterable);
     *       ...
     *     }
     *
     * @param {?object} maybeIterable
     * @return {?function}
     */

    function getIteratorFn(maybeIterable) {
      var iteratorFn = maybeIterable && (ITERATOR_SYMBOL && maybeIterable[ITERATOR_SYMBOL] || maybeIterable[FAUX_ITERATOR_SYMBOL]);

      if (typeof iteratorFn === 'function') {
        return iteratorFn;
      }
    }
    /**
     * Collection of methods that allow declaration and validation of props that are
     * supplied to React components. Example usage:
     *
     *   var Props = require('ReactPropTypes');
     *   var MyArticle = React.createClass({
     *     propTypes: {
     *       // An optional string prop named "description".
     *       description: Props.string,
     *
     *       // A required enum prop named "category".
     *       category: Props.oneOf(['News','Photos']).isRequired,
     *
     *       // A prop named "dialog" that requires an instance of Dialog.
     *       dialog: Props.instanceOf(Dialog).isRequired
     *     },
     *     render: function() { ... }
     *   });
     *
     * A more formal specification of how these methods are used:
     *
     *   type := array|bool|func|object|number|string|oneOf([...])|instanceOf(...)
     *   decl := ReactPropTypes.{type}(.isRequired)?
     *
     * Each and every declaration produces a function with the same signature. This
     * allows the creation of custom validation functions. For example:
     *
     *  var MyLink = React.createClass({
     *    propTypes: {
     *      // An optional string or URI prop named "href".
     *      href: function(props, propName, componentName) {
     *        var propValue = props[propName];
     *        if (propValue != null && typeof propValue !== 'string' &&
     *            !(propValue instanceof URI)) {
     *          return new Error(
     *            'Expected a string or an URI for ' + propName + ' in ' +
     *            componentName
     *          );
     *        }
     *      }
     *    },
     *    render: function() {...}
     *  });
     *
     * @internal
     */


    var ANONYMOUS = '<<anonymous>>'; // Important!
    // Keep this list in sync with production version in `./factoryWithThrowingShims.js`.

    var ReactPropTypes = {
      array: createPrimitiveTypeChecker('array'),
      bool: createPrimitiveTypeChecker('boolean'),
      func: createPrimitiveTypeChecker('function'),
      number: createPrimitiveTypeChecker('number'),
      object: createPrimitiveTypeChecker('object'),
      string: createPrimitiveTypeChecker('string'),
      symbol: createPrimitiveTypeChecker('symbol'),
      any: createAnyTypeChecker(),
      arrayOf: createArrayOfTypeChecker,
      element: createElementTypeChecker(),
      elementType: createElementTypeTypeChecker(),
      instanceOf: createInstanceTypeChecker,
      node: createNodeChecker(),
      objectOf: createObjectOfTypeChecker,
      oneOf: createEnumTypeChecker,
      oneOfType: createUnionTypeChecker,
      shape: createShapeTypeChecker,
      exact: createStrictShapeTypeChecker
    };
    /**
     * inlined Object.is polyfill to avoid requiring consumers ship their own
     * https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Object/is
     */

    /*eslint-disable no-self-compare*/

    function is(x, y) {
      // SameValue algorithm
      if (x === y) {
        // Steps 1-5, 7-10
        // Steps 6.b-6.e: +0 != -0
        return x !== 0 || 1 / x === 1 / y;
      } else {
        // Step 6.a: NaN == NaN
        return x !== x && y !== y;
      }
    }
    /*eslint-enable no-self-compare*/

    /**
     * We use an Error-like object for backward compatibility as people may call
     * PropTypes directly and inspect their output. However, we don't use real
     * Errors anymore. We don't inspect their stack anyway, and creating them
     * is prohibitively expensive if they are created too often, such as what
     * happens in oneOfType() for any type before the one that matched.
     */


    function PropTypeError(message) {
      this.message = message;
      this.stack = '';
    } // Make `instanceof Error` still work for returned errors.


    PropTypeError.prototype = Error.prototype;

    function createChainableTypeChecker(validate) {
      {
        var manualPropTypeCallCache = {};
        var manualPropTypeWarningCount = 0;
      }

      function checkType(isRequired, props, propName, componentName, location, propFullName, secret) {
        componentName = componentName || ANONYMOUS;
        propFullName = propFullName || propName;

        if (secret !== ReactPropTypesSecret_1) {
          if (throwOnDirectAccess) {
            // New behavior only for users of `prop-types` package
            var err = new Error('Calling PropTypes validators directly is not supported by the `prop-types` package. ' + 'Use `PropTypes.checkPropTypes()` to call them. ' + 'Read more at http://fb.me/use-check-prop-types');
            err.name = 'Invariant Violation';
            throw err;
          } else if ( typeof console !== 'undefined') {
            // Old behavior for people using React.PropTypes
            var cacheKey = componentName + ':' + propName;

            if (!manualPropTypeCallCache[cacheKey] && // Avoid spamming the console because they are often not actionable except for lib authors
            manualPropTypeWarningCount < 3) {
              printWarning$1('You are manually calling a React.PropTypes validation ' + 'function for the `' + propFullName + '` prop on `' + componentName + '`. This is deprecated ' + 'and will throw in the standalone `prop-types` package. ' + 'You may be seeing this warning due to a third-party PropTypes ' + 'library. See https://fb.me/react-warning-dont-call-proptypes ' + 'for details.');
              manualPropTypeCallCache[cacheKey] = true;
              manualPropTypeWarningCount++;
            }
          }
        }

        if (props[propName] == null) {
          if (isRequired) {
            if (props[propName] === null) {
              return new PropTypeError('The ' + location + ' `' + propFullName + '` is marked as required ' + ('in `' + componentName + '`, but its value is `null`.'));
            }

            return new PropTypeError('The ' + location + ' `' + propFullName + '` is marked as required in ' + ('`' + componentName + '`, but its value is `undefined`.'));
          }

          return null;
        } else {
          return validate(props, propName, componentName, location, propFullName);
        }
      }

      var chainedCheckType = checkType.bind(null, false);
      chainedCheckType.isRequired = checkType.bind(null, true);
      return chainedCheckType;
    }

    function createPrimitiveTypeChecker(expectedType) {
      function validate(props, propName, componentName, location, propFullName, secret) {
        var propValue = props[propName];
        var propType = getPropType(propValue);

        if (propType !== expectedType) {
          // `propValue` being instance of, say, date/regexp, pass the 'object'
          // check, but we can offer a more precise error message here rather than
          // 'of type `object`'.
          var preciseType = getPreciseType(propValue);
          return new PropTypeError('Invalid ' + location + ' `' + propFullName + '` of type ' + ('`' + preciseType + '` supplied to `' + componentName + '`, expected ') + ('`' + expectedType + '`.'));
        }

        return null;
      }

      return createChainableTypeChecker(validate);
    }

    function createAnyTypeChecker() {
      return createChainableTypeChecker(emptyFunctionThatReturnsNull);
    }

    function createArrayOfTypeChecker(typeChecker) {
      function validate(props, propName, componentName, location, propFullName) {
        if (typeof typeChecker !== 'function') {
          return new PropTypeError('Property `' + propFullName + '` of component `' + componentName + '` has invalid PropType notation inside arrayOf.');
        }

        var propValue = props[propName];

        if (!Array.isArray(propValue)) {
          var propType = getPropType(propValue);
          return new PropTypeError('Invalid ' + location + ' `' + propFullName + '` of type ' + ('`' + propType + '` supplied to `' + componentName + '`, expected an array.'));
        }

        for (var i = 0; i < propValue.length; i++) {
          var error = typeChecker(propValue, i, componentName, location, propFullName + '[' + i + ']', ReactPropTypesSecret_1);

          if (error instanceof Error) {
            return error;
          }
        }

        return null;
      }

      return createChainableTypeChecker(validate);
    }

    function createElementTypeChecker() {
      function validate(props, propName, componentName, location, propFullName) {
        var propValue = props[propName];

        if (!isValidElement(propValue)) {
          var propType = getPropType(propValue);
          return new PropTypeError('Invalid ' + location + ' `' + propFullName + '` of type ' + ('`' + propType + '` supplied to `' + componentName + '`, expected a single ReactElement.'));
        }

        return null;
      }

      return createChainableTypeChecker(validate);
    }

    function createElementTypeTypeChecker() {
      function validate(props, propName, componentName, location, propFullName) {
        var propValue = props[propName];

        if (!reactIs.isValidElementType(propValue)) {
          var propType = getPropType(propValue);
          return new PropTypeError('Invalid ' + location + ' `' + propFullName + '` of type ' + ('`' + propType + '` supplied to `' + componentName + '`, expected a single ReactElement type.'));
        }

        return null;
      }

      return createChainableTypeChecker(validate);
    }

    function createInstanceTypeChecker(expectedClass) {
      function validate(props, propName, componentName, location, propFullName) {
        if (!(props[propName] instanceof expectedClass)) {
          var expectedClassName = expectedClass.name || ANONYMOUS;
          var actualClassName = getClassName(props[propName]);
          return new PropTypeError('Invalid ' + location + ' `' + propFullName + '` of type ' + ('`' + actualClassName + '` supplied to `' + componentName + '`, expected ') + ('instance of `' + expectedClassName + '`.'));
        }

        return null;
      }

      return createChainableTypeChecker(validate);
    }

    function createEnumTypeChecker(expectedValues) {
      if (!Array.isArray(expectedValues)) {
        {
          if (arguments.length > 1) {
            printWarning$1('Invalid arguments supplied to oneOf, expected an array, got ' + arguments.length + ' arguments. ' + 'A common mistake is to write oneOf(x, y, z) instead of oneOf([x, y, z]).');
          } else {
            printWarning$1('Invalid argument supplied to oneOf, expected an array.');
          }
        }

        return emptyFunctionThatReturnsNull;
      }

      function validate(props, propName, componentName, location, propFullName) {
        var propValue = props[propName];

        for (var i = 0; i < expectedValues.length; i++) {
          if (is(propValue, expectedValues[i])) {
            return null;
          }
        }

        var valuesString = JSON.stringify(expectedValues, function replacer(key, value) {
          var type = getPreciseType(value);

          if (type === 'symbol') {
            return String(value);
          }

          return value;
        });
        return new PropTypeError('Invalid ' + location + ' `' + propFullName + '` of value `' + String(propValue) + '` ' + ('supplied to `' + componentName + '`, expected one of ' + valuesString + '.'));
      }

      return createChainableTypeChecker(validate);
    }

    function createObjectOfTypeChecker(typeChecker) {
      function validate(props, propName, componentName, location, propFullName) {
        if (typeof typeChecker !== 'function') {
          return new PropTypeError('Property `' + propFullName + '` of component `' + componentName + '` has invalid PropType notation inside objectOf.');
        }

        var propValue = props[propName];
        var propType = getPropType(propValue);

        if (propType !== 'object') {
          return new PropTypeError('Invalid ' + location + ' `' + propFullName + '` of type ' + ('`' + propType + '` supplied to `' + componentName + '`, expected an object.'));
        }

        for (var key in propValue) {
          if (has$1(propValue, key)) {
            var error = typeChecker(propValue, key, componentName, location, propFullName + '.' + key, ReactPropTypesSecret_1);

            if (error instanceof Error) {
              return error;
            }
          }
        }

        return null;
      }

      return createChainableTypeChecker(validate);
    }

    function createUnionTypeChecker(arrayOfTypeCheckers) {
      if (!Array.isArray(arrayOfTypeCheckers)) {
         printWarning$1('Invalid argument supplied to oneOfType, expected an instance of array.') ;
        return emptyFunctionThatReturnsNull;
      }

      for (var i = 0; i < arrayOfTypeCheckers.length; i++) {
        var checker = arrayOfTypeCheckers[i];

        if (typeof checker !== 'function') {
          printWarning$1('Invalid argument supplied to oneOfType. Expected an array of check functions, but ' + 'received ' + getPostfixForTypeWarning(checker) + ' at index ' + i + '.');
          return emptyFunctionThatReturnsNull;
        }
      }

      function validate(props, propName, componentName, location, propFullName) {
        for (var i = 0; i < arrayOfTypeCheckers.length; i++) {
          var checker = arrayOfTypeCheckers[i];

          if (checker(props, propName, componentName, location, propFullName, ReactPropTypesSecret_1) == null) {
            return null;
          }
        }

        return new PropTypeError('Invalid ' + location + ' `' + propFullName + '` supplied to ' + ('`' + componentName + '`.'));
      }

      return createChainableTypeChecker(validate);
    }

    function createNodeChecker() {
      function validate(props, propName, componentName, location, propFullName) {
        if (!isNode(props[propName])) {
          return new PropTypeError('Invalid ' + location + ' `' + propFullName + '` supplied to ' + ('`' + componentName + '`, expected a ReactNode.'));
        }

        return null;
      }

      return createChainableTypeChecker(validate);
    }

    function createShapeTypeChecker(shapeTypes) {
      function validate(props, propName, componentName, location, propFullName) {
        var propValue = props[propName];
        var propType = getPropType(propValue);

        if (propType !== 'object') {
          return new PropTypeError('Invalid ' + location + ' `' + propFullName + '` of type `' + propType + '` ' + ('supplied to `' + componentName + '`, expected `object`.'));
        }

        for (var key in shapeTypes) {
          var checker = shapeTypes[key];

          if (!checker) {
            continue;
          }

          var error = checker(propValue, key, componentName, location, propFullName + '.' + key, ReactPropTypesSecret_1);

          if (error) {
            return error;
          }
        }

        return null;
      }

      return createChainableTypeChecker(validate);
    }

    function createStrictShapeTypeChecker(shapeTypes) {
      function validate(props, propName, componentName, location, propFullName) {
        var propValue = props[propName];
        var propType = getPropType(propValue);

        if (propType !== 'object') {
          return new PropTypeError('Invalid ' + location + ' `' + propFullName + '` of type `' + propType + '` ' + ('supplied to `' + componentName + '`, expected `object`.'));
        } // We need to check all keys in case some are required but missing from
        // props.


        var allKeys = objectAssign({}, props[propName], shapeTypes);

        for (var key in allKeys) {
          var checker = shapeTypes[key];

          if (!checker) {
            return new PropTypeError('Invalid ' + location + ' `' + propFullName + '` key `' + key + '` supplied to `' + componentName + '`.' + '\nBad object: ' + JSON.stringify(props[propName], null, '  ') + '\nValid keys: ' + JSON.stringify(Object.keys(shapeTypes), null, '  '));
          }

          var error = checker(propValue, key, componentName, location, propFullName + '.' + key, ReactPropTypesSecret_1);

          if (error) {
            return error;
          }
        }

        return null;
      }

      return createChainableTypeChecker(validate);
    }

    function isNode(propValue) {
      switch (typeof propValue) {
        case 'number':
        case 'string':
        case 'undefined':
          return true;

        case 'boolean':
          return !propValue;

        case 'object':
          if (Array.isArray(propValue)) {
            return propValue.every(isNode);
          }

          if (propValue === null || isValidElement(propValue)) {
            return true;
          }

          var iteratorFn = getIteratorFn(propValue);

          if (iteratorFn) {
            var iterator = iteratorFn.call(propValue);
            var step;

            if (iteratorFn !== propValue.entries) {
              while (!(step = iterator.next()).done) {
                if (!isNode(step.value)) {
                  return false;
                }
              }
            } else {
              // Iterator will provide entry [k,v] tuples rather than values.
              while (!(step = iterator.next()).done) {
                var entry = step.value;

                if (entry) {
                  if (!isNode(entry[1])) {
                    return false;
                  }
                }
              }
            }
          } else {
            return false;
          }

          return true;

        default:
          return false;
      }
    }

    function isSymbol(propType, propValue) {
      // Native Symbol.
      if (propType === 'symbol') {
        return true;
      } // falsy value can't be a Symbol


      if (!propValue) {
        return false;
      } // 19.4.3.5 Symbol.prototype[@@toStringTag] === 'Symbol'


      if (propValue['@@toStringTag'] === 'Symbol') {
        return true;
      } // Fallback for non-spec compliant Symbols which are polyfilled.


      if (typeof Symbol === 'function' && propValue instanceof Symbol) {
        return true;
      }

      return false;
    } // Equivalent of `typeof` but with special handling for array and regexp.


    function getPropType(propValue) {
      var propType = typeof propValue;

      if (Array.isArray(propValue)) {
        return 'array';
      }

      if (propValue instanceof RegExp) {
        // Old webkits (at least until Android 4.0) return 'function' rather than
        // 'object' for typeof a RegExp. We'll normalize this here so that /bla/
        // passes PropTypes.object.
        return 'object';
      }

      if (isSymbol(propType, propValue)) {
        return 'symbol';
      }

      return propType;
    } // This handles more types than `getPropType`. Only used for error messages.
    // See `createPrimitiveTypeChecker`.


    function getPreciseType(propValue) {
      if (typeof propValue === 'undefined' || propValue === null) {
        return '' + propValue;
      }

      var propType = getPropType(propValue);

      if (propType === 'object') {
        if (propValue instanceof Date) {
          return 'date';
        } else if (propValue instanceof RegExp) {
          return 'regexp';
        }
      }

      return propType;
    } // Returns a string that is postfixed to a warning about an invalid type.
    // For example, "undefined" or "of type array"


    function getPostfixForTypeWarning(value) {
      var type = getPreciseType(value);

      switch (type) {
        case 'array':
        case 'object':
          return 'an ' + type;

        case 'boolean':
        case 'date':
        case 'regexp':
          return 'a ' + type;

        default:
          return type;
      }
    } // Returns class name of the object, if any.


    function getClassName(propValue) {
      if (!propValue.constructor || !propValue.constructor.name) {
        return ANONYMOUS;
      }

      return propValue.constructor.name;
    }

    ReactPropTypes.checkPropTypes = checkPropTypes_1;
    ReactPropTypes.resetWarningCache = checkPropTypes_1.resetWarningCache;
    ReactPropTypes.PropTypes = ReactPropTypes;
    return ReactPropTypes;
  };

  var propTypes = createCommonjsModule(function (module) {
    /**
     * Copyright (c) 2013-present, Facebook, Inc.
     *
     * This source code is licensed under the MIT license found in the
     * LICENSE file in the root directory of this source tree.
     */
    {
      var ReactIs = reactIs; // By explicitly using `prop-types` you are opting into new development behavior.
      // http://fb.me/prop-types-in-prod

      var throwOnDirectAccess = true;
      module.exports = factoryWithTypeCheckers(ReactIs.isElement, throwOnDirectAccess);
    }
  });

  function isElement(el) {
    return el != null && typeof el === 'object' && el.nodeType === 1;
  }

  function canOverflow(overflow, skipOverflowHiddenElements) {
    if (skipOverflowHiddenElements && overflow === 'hidden') {
      return false;
    }

    return overflow !== 'visible' && overflow !== 'clip';
  }

  function isScrollable(el, skipOverflowHiddenElements) {
    if (el.clientHeight < el.scrollHeight || el.clientWidth < el.scrollWidth) {
      var style = getComputedStyle(el, null);
      return canOverflow(style.overflowY, skipOverflowHiddenElements) || canOverflow(style.overflowX, skipOverflowHiddenElements);
    }

    return false;
  }

  function alignNearest(scrollingEdgeStart, scrollingEdgeEnd, scrollingSize, scrollingBorderStart, scrollingBorderEnd, elementEdgeStart, elementEdgeEnd, elementSize) {
    if (elementEdgeStart < scrollingEdgeStart && elementEdgeEnd > scrollingEdgeEnd || elementEdgeStart > scrollingEdgeStart && elementEdgeEnd < scrollingEdgeEnd) {
      return 0;
    }

    if (elementEdgeStart <= scrollingEdgeStart && elementSize <= scrollingSize || elementEdgeEnd >= scrollingEdgeEnd && elementSize >= scrollingSize) {
      return elementEdgeStart - scrollingEdgeStart - scrollingBorderStart;
    }

    if (elementEdgeEnd > scrollingEdgeEnd && elementSize < scrollingSize || elementEdgeStart < scrollingEdgeStart && elementSize > scrollingSize) {
      return elementEdgeEnd - scrollingEdgeEnd + scrollingBorderEnd;
    }

    return 0;
  }

  var computeScrollIntoView = (function (target, options) {
    var scrollMode = options.scrollMode,
        block = options.block,
        inline = options.inline,
        boundary = options.boundary,
        skipOverflowHiddenElements = options.skipOverflowHiddenElements;
    var checkBoundary = typeof boundary === 'function' ? boundary : function (node) {
      return node !== boundary;
    };

    if (!isElement(target)) {
      throw new TypeError('Invalid target');
    }

    var scrollingElement = document.scrollingElement || document.documentElement;
    var frames = [];
    var cursor = target;

    while (isElement(cursor) && checkBoundary(cursor)) {
      cursor = cursor.parentNode;

      if (cursor === scrollingElement) {
        frames.push(cursor);
        break;
      }

      if (cursor === document.body && isScrollable(cursor) && !isScrollable(document.documentElement)) {
        continue;
      }

      if (isScrollable(cursor, skipOverflowHiddenElements)) {
        frames.push(cursor);
      }
    }

    var viewportWidth = window.visualViewport ? visualViewport.width : innerWidth;
    var viewportHeight = window.visualViewport ? visualViewport.height : innerHeight;
    var viewportX = window.scrollX || pageXOffset;
    var viewportY = window.scrollY || pageYOffset;

    var _target$getBoundingCl = target.getBoundingClientRect(),
        targetHeight = _target$getBoundingCl.height,
        targetWidth = _target$getBoundingCl.width,
        targetTop = _target$getBoundingCl.top,
        targetRight = _target$getBoundingCl.right,
        targetBottom = _target$getBoundingCl.bottom,
        targetLeft = _target$getBoundingCl.left;

    var targetBlock = block === 'start' || block === 'nearest' ? targetTop : block === 'end' ? targetBottom : targetTop + targetHeight / 2;
    var targetInline = inline === 'center' ? targetLeft + targetWidth / 2 : inline === 'end' ? targetRight : targetLeft;
    var computations = [];

    for (var index = 0; index < frames.length; index++) {
      var frame = frames[index];

      var _frame$getBoundingCli = frame.getBoundingClientRect(),
          _height = _frame$getBoundingCli.height,
          _width = _frame$getBoundingCli.width,
          _top = _frame$getBoundingCli.top,
          right = _frame$getBoundingCli.right,
          bottom = _frame$getBoundingCli.bottom,
          _left = _frame$getBoundingCli.left;

      if (scrollMode === 'if-needed' && targetTop >= 0 && targetLeft >= 0 && targetBottom <= viewportHeight && targetRight <= viewportWidth && targetTop >= _top && targetBottom <= bottom && targetLeft >= _left && targetRight <= right) {
        return computations;
      }

      var frameStyle = getComputedStyle(frame);
      var borderLeft = parseInt(frameStyle.borderLeftWidth, 10);
      var borderTop = parseInt(frameStyle.borderTopWidth, 10);
      var borderRight = parseInt(frameStyle.borderRightWidth, 10);
      var borderBottom = parseInt(frameStyle.borderBottomWidth, 10);
      var blockScroll = 0;
      var inlineScroll = 0;
      var scrollbarWidth = 'offsetWidth' in frame ? frame.offsetWidth - frame.clientWidth - borderLeft - borderRight : 0;
      var scrollbarHeight = 'offsetHeight' in frame ? frame.offsetHeight - frame.clientHeight - borderTop - borderBottom : 0;

      if (scrollingElement === frame) {
        if (block === 'start') {
          blockScroll = targetBlock;
        } else if (block === 'end') {
          blockScroll = targetBlock - viewportHeight;
        } else if (block === 'nearest') {
          blockScroll = alignNearest(viewportY, viewportY + viewportHeight, viewportHeight, borderTop, borderBottom, viewportY + targetBlock, viewportY + targetBlock + targetHeight, targetHeight);
        } else {
          blockScroll = targetBlock - viewportHeight / 2;
        }

        if (inline === 'start') {
          inlineScroll = targetInline;
        } else if (inline === 'center') {
          inlineScroll = targetInline - viewportWidth / 2;
        } else if (inline === 'end') {
          inlineScroll = targetInline - viewportWidth;
        } else {
          inlineScroll = alignNearest(viewportX, viewportX + viewportWidth, viewportWidth, borderLeft, borderRight, viewportX + targetInline, viewportX + targetInline + targetWidth, targetWidth);
        }

        blockScroll = Math.max(0, blockScroll + viewportY);
        inlineScroll = Math.max(0, inlineScroll + viewportX);
      } else {
        if (block === 'start') {
          blockScroll = targetBlock - _top - borderTop;
        } else if (block === 'end') {
          blockScroll = targetBlock - bottom + borderBottom + scrollbarHeight;
        } else if (block === 'nearest') {
          blockScroll = alignNearest(_top, bottom, _height, borderTop, borderBottom + scrollbarHeight, targetBlock, targetBlock + targetHeight, targetHeight);
        } else {
          blockScroll = targetBlock - (_top + _height / 2) + scrollbarHeight / 2;
        }

        if (inline === 'start') {
          inlineScroll = targetInline - _left - borderLeft;
        } else if (inline === 'center') {
          inlineScroll = targetInline - (_left + _width / 2) + scrollbarWidth / 2;
        } else if (inline === 'end') {
          inlineScroll = targetInline - right + borderRight + scrollbarWidth;
        } else {
          inlineScroll = alignNearest(_left, right, _width, borderLeft, borderRight + scrollbarWidth, targetInline, targetInline + targetWidth, targetWidth);
        }

        var scrollLeft = frame.scrollLeft,
            scrollTop = frame.scrollTop;
        blockScroll = Math.max(0, Math.min(scrollTop + blockScroll, frame.scrollHeight - _height + scrollbarHeight));
        inlineScroll = Math.max(0, Math.min(scrollLeft + inlineScroll, frame.scrollWidth - _width + scrollbarWidth));
        targetBlock += scrollTop - blockScroll;
        targetInline += scrollLeft - inlineScroll;
      }

      computations.push({
        el: frame,
        top: blockScroll,
        left: inlineScroll
      });
    }

    return computations;
  });

  var idCounter = 0;
  /**
   * Accepts a parameter and returns it if it's a function
   * or a noop function if it's not. This allows us to
   * accept a callback, but not worry about it if it's not
   * passed.
   * @param {Function} cb the callback
   * @return {Function} a function
   */

  function cbToCb(cb) {
    return typeof cb === 'function' ? cb : noop;
  }

  function noop() {}
  /**
   * Scroll node into view if necessary
   * @param {HTMLElement} node the element that should scroll into view
   * @param {HTMLElement} menuNode the menu element of the component
   */


  function scrollIntoView(node, menuNode) {
    if (node === null) {
      return;
    }

    var actions = computeScrollIntoView(node, {
      boundary: menuNode,
      block: 'nearest',
      scrollMode: 'if-needed'
    });
    actions.forEach(function (_ref) {
      var el = _ref.el,
          top = _ref.top,
          left = _ref.left;
      el.scrollTop = top;
      el.scrollLeft = left;
    });
  }
  /**
   * @param {HTMLElement} parent the parent node
   * @param {HTMLElement} child the child node
   * @return {Boolean} whether the parent is the child or the child is in the parent
   */


  function isOrContainsNode(parent, child) {
    return parent === child || parent.contains && parent.contains(child);
  }
  /**
   * Simple debounce implementation. Will call the given
   * function once after the time given has passed since
   * it was last called.
   * @param {Function} fn the function to call after the time
   * @param {Number} time the time to wait
   * @return {Function} the debounced function
   */


  function debounce(fn, time) {
    var timeoutId;

    function cancel() {
      if (timeoutId) {
        clearTimeout(timeoutId);
      }
    }

    function wrapper() {
      for (var _len = arguments.length, args = new Array(_len), _key = 0; _key < _len; _key++) {
        args[_key] = arguments[_key];
      }

      cancel();
      timeoutId = setTimeout(function () {
        timeoutId = null;
        fn.apply(void 0, args);
      }, time);
    }

    wrapper.cancel = cancel;
    return wrapper;
  }
  /**
   * This is intended to be used to compose event handlers.
   * They are executed in order until one of them sets
   * `event.preventDownshiftDefault = true`.
   * @param {...Function} fns the event handler functions
   * @return {Function} the event handler to add to an element
   */


  function callAllEventHandlers() {
    for (var _len2 = arguments.length, fns = new Array(_len2), _key2 = 0; _key2 < _len2; _key2++) {
      fns[_key2] = arguments[_key2];
    }

    return function (event) {
      for (var _len3 = arguments.length, args = new Array(_len3 > 1 ? _len3 - 1 : 0), _key3 = 1; _key3 < _len3; _key3++) {
        args[_key3 - 1] = arguments[_key3];
      }

      return fns.some(function (fn) {
        if (fn) {
          fn.apply(void 0, [event].concat(args));
        }

        return event.preventDownshiftDefault || event.hasOwnProperty('nativeEvent') && event.nativeEvent.preventDownshiftDefault;
      });
    };
  }

  function handleRefs() {
    for (var _len4 = arguments.length, refs = new Array(_len4), _key4 = 0; _key4 < _len4; _key4++) {
      refs[_key4] = arguments[_key4];
    }

    return function (node) {
      refs.forEach(function (ref) {
        if (typeof ref === 'function') {
          ref(node);
        } else if (ref) {
          ref.current = node;
        }
      });
    };
  }
  /**
   * This generates a unique ID for an instance of Downshift
   * @return {String} the unique ID
   */


  function generateId() {
    return String(idCounter++);
  }
  /**
   * Resets idCounter to 0. Used for SSR.
   */


  function resetIdCounter() {
    idCounter = 0;
  }
  /**
   * @param {Object} param the downshift state and other relevant properties
   * @return {String} the a11y status message
   */


  function getA11yStatusMessage(_ref2) {
    var isOpen = _ref2.isOpen,
        selectedItem = _ref2.selectedItem,
        resultCount = _ref2.resultCount,
        previousResultCount = _ref2.previousResultCount,
        itemToString = _ref2.itemToString;

    if (!isOpen) {
      return selectedItem ? itemToString(selectedItem) : '';
    }

    if (!resultCount) {
      return 'No results are available.';
    }

    if (resultCount !== previousResultCount) {
      return resultCount + " result" + (resultCount === 1 ? ' is' : 's are') + " available, use up and down arrow keys to navigate. Press Enter key to select.";
    }

    return '';
  }
  /**
   * Takes an argument and if it's an array, returns the first item in the array
   * otherwise returns the argument
   * @param {*} arg the maybe-array
   * @param {*} defaultValue the value if arg is falsey not defined
   * @return {*} the arg or it's first item
   */


  function unwrapArray(arg, defaultValue) {
    arg = Array.isArray(arg) ?
    /* istanbul ignore next (preact) */
    arg[0] : arg;

    if (!arg && defaultValue) {
      return defaultValue;
    } else {
      return arg;
    }
  }
  /**
   * @param {Object} element (P)react element
   * @return {Boolean} whether it's a DOM element
   */


  function isDOMElement(element) {
    // then we assume this is react
    return typeof element.type === 'string';
  }
  /**
   * @param {Object} element (P)react element
   * @return {Object} the props
   */


  function getElementProps(element) {
    return element.props;
  }
  /**
   * Throws a helpful error message for required properties. Useful
   * to be used as a default in destructuring or object params.
   * @param {String} fnName the function name
   * @param {String} propName the prop name
   */


  function requiredProp(fnName, propName) {
    // eslint-disable-next-line no-console
    console.error("The property \"" + propName + "\" is required in \"" + fnName + "\"");
  }

  var stateKeys = ['highlightedIndex', 'inputValue', 'isOpen', 'selectedItem', 'type'];
  /**
   * @param {Object} state the state object
   * @return {Object} state that is relevant to downshift
   */

  function pickState(state) {
    if (state === void 0) {
      state = {};
    }

    var result = {};
    stateKeys.forEach(function (k) {
      if (state.hasOwnProperty(k)) {
        result[k] = state[k];
      }
    });
    return result;
  }
  /**
   * Normalizes the 'key' property of a KeyboardEvent in IE/Edge
   * @param {Object} event a keyboardEvent object
   * @return {String} keyboard key
   */


  function normalizeArrowKey(event) {
    var key = event.key,
        keyCode = event.keyCode;
    /* istanbul ignore next (ie) */

    if (keyCode >= 37 && keyCode <= 40 && key.indexOf('Arrow') !== 0) {
      return "Arrow" + key;
    }

    return key;
  }
  /**
   * Simple check if the value passed is object literal
   * @param {*} obj any things
   * @return {Boolean} whether it's object literal
   */


  function isPlainObject(obj) {
    return Object.prototype.toString.call(obj) === '[object Object]';
  }
  /**
   * Returns the new index in the list, in a circular way. If next value is out of bonds from the total,
   * it will wrap to either 0 or itemCount - 1.
   *
   * @param {number} moveAmount Number of positions to move. Negative to move backwards, positive forwards.
   * @param {number} baseIndex The initial position to move from.
   * @param {number} itemCount The total number of items.
   * @returns {number} The new index after the move.
   */


  function getNextWrappingIndex(moveAmount, baseIndex, itemCount) {
    var itemsLastIndex = itemCount - 1;

    if (typeof baseIndex !== 'number' || baseIndex < 0 || baseIndex >= itemCount) {
      baseIndex = moveAmount > 0 ? -1 : itemsLastIndex + 1;
    }

    var newIndex = baseIndex + moveAmount;

    if (newIndex < 0) {
      newIndex = itemsLastIndex;
    } else if (newIndex > itemsLastIndex) {
      newIndex = 0;
    }

    return newIndex;
  }

  var cleanupStatus = debounce(function () {
    getStatusDiv().textContent = '';
  }, 500);
  /**
   * @param {String} status the status message
   * @param {Object} documentProp document passed by the user.
   */

  function setStatus(status, documentProp) {
    var div = getStatusDiv(documentProp);

    if (!status) {
      return;
    }

    div.textContent = status;
    cleanupStatus();
  }
  /**
   * Get the status node or create it if it does not already exist.
   * @param {Object} documentProp document passed by the user.
   * @return {HTMLElement} the status node.
   */


  function getStatusDiv(documentProp) {
    if (documentProp === void 0) {
      documentProp = document;
    }

    var statusDiv = documentProp.getElementById('a11y-status-message');

    if (statusDiv) {
      return statusDiv;
    }

    statusDiv = documentProp.createElement('div');
    statusDiv.setAttribute('id', 'a11y-status-message');
    statusDiv.setAttribute('role', 'status');
    statusDiv.setAttribute('aria-live', 'polite');
    statusDiv.setAttribute('aria-relevant', 'additions text');
    Object.assign(statusDiv.style, {
      border: '0',
      clip: 'rect(0 0 0 0)',
      height: '1px',
      margin: '-1px',
      overflow: 'hidden',
      padding: '0',
      position: 'absolute',
      width: '1px'
    });
    documentProp.body.appendChild(statusDiv);
    return statusDiv;
  }

  var unknown = '__autocomplete_unknown__';
  var mouseUp = '__autocomplete_mouseup__';
  var itemMouseEnter = '__autocomplete_item_mouseenter__';
  var keyDownArrowUp = '__autocomplete_keydown_arrow_up__';
  var keyDownArrowDown = '__autocomplete_keydown_arrow_down__';
  var keyDownEscape = '__autocomplete_keydown_escape__';
  var keyDownEnter = '__autocomplete_keydown_enter__';
  var keyDownHome = '__autocomplete_keydown_home__';
  var keyDownEnd = '__autocomplete_keydown_end__';
  var clickItem = '__autocomplete_click_item__';
  var blurInput = '__autocomplete_blur_input__';
  var changeInput = '__autocomplete_change_input__';
  var keyDownSpaceButton = '__autocomplete_keydown_space_button__';
  var clickButton = '__autocomplete_click_button__';
  var blurButton = '__autocomplete_blur_button__';
  var controlledPropUpdatedSelectedItem = '__autocomplete_controlled_prop_updated_selected_item__';
  var touchEnd = '__autocomplete_touchend__';

  var stateChangeTypes = /*#__PURE__*/Object.freeze({
    __proto__: null,
    unknown: unknown,
    mouseUp: mouseUp,
    itemMouseEnter: itemMouseEnter,
    keyDownArrowUp: keyDownArrowUp,
    keyDownArrowDown: keyDownArrowDown,
    keyDownEscape: keyDownEscape,
    keyDownEnter: keyDownEnter,
    keyDownHome: keyDownHome,
    keyDownEnd: keyDownEnd,
    clickItem: clickItem,
    blurInput: blurInput,
    changeInput: changeInput,
    keyDownSpaceButton: keyDownSpaceButton,
    clickButton: clickButton,
    blurButton: blurButton,
    controlledPropUpdatedSelectedItem: controlledPropUpdatedSelectedItem,
    touchEnd: touchEnd
  });

  var Downshift =
  /*#__PURE__*/
  function () {
    var Downshift =
    /*#__PURE__*/
    function (_Component) {
      _inheritsLoose(Downshift, _Component);

      function Downshift(_props) {
        var _this = _Component.call(this, _props) || this;

        _this.id = _this.props.id || "downshift-" + generateId();
        _this.menuId = _this.props.menuId || _this.id + "-menu";
        _this.labelId = _this.props.labelId || _this.id + "-label";
        _this.inputId = _this.props.inputId || _this.id + "-input";

        _this.getItemId = _this.props.getItemId || function (index) {
          return _this.id + "-item-" + index;
        };

        _this.input = null;
        _this.items = [];
        _this.itemCount = null;
        _this.previousResultCount = 0;
        _this.timeoutIds = [];

        _this.internalSetTimeout = function (fn, time) {
          var id = setTimeout(function () {
            _this.timeoutIds = _this.timeoutIds.filter(function (i) {
              return i !== id;
            });
            fn();
          }, time);

          _this.timeoutIds.push(id);
        };

        _this.setItemCount = function (count) {
          _this.itemCount = count;
        };

        _this.unsetItemCount = function () {
          _this.itemCount = null;
        };

        _this.setHighlightedIndex = function (highlightedIndex, otherStateToSet) {
          if (highlightedIndex === void 0) {
            highlightedIndex = _this.props.defaultHighlightedIndex;
          }

          if (otherStateToSet === void 0) {
            otherStateToSet = {};
          }

          otherStateToSet = pickState(otherStateToSet);

          _this.internalSetState(_extends({
            highlightedIndex: highlightedIndex
          }, otherStateToSet));
        };

        _this.clearSelection = function (cb) {
          _this.internalSetState({
            selectedItem: null,
            inputValue: '',
            highlightedIndex: _this.props.defaultHighlightedIndex,
            isOpen: _this.props.defaultIsOpen
          }, cb);
        };

        _this.selectItem = function (item, otherStateToSet, cb) {
          otherStateToSet = pickState(otherStateToSet);

          _this.internalSetState(_extends({
            isOpen: _this.props.defaultIsOpen,
            highlightedIndex: _this.props.defaultHighlightedIndex,
            selectedItem: item,
            inputValue: _this.props.itemToString(item)
          }, otherStateToSet), cb);
        };

        _this.selectItemAtIndex = function (itemIndex, otherStateToSet, cb) {
          var item = _this.items[itemIndex];

          if (item == null) {
            return;
          }

          _this.selectItem(item, otherStateToSet, cb);
        };

        _this.selectHighlightedItem = function (otherStateToSet, cb) {
          return _this.selectItemAtIndex(_this.getState().highlightedIndex, otherStateToSet, cb);
        };

        _this.internalSetState = function (stateToSet, cb) {
          var isItemSelected, onChangeArg;
          var onStateChangeArg = {};
          var isStateToSetFunction = typeof stateToSet === 'function'; // we want to call `onInputValueChange` before the `setState` call
          // so someone controlling the `inputValue` state gets notified of
          // the input change as soon as possible. This avoids issues with
          // preserving the cursor position.
          // See https://github.com/downshift-js/downshift/issues/217 for more info.

          if (!isStateToSetFunction && stateToSet.hasOwnProperty('inputValue')) {
            _this.props.onInputValueChange(stateToSet.inputValue, _extends({}, _this.getStateAndHelpers(), {}, stateToSet));
          }

          return _this.setState(function (state) {
            state = _this.getState(state);
            var newStateToSet = isStateToSetFunction ? stateToSet(state) : stateToSet; // Your own function that could modify the state that will be set.

            newStateToSet = _this.props.stateReducer(state, newStateToSet); // checks if an item is selected, regardless of if it's different from
            // what was selected before
            // used to determine if onSelect and onChange callbacks should be called

            isItemSelected = newStateToSet.hasOwnProperty('selectedItem'); // this keeps track of the object we want to call with setState

            var nextState = {}; // this is just used to tell whether the state changed

            var nextFullState = {}; // we need to call on change if the outside world is controlling any of our state
            // and we're trying to update that state. OR if the selection has changed and we're
            // trying to update the selection

            if (isItemSelected && newStateToSet.selectedItem !== state.selectedItem) {
              onChangeArg = newStateToSet.selectedItem;
            }

            newStateToSet.type = newStateToSet.type || unknown;
            Object.keys(newStateToSet).forEach(function (key) {
              // onStateChangeArg should only have the state that is
              // actually changing
              if (state[key] !== newStateToSet[key]) {
                onStateChangeArg[key] = newStateToSet[key];
              } // the type is useful for the onStateChangeArg
              // but we don't actually want to set it in internal state.
              // this is an undocumented feature for now... Not all internalSetState
              // calls support it and I'm not certain we want them to yet.
              // But it enables users controlling the isOpen state to know when
              // the isOpen state changes due to mouseup events which is quite handy.


              if (key === 'type') {
                return;
              }

              nextFullState[key] = newStateToSet[key]; // if it's coming from props, then we don't care to set it internally

              if (!_this.isControlledProp(key)) {
                nextState[key] = newStateToSet[key];
              }
            }); // if stateToSet is a function, then we weren't able to call onInputValueChange
            // earlier, so we'll call it now that we know what the inputValue state will be.

            if (isStateToSetFunction && newStateToSet.hasOwnProperty('inputValue')) {
              _this.props.onInputValueChange(newStateToSet.inputValue, _extends({}, _this.getStateAndHelpers(), {}, newStateToSet));
            }

            return nextState;
          }, function () {
            // call the provided callback if it's a function
            cbToCb(cb)(); // only call the onStateChange and onChange callbacks if
            // we have relevant information to pass them.

            var hasMoreStateThanType = Object.keys(onStateChangeArg).length > 1;

            if (hasMoreStateThanType) {
              _this.props.onStateChange(onStateChangeArg, _this.getStateAndHelpers());
            }

            if (isItemSelected) {
              _this.props.onSelect(stateToSet.selectedItem, _this.getStateAndHelpers());
            }

            if (onChangeArg !== undefined) {
              _this.props.onChange(onChangeArg, _this.getStateAndHelpers());
            } // this is currently undocumented and therefore subject to change
            // We'll try to not break it, but just be warned.


            _this.props.onUserAction(onStateChangeArg, _this.getStateAndHelpers());
          });
        };

        _this.rootRef = function (node) {
          return _this._rootNode = node;
        };

        _this.getRootProps = function (_temp, _temp2) {
          var _extends2;

          var _ref = _temp === void 0 ? {} : _temp,
              _ref$refKey = _ref.refKey,
              refKey = _ref$refKey === void 0 ? 'ref' : _ref$refKey,
              ref = _ref.ref,
              rest = _objectWithoutPropertiesLoose(_ref, ["refKey", "ref"]);

          var _ref2 = _temp2 === void 0 ? {} : _temp2,
              _ref2$suppressRefErro = _ref2.suppressRefError,
              suppressRefError = _ref2$suppressRefErro === void 0 ? false : _ref2$suppressRefErro;

          // this is used in the render to know whether the user has called getRootProps.
          // It uses that to know whether to apply the props automatically
          _this.getRootProps.called = true;
          _this.getRootProps.refKey = refKey;
          _this.getRootProps.suppressRefError = suppressRefError;

          var _this$getState = _this.getState(),
              isOpen = _this$getState.isOpen;

          return _extends((_extends2 = {}, _extends2[refKey] = handleRefs(ref, _this.rootRef), _extends2.role = 'combobox', _extends2['aria-expanded'] = isOpen, _extends2['aria-haspopup'] = 'listbox', _extends2['aria-owns'] = isOpen ? _this.menuId : null, _extends2['aria-labelledby'] = _this.labelId, _extends2), rest);
        };

        _this.keyDownHandlers = {
          ArrowDown: function ArrowDown(event) {
            var _this2 = this;

            event.preventDefault();

            if (this.getState().isOpen) {
              var amount = event.shiftKey ? 5 : 1;
              this.moveHighlightedIndex(amount, {
                type: keyDownArrowDown
              });
            } else {
              this.internalSetState({
                isOpen: true,
                type: keyDownArrowDown
              }, function () {
                var itemCount = _this2.getItemCount();

                if (itemCount > 0) {
                  _this2.setHighlightedIndex(getNextWrappingIndex(1, _this2.getState().highlightedIndex, itemCount), {
                    type: keyDownArrowDown
                  });
                }
              });
            }
          },
          ArrowUp: function ArrowUp(event) {
            var _this3 = this;

            event.preventDefault();

            if (this.getState().isOpen) {
              var amount = event.shiftKey ? -5 : -1;
              this.moveHighlightedIndex(amount, {
                type: keyDownArrowUp
              });
            } else {
              this.internalSetState({
                isOpen: true,
                type: keyDownArrowUp
              }, function () {
                var itemCount = _this3.getItemCount();

                if (itemCount > 0) {
                  _this3.setHighlightedIndex(getNextWrappingIndex(-1, _this3.getState().highlightedIndex, itemCount), {
                    type: keyDownArrowDown
                  });
                }
              });
            }
          },
          Enter: function Enter(event) {
            var _this$getState2 = this.getState(),
                isOpen = _this$getState2.isOpen,
                highlightedIndex = _this$getState2.highlightedIndex;

            if (isOpen && highlightedIndex != null) {
              event.preventDefault();
              var item = this.items[highlightedIndex];
              var itemNode = this.getItemNodeFromIndex(highlightedIndex);

              if (item == null || itemNode && itemNode.hasAttribute('disabled')) {
                return;
              }

              this.selectHighlightedItem({
                type: keyDownEnter
              });
            }
          },
          Escape: function Escape(event) {
            event.preventDefault();
            this.reset({
              type: keyDownEscape,
              selectedItem: null,
              inputValue: ''
            });
          }
        };
        _this.buttonKeyDownHandlers = _extends({}, _this.keyDownHandlers, {
          ' ': function _(event) {
            event.preventDefault();
            this.toggleMenu({
              type: keyDownSpaceButton
            });
          }
        });
        _this.inputKeyDownHandlers = _extends({}, _this.keyDownHandlers, {
          Home: function Home(event) {
            this.highlightFirstOrLastIndex(event, true, {
              type: keyDownHome
            });
          },
          End: function End(event) {
            this.highlightFirstOrLastIndex(event, false, {
              type: keyDownEnd
            });
          }
        });

        _this.getToggleButtonProps = function (_temp3) {
          var _ref3 = _temp3 === void 0 ? {} : _temp3,
              onClick = _ref3.onClick,
              onPress = _ref3.onPress,
              onKeyDown = _ref3.onKeyDown,
              onKeyUp = _ref3.onKeyUp,
              onBlur = _ref3.onBlur,
              rest = _objectWithoutPropertiesLoose(_ref3, ["onClick", "onPress", "onKeyDown", "onKeyUp", "onBlur"]);

          var _this$getState3 = _this.getState(),
              isOpen = _this$getState3.isOpen;

          var enabledEventHandlers = {
            onClick: callAllEventHandlers(onClick, _this.buttonHandleClick),
            onKeyDown: callAllEventHandlers(onKeyDown, _this.buttonHandleKeyDown),
            onKeyUp: callAllEventHandlers(onKeyUp, _this.buttonHandleKeyUp),
            onBlur: callAllEventHandlers(onBlur, _this.buttonHandleBlur)
          };
          var eventHandlers = rest.disabled ? {} : enabledEventHandlers;
          return _extends({
            type: 'button',
            role: 'button',
            'aria-label': isOpen ? 'close menu' : 'open menu',
            'aria-haspopup': true,
            'data-toggle': true
          }, eventHandlers, {}, rest);
        };

        _this.buttonHandleKeyUp = function (event) {
          // Prevent click event from emitting in Firefox
          event.preventDefault();
        };

        _this.buttonHandleKeyDown = function (event) {
          var key = normalizeArrowKey(event);

          if (_this.buttonKeyDownHandlers[key]) {
            _this.buttonKeyDownHandlers[key].call(_assertThisInitialized(_this), event);
          }
        };

        _this.buttonHandleClick = function (event) {
          event.preventDefault(); // handle odd case for Safari and Firefox which
          // don't give the button the focus properly.

          /* istanbul ignore if (can't reasonably test this) */

          if ( _this.props.environment.document.activeElement === _this.props.environment.document.body) {
            event.target.focus();
          } // to simplify testing components that use downshift, we'll not wrap this in a setTimeout
          // if the NODE_ENV is test. With the proper build system, this should be dead code eliminated
          // when building for production and should therefore have no impact on production code.


          // Ensure that toggle of menu occurs after the potential blur event in iOS
          _this.internalSetTimeout(function () {
            return _this.toggleMenu({
              type: clickButton
            });
          });
        };

        _this.buttonHandleBlur = function (event) {
          var blurTarget = event.target; // Save blur target for comparison with activeElement later
          // Need setTimeout, so that when the user presses Tab, the activeElement is the next focused element, not body element

          _this.internalSetTimeout(function () {
            if (!_this.isMouseDown && (_this.props.environment.document.activeElement == null || _this.props.environment.document.activeElement.id !== _this.inputId) && _this.props.environment.document.activeElement !== blurTarget // Do nothing if we refocus the same element again (to solve issue in Safari on iOS)
            ) {
                _this.reset({
                  type: blurButton
                });
              }
          });
        };

        _this.getLabelProps = function (props) {
          return _extends({
            htmlFor: _this.inputId,
            id: _this.labelId
          }, props);
        };

        _this.getInputProps = function (_temp4) {
          var _ref4 = _temp4 === void 0 ? {} : _temp4,
              onKeyDown = _ref4.onKeyDown,
              onBlur = _ref4.onBlur,
              onChange = _ref4.onChange,
              onInput = _ref4.onInput,
              onChangeText = _ref4.onChangeText,
              rest = _objectWithoutPropertiesLoose(_ref4, ["onKeyDown", "onBlur", "onChange", "onInput", "onChangeText"]);

          var onChangeKey;
          var eventHandlers = {};
          /* istanbul ignore next (preact) */

          onChangeKey = 'onChange';

          var _this$getState4 = _this.getState(),
              inputValue = _this$getState4.inputValue,
              isOpen = _this$getState4.isOpen,
              highlightedIndex = _this$getState4.highlightedIndex;

          if (!rest.disabled) {
            var _eventHandlers;

            eventHandlers = (_eventHandlers = {}, _eventHandlers[onChangeKey] = callAllEventHandlers(onChange, onInput, _this.inputHandleChange), _eventHandlers.onKeyDown = callAllEventHandlers(onKeyDown, _this.inputHandleKeyDown), _eventHandlers.onBlur = callAllEventHandlers(onBlur, _this.inputHandleBlur), _eventHandlers);
          }
          /* istanbul ignore if (react-native) */


          return _extends({
            'aria-autocomplete': 'list',
            'aria-activedescendant': isOpen && typeof highlightedIndex === 'number' && highlightedIndex >= 0 ? _this.getItemId(highlightedIndex) : null,
            'aria-controls': isOpen ? _this.menuId : null,
            'aria-labelledby': _this.labelId,
            // https://developer.mozilla.org/en-US/docs/Web/Security/Securing_your_site/Turning_off_form_autocompletion
            // revert back since autocomplete="nope" is ignored on latest Chrome and Opera
            autoComplete: 'off',
            value: inputValue,
            id: _this.inputId
          }, eventHandlers, {}, rest);
        };

        _this.inputHandleKeyDown = function (event) {
          var key = normalizeArrowKey(event);

          if (key && _this.inputKeyDownHandlers[key]) {
            _this.inputKeyDownHandlers[key].call(_assertThisInitialized(_this), event);
          }
        };

        _this.inputHandleChange = function (event) {
          _this.internalSetState({
            type: changeInput,
            isOpen: true,
            inputValue: event.target.value,
            highlightedIndex: _this.props.defaultHighlightedIndex
          });
        };

        _this.inputHandleBlur = function () {
          // Need setTimeout, so that when the user presses Tab, the activeElement is the next focused element, not the body element
          _this.internalSetTimeout(function () {
            var downshiftButtonIsActive = _this.props.environment.document && !!_this.props.environment.document.activeElement && !!_this.props.environment.document.activeElement.dataset && _this.props.environment.document.activeElement.dataset.toggle && _this._rootNode && _this._rootNode.contains(_this.props.environment.document.activeElement);

            if (!_this.isMouseDown && !downshiftButtonIsActive) {
              _this.reset({
                type: blurInput
              });
            }
          });
        };

        _this.menuRef = function (node) {
          _this._menuNode = node;
        };

        _this.getMenuProps = function (_temp5, _temp6) {
          var _extends3;

          var _ref5 = _temp5 === void 0 ? {} : _temp5,
              _ref5$refKey = _ref5.refKey,
              refKey = _ref5$refKey === void 0 ? 'ref' : _ref5$refKey,
              ref = _ref5.ref,
              props = _objectWithoutPropertiesLoose(_ref5, ["refKey", "ref"]);

          var _ref6 = _temp6 === void 0 ? {} : _temp6,
              _ref6$suppressRefErro = _ref6.suppressRefError,
              suppressRefError = _ref6$suppressRefErro === void 0 ? false : _ref6$suppressRefErro;

          _this.getMenuProps.called = true;
          _this.getMenuProps.refKey = refKey;
          _this.getMenuProps.suppressRefError = suppressRefError;
          return _extends((_extends3 = {}, _extends3[refKey] = handleRefs(ref, _this.menuRef), _extends3.role = 'listbox', _extends3['aria-labelledby'] = props && props['aria-label'] ? null : _this.labelId, _extends3.id = _this.menuId, _extends3), props);
        };

        _this.getItemProps = function (_temp7) {
          var _enabledEventHandlers;

          var _ref7 = _temp7 === void 0 ? {} : _temp7,
              onMouseMove = _ref7.onMouseMove,
              onMouseDown = _ref7.onMouseDown,
              onClick = _ref7.onClick,
              onPress = _ref7.onPress,
              index = _ref7.index,
              _ref7$item = _ref7.item,
              item = _ref7$item === void 0 ? requiredProp('getItemProps', 'item') : _ref7$item,
              rest = _objectWithoutPropertiesLoose(_ref7, ["onMouseMove", "onMouseDown", "onClick", "onPress", "index", "item"]);

          if (index === undefined) {
            _this.items.push(item);

            index = _this.items.indexOf(item);
          } else {
            _this.items[index] = item;
          }

          var onSelectKey = 'onClick';
          var customClickHandler = onClick;
          var enabledEventHandlers = (_enabledEventHandlers = {
            // onMouseMove is used over onMouseEnter here. onMouseMove
            // is only triggered on actual mouse movement while onMouseEnter
            // can fire on DOM changes, interrupting keyboard navigation
            onMouseMove: callAllEventHandlers(onMouseMove, function () {
              if (index === _this.getState().highlightedIndex) {
                return;
              }

              _this.setHighlightedIndex(index, {
                type: itemMouseEnter
              }); // We never want to manually scroll when changing state based
              // on `onMouseMove` because we will be moving the element out
              // from under the user which is currently scrolling/moving the
              // cursor


              _this.avoidScrolling = true;

              _this.internalSetTimeout(function () {
                return _this.avoidScrolling = false;
              }, 250);
            }),
            onMouseDown: callAllEventHandlers(onMouseDown, function (event) {
              // This prevents the activeElement from being changed
              // to the item so it can remain with the current activeElement
              // which is a more common use case.
              event.preventDefault();
            })
          }, _enabledEventHandlers[onSelectKey] = callAllEventHandlers(customClickHandler, function () {
            _this.selectItemAtIndex(index, {
              type: clickItem
            });
          }), _enabledEventHandlers); // Passing down the onMouseDown handler to prevent redirect
          // of the activeElement if clicking on disabled items

          var eventHandlers = rest.disabled ? {
            onMouseDown: enabledEventHandlers.onMouseDown
          } : enabledEventHandlers;
          return _extends({
            id: _this.getItemId(index),
            role: 'option',
            'aria-selected': _this.getState().highlightedIndex === index
          }, eventHandlers, {}, rest);
        };

        _this.clearItems = function () {
          _this.items = [];
        };

        _this.reset = function (otherStateToSet, cb) {
          if (otherStateToSet === void 0) {
            otherStateToSet = {};
          }

          otherStateToSet = pickState(otherStateToSet);

          _this.internalSetState(function (_ref8) {
            var selectedItem = _ref8.selectedItem;
            return _extends({
              isOpen: _this.props.defaultIsOpen,
              highlightedIndex: _this.props.defaultHighlightedIndex,
              inputValue: _this.props.itemToString(selectedItem)
            }, otherStateToSet);
          }, cb);
        };

        _this.toggleMenu = function (otherStateToSet, cb) {
          if (otherStateToSet === void 0) {
            otherStateToSet = {};
          }

          otherStateToSet = pickState(otherStateToSet);

          _this.internalSetState(function (_ref9) {
            var isOpen = _ref9.isOpen;
            return _extends({
              isOpen: !isOpen
            }, isOpen && {
              highlightedIndex: _this.props.defaultHighlightedIndex
            }, {}, otherStateToSet);
          }, function () {
            var _this$getState5 = _this.getState(),
                isOpen = _this$getState5.isOpen,
                highlightedIndex = _this$getState5.highlightedIndex;

            if (isOpen) {
              if (_this.getItemCount() > 0 && typeof highlightedIndex === 'number') {
                _this.setHighlightedIndex(highlightedIndex, otherStateToSet);
              }
            }

            cbToCb(cb)();
          });
        };

        _this.openMenu = function (cb) {
          _this.internalSetState({
            isOpen: true
          }, cb);
        };

        _this.closeMenu = function (cb) {
          _this.internalSetState({
            isOpen: false
          }, cb);
        };

        _this.updateStatus = debounce(function () {
          var state = _this.getState();

          var item = _this.items[state.highlightedIndex];

          var resultCount = _this.getItemCount();

          var status = _this.props.getA11yStatusMessage(_extends({
            itemToString: _this.props.itemToString,
            previousResultCount: _this.previousResultCount,
            resultCount: resultCount,
            highlightedItem: item
          }, state));

          _this.previousResultCount = resultCount;
          setStatus(status, _this.props.environment.document);
        }, 200);

        // fancy destructuring + defaults + aliases
        // this basically says each value of state should either be set to
        // the initial value or the default value if the initial value is not provided
        var _this$props = _this.props,
            defaultHighlightedIndex = _this$props.defaultHighlightedIndex,
            _this$props$initialHi = _this$props.initialHighlightedIndex,
            _highlightedIndex = _this$props$initialHi === void 0 ? defaultHighlightedIndex : _this$props$initialHi,
            defaultIsOpen = _this$props.defaultIsOpen,
            _this$props$initialIs = _this$props.initialIsOpen,
            _isOpen = _this$props$initialIs === void 0 ? defaultIsOpen : _this$props$initialIs,
            _this$props$initialIn = _this$props.initialInputValue,
            _inputValue = _this$props$initialIn === void 0 ? '' : _this$props$initialIn,
            _this$props$initialSe = _this$props.initialSelectedItem,
            _selectedItem = _this$props$initialSe === void 0 ? null : _this$props$initialSe;

        var _state = _this.getState({
          highlightedIndex: _highlightedIndex,
          isOpen: _isOpen,
          inputValue: _inputValue,
          selectedItem: _selectedItem
        });

        if (_state.selectedItem != null && _this.props.initialInputValue === undefined) {
          _state.inputValue = _this.props.itemToString(_state.selectedItem);
        }

        _this.state = _state;
        return _this;
      }

      var _proto = Downshift.prototype;

      /**
       * Clear all running timeouts
       */
      _proto.internalClearTimeouts = function internalClearTimeouts() {
        this.timeoutIds.forEach(function (id) {
          clearTimeout(id);
        });
        this.timeoutIds = [];
      }
      /**
       * Gets the state based on internal state or props
       * If a state value is passed via props, then that
       * is the value given, otherwise it's retrieved from
       * stateToMerge
       *
       * This will perform a shallow merge of the given state object
       * with the state coming from props
       * (for the controlled component scenario)
       * This is used in state updater functions so they're referencing
       * the right state regardless of where it comes from.
       *
       * @param {Object} stateToMerge defaults to this.state
       * @return {Object} the state
       */
      ;

      _proto.getState = function getState(stateToMerge) {
        var _this4 = this;

        if (stateToMerge === void 0) {
          stateToMerge = this.state;
        }

        return Object.keys(stateToMerge).reduce(function (state, key) {
          state[key] = _this4.isControlledProp(key) ? _this4.props[key] : stateToMerge[key];
          return state;
        }, {});
      }
      /**
       * This determines whether a prop is a "controlled prop" meaning it is
       * state which is controlled by the outside of this component rather
       * than within this component.
       * @param {String} key the key to check
       * @return {Boolean} whether it is a controlled controlled prop
       */
      ;

      _proto.isControlledProp = function isControlledProp(key) {
        return this.props[key] !== undefined;
      };

      _proto.getItemCount = function getItemCount() {
        // things read better this way. They're in priority order:
        // 1. `this.itemCount`
        // 2. `this.props.itemCount`
        // 3. `this.items.length`
        var itemCount = this.items.length;

        if (this.itemCount != null) {
          itemCount = this.itemCount;
        } else if (this.props.itemCount !== undefined) {
          itemCount = this.props.itemCount;
        }

        return itemCount;
      };

      _proto.getItemNodeFromIndex = function getItemNodeFromIndex(index) {
        return this.props.environment.document.getElementById(this.getItemId(index));
      };

      _proto.scrollHighlightedItemIntoView = function scrollHighlightedItemIntoView() {
        /* istanbul ignore else (react-native) */
        {
          var node = this.getItemNodeFromIndex(this.getState().highlightedIndex);
          this.props.scrollIntoView(node, this._menuNode);
        }
      };

      _proto.moveHighlightedIndex = function moveHighlightedIndex(amount, otherStateToSet) {
        var itemCount = this.getItemCount();

        if (itemCount > 0) {
          var nextHighlightedIndex = getNextWrappingIndex(amount, this.getState().highlightedIndex, itemCount);
          this.setHighlightedIndex(nextHighlightedIndex, otherStateToSet);
        }
      };

      _proto.highlightFirstOrLastIndex = function highlightFirstOrLastIndex(event, first, otherStateToSet) {
        var itemsLastIndex = this.getItemCount() - 1;

        if (itemsLastIndex < 0 || !this.getState().isOpen) {
          return;
        }

        event.preventDefault();
        this.setHighlightedIndex(first ? 0 : itemsLastIndex, otherStateToSet);
      };

      _proto.getStateAndHelpers = function getStateAndHelpers() {
        var _this$getState6 = this.getState(),
            highlightedIndex = _this$getState6.highlightedIndex,
            inputValue = _this$getState6.inputValue,
            selectedItem = _this$getState6.selectedItem,
            isOpen = _this$getState6.isOpen;

        var itemToString = this.props.itemToString;
        var id = this.id;
        var getRootProps = this.getRootProps,
            getToggleButtonProps = this.getToggleButtonProps,
            getLabelProps = this.getLabelProps,
            getMenuProps = this.getMenuProps,
            getInputProps = this.getInputProps,
            getItemProps = this.getItemProps,
            openMenu = this.openMenu,
            closeMenu = this.closeMenu,
            toggleMenu = this.toggleMenu,
            selectItem = this.selectItem,
            selectItemAtIndex = this.selectItemAtIndex,
            selectHighlightedItem = this.selectHighlightedItem,
            setHighlightedIndex = this.setHighlightedIndex,
            clearSelection = this.clearSelection,
            clearItems = this.clearItems,
            reset = this.reset,
            setItemCount = this.setItemCount,
            unsetItemCount = this.unsetItemCount,
            setState = this.internalSetState;
        return {
          // prop getters
          getRootProps: getRootProps,
          getToggleButtonProps: getToggleButtonProps,
          getLabelProps: getLabelProps,
          getMenuProps: getMenuProps,
          getInputProps: getInputProps,
          getItemProps: getItemProps,
          // actions
          reset: reset,
          openMenu: openMenu,
          closeMenu: closeMenu,
          toggleMenu: toggleMenu,
          selectItem: selectItem,
          selectItemAtIndex: selectItemAtIndex,
          selectHighlightedItem: selectHighlightedItem,
          setHighlightedIndex: setHighlightedIndex,
          clearSelection: clearSelection,
          clearItems: clearItems,
          setItemCount: setItemCount,
          unsetItemCount: unsetItemCount,
          setState: setState,
          // props
          itemToString: itemToString,
          // derived
          id: id,
          // state
          highlightedIndex: highlightedIndex,
          inputValue: inputValue,
          isOpen: isOpen,
          selectedItem: selectedItem
        };
      } //////////////////////////// ROOT
      ;

      _proto.componentDidMount = function componentDidMount() {
        var _this5 = this;

        /* istanbul ignore if (react-native) */
        if ( this.getMenuProps.called && !this.getMenuProps.suppressRefError) {
          validateGetMenuPropsCalledCorrectly(this._menuNode, this.getMenuProps);
        }
        /* istanbul ignore if (react-native) */


        {
          var targetWithinDownshift = function (target, checkActiveElement) {
            if (checkActiveElement === void 0) {
              checkActiveElement = true;
            }

            var document = _this5.props.environment.document;
            return [_this5._rootNode, _this5._menuNode].some(function (contextNode) {
              return contextNode && (isOrContainsNode(contextNode, target) || checkActiveElement && isOrContainsNode(contextNode, document.activeElement));
            });
          }; // this.isMouseDown helps us track whether the mouse is currently held down.
          // This is useful when the user clicks on an item in the list, but holds the mouse
          // down long enough for the list to disappear (because the blur event fires on the input)
          // this.isMouseDown is used in the blur handler on the input to determine whether the blur event should
          // trigger hiding the menu.


          var onMouseDown = function () {
            _this5.isMouseDown = true;
          };

          var onMouseUp = function (event) {
            _this5.isMouseDown = false; // if the target element or the activeElement is within a downshift node
            // then we don't want to reset downshift

            var contextWithinDownshift = targetWithinDownshift(event.target);

            if (!contextWithinDownshift && _this5.getState().isOpen) {
              _this5.reset({
                type: mouseUp
              }, function () {
                return _this5.props.onOuterClick(_this5.getStateAndHelpers());
              });
            }
          }; // Touching an element in iOS gives focus and hover states, but touching out of
          // the element will remove hover, and persist the focus state, resulting in the
          // blur event not being triggered.
          // this.isTouchMove helps us track whether the user is tapping or swiping on a touch screen.
          // If the user taps outside of Downshift, the component should be reset,
          // but not if the user is swiping


          var onTouchStart = function () {
            _this5.isTouchMove = false;
          };

          var onTouchMove = function () {
            _this5.isTouchMove = true;
          };

          var onTouchEnd = function (event) {
            var contextWithinDownshift = targetWithinDownshift(event.target, false);

            if (!_this5.isTouchMove && !contextWithinDownshift && _this5.getState().isOpen) {
              _this5.reset({
                type: touchEnd
              }, function () {
                return _this5.props.onOuterClick(_this5.getStateAndHelpers());
              });
            }
          };

          var environment = this.props.environment;
          environment.addEventListener('mousedown', onMouseDown);
          environment.addEventListener('mouseup', onMouseUp);
          environment.addEventListener('touchstart', onTouchStart);
          environment.addEventListener('touchmove', onTouchMove);
          environment.addEventListener('touchend', onTouchEnd);

          this.cleanup = function () {
            _this5.internalClearTimeouts();

            _this5.updateStatus.cancel();

            environment.removeEventListener('mousedown', onMouseDown);
            environment.removeEventListener('mouseup', onMouseUp);
            environment.removeEventListener('touchstart', onTouchStart);
            environment.removeEventListener('touchmove', onTouchMove);
            environment.removeEventListener('touchend', onTouchEnd);
          };
        }
      };

      _proto.shouldScroll = function shouldScroll(prevState, prevProps) {
        var _ref10 = this.props.highlightedIndex === undefined ? this.getState() : this.props,
            currentHighlightedIndex = _ref10.highlightedIndex;

        var _ref11 = prevProps.highlightedIndex === undefined ? prevState : prevProps,
            prevHighlightedIndex = _ref11.highlightedIndex;

        var scrollWhenOpen = currentHighlightedIndex && this.getState().isOpen && !prevState.isOpen;
        return scrollWhenOpen || currentHighlightedIndex !== prevHighlightedIndex;
      };

      _proto.componentDidUpdate = function componentDidUpdate(prevProps, prevState) {
        validateControlledUnchanged(prevProps, this.props);
        /* istanbul ignore if (react-native) */

        if ( this.getMenuProps.called && !this.getMenuProps.suppressRefError) {
          validateGetMenuPropsCalledCorrectly(this._menuNode, this.getMenuProps);
        }

        if (this.isControlledProp('selectedItem') && this.props.selectedItemChanged(prevProps.selectedItem, this.props.selectedItem)) {
          this.internalSetState({
            type: controlledPropUpdatedSelectedItem,
            inputValue: this.props.itemToString(this.props.selectedItem)
          });
        }

        if (!this.avoidScrolling && this.shouldScroll(prevState, prevProps)) {
          this.scrollHighlightedItemIntoView();
        }
        /* istanbul ignore else (react-native) */


        this.updateStatus();
      };

      _proto.componentWillUnmount = function componentWillUnmount() {
        this.cleanup(); // avoids memory leak
      };

      _proto.render = function render() {
        var children = unwrapArray(this.props.children, noop); // because the items are rerendered every time we call the children
        // we clear this out each render and it will be populated again as
        // getItemProps is called.

        this.clearItems(); // we reset this so we know whether the user calls getRootProps during
        // this render. If they do then we don't need to do anything,
        // if they don't then we need to clone the element they return and
        // apply the props for them.

        this.getRootProps.called = false;
        this.getRootProps.refKey = undefined;
        this.getRootProps.suppressRefError = undefined; // we do something similar for getMenuProps

        this.getMenuProps.called = false;
        this.getMenuProps.refKey = undefined;
        this.getMenuProps.suppressRefError = undefined; // we do something similar for getLabelProps

        this.getLabelProps.called = false; // and something similar for getInputProps

        this.getInputProps.called = false;
        var element = unwrapArray(children(this.getStateAndHelpers()));

        if (!element) {
          return null;
        }

        if (this.getRootProps.called || this.props.suppressRefError) {
          if ( !this.getRootProps.suppressRefError && !this.props.suppressRefError) {
            validateGetRootPropsCalledCorrectly(element, this.getRootProps);
          }

          return element;
        } else if (isDOMElement(element)) {
          // they didn't apply the root props, but we can clone
          // this and apply the props ourselves
          return react.cloneElement(element, this.getRootProps(getElementProps(element)));
        }
        /* istanbul ignore else */


        // they didn't apply the root props, but they need to
        // otherwise we can't query around the autocomplete
        throw new Error('downshift: If you return a non-DOM element, you must apply the getRootProps function');
        /* istanbul ignore next */
      };

      return Downshift;
    }(react.Component);

    Downshift.defaultProps = {
      defaultHighlightedIndex: null,
      defaultIsOpen: false,
      getA11yStatusMessage: getA11yStatusMessage,
      itemToString: function itemToString(i) {
        if (i == null) {
          return '';
        }

        if ( isPlainObject(i) && !i.hasOwnProperty('toString')) {
          // eslint-disable-next-line no-console
          console.warn('downshift: An object was passed to the default implementation of `itemToString`. You should probably provide your own `itemToString` implementation. Please refer to the `itemToString` API documentation.', 'The object that was passed:', i);
        }

        return String(i);
      },
      onStateChange: noop,
      onInputValueChange: noop,
      onUserAction: noop,
      onChange: noop,
      onSelect: noop,
      onOuterClick: noop,
      selectedItemChanged: function selectedItemChanged(prevItem, item) {
        return prevItem !== item;
      },
      environment: typeof window === 'undefined'
      /* istanbul ignore next (ssr) */
      ? {} : window,
      stateReducer: function stateReducer(state, stateToSet) {
        return stateToSet;
      },
      suppressRefError: false,
      scrollIntoView: scrollIntoView
    };
    Downshift.stateChangeTypes = stateChangeTypes;
    return Downshift;
  }();

  Downshift.propTypes = {
    children: propTypes.func,
    defaultHighlightedIndex: propTypes.number,
    defaultIsOpen: propTypes.bool,
    initialHighlightedIndex: propTypes.number,
    initialSelectedItem: propTypes.any,
    initialInputValue: propTypes.string,
    initialIsOpen: propTypes.bool,
    getA11yStatusMessage: propTypes.func,
    itemToString: propTypes.func,
    onChange: propTypes.func,
    onSelect: propTypes.func,
    onStateChange: propTypes.func,
    onInputValueChange: propTypes.func,
    onUserAction: propTypes.func,
    onOuterClick: propTypes.func,
    selectedItemChanged: propTypes.func,
    stateReducer: propTypes.func,
    itemCount: propTypes.number,
    id: propTypes.string,
    environment: propTypes.shape({
      addEventListener: propTypes.func,
      removeEventListener: propTypes.func,
      document: propTypes.shape({
        getElementById: propTypes.func,
        activeElement: propTypes.any,
        body: propTypes.any
      })
    }),
    suppressRefError: propTypes.bool,
    scrollIntoView: propTypes.func,
    // things we keep in state for uncontrolled components
    // but can accept as props for controlled components

    /* eslint-disable react/no-unused-prop-types */
    selectedItem: propTypes.any,
    isOpen: propTypes.bool,
    inputValue: propTypes.string,
    highlightedIndex: propTypes.number,
    labelId: propTypes.string,
    inputId: propTypes.string,
    menuId: propTypes.string,
    getItemId: propTypes.func
    /* eslint-enable react/no-unused-prop-types */

  };

  function validateGetMenuPropsCalledCorrectly(node, _ref12) {
    var refKey = _ref12.refKey;

    if (!node) {
      // eslint-disable-next-line no-console
      console.error("downshift: The ref prop \"" + refKey + "\" from getMenuProps was not applied correctly on your menu element.");
    }
  }

  function validateGetRootPropsCalledCorrectly(element, _ref13) {
    var refKey = _ref13.refKey;
    var refKeySpecified = refKey !== 'ref';
    var isComposite = !isDOMElement(element);

    if (isComposite && !refKeySpecified && !reactIs_1(element)) {
      // eslint-disable-next-line no-console
      console.error('downshift: You returned a non-DOM element. You must specify a refKey in getRootProps');
    } else if (!isComposite && refKeySpecified) {
      // eslint-disable-next-line no-console
      console.error("downshift: You returned a DOM element. You should not specify a refKey in getRootProps. You specified \"" + refKey + "\"");
    }

    if (!reactIs_1(element) && !getElementProps(element)[refKey]) {
      // eslint-disable-next-line no-console
      console.error("downshift: You must apply the ref prop \"" + refKey + "\" from getRootProps onto your root element.");
    }
  }

  function validateControlledUnchanged(prevProps, nextProps) {
    var warningDescription = "This prop should not switch from controlled to uncontrolled (or vice versa). Decide between using a controlled or uncontrolled Downshift element for the lifetime of the component. More info: https://github.com/downshift-js/downshift#control-props";
    ['selectedItem', 'isOpen', 'inputValue', 'highlightedIndex'].forEach(function (propKey) {
      if (prevProps[propKey] !== undefined && nextProps[propKey] === undefined) {
        // eslint-disable-next-line no-console
        console.error("downshift: A component has changed the controlled prop \"" + propKey + "\" to be uncontrolled. " + warningDescription);
      } else if (prevProps[propKey] === undefined && nextProps[propKey] !== undefined) {
        // eslint-disable-next-line no-console
        console.error("downshift: A component has changed the uncontrolled prop \"" + propKey + "\" to be controlled. " + warningDescription);
      }
    });
  }

  function getElementIds(generateDefaultId, _temp) {
    var _ref = _temp === void 0 ? {} : _temp,
        id = _ref.id,
        labelId = _ref.labelId,
        menuId = _ref.menuId,
        getItemId = _ref.getItemId,
        toggleButtonId = _ref.toggleButtonId;

    var uniqueId = id === undefined ? "downshift-" + generateDefaultId() : id;
    return {
      labelId: labelId || uniqueId + "-label",
      menuId: menuId || uniqueId + "-menu",
      getItemId: getItemId || function (index) {
        return uniqueId + "-item-" + index;
      },
      toggleButtonId: toggleButtonId || uniqueId + "-toggle-button"
    };
  }

  function getNextWrappingIndex$1(moveAmount, baseIndex, itemsLength, circular) {
    if (baseIndex === -1) {
      return moveAmount > 0 ? 0 : itemsLength - 1;
    }

    var nextIndex = baseIndex + moveAmount;

    if (nextIndex < 0) {
      return circular ? itemsLength - 1 : 0;
    }

    if (nextIndex >= itemsLength) {
      return circular ? 0 : itemsLength - 1;
    }

    return nextIndex;
  }

  function getItemIndexByCharacterKey(keysSoFar, highlightedIndex, items, itemToStringParam) {
    var newHighlightedIndex = -1;
    var itemStrings = items.map(function (item) {
      return itemToStringParam(item).toLowerCase();
    });
    var startPosition = highlightedIndex + 1;
    newHighlightedIndex = itemStrings.slice(startPosition).findIndex(function (itemString) {
      return itemString.startsWith(keysSoFar);
    });

    if (newHighlightedIndex > -1) {
      return newHighlightedIndex + startPosition;
    } else {
      return itemStrings.slice(0, startPosition).findIndex(function (itemString) {
        return itemString.startsWith(keysSoFar);
      });
    }
  }

  function getState(state, props) {
    return Object.keys(state).reduce(function (prevState, key) {
      // eslint-disable-next-line no-param-reassign
      prevState[key] = key in props ? props[key] : state[key];
      return prevState;
    }, {});
  }

  function getItemIndex(index, item, items) {
    if (index !== undefined) {
      return index;
    }

    if (items.length === 0) {
      return -1;
    }

    return items.indexOf(item);
  }

  function itemToString(item) {
    return item ? String(item) : '';
  }

  function getPropTypesValidator(caller, propTypes$1) {
    // istanbul ignore next
    return function (options) {
      if (options === void 0) {
        options = {};
      }

      Object.entries(propTypes$1).forEach(function (_ref2) {
        var key = _ref2[0];
        propTypes.checkPropTypes(propTypes$1, options, key, caller.name);
      });
    };
  }

  function isAcceptedCharacterKey(key) {
    return /^\S{1}$/.test(key);
  }

  function capitalizeString(string) {
    return "" + string.slice(0, 1).toUpperCase() + string.slice(1);
  }

  function invokeOnChangeHandler(propKey, props, state, changes) {
    var handler = "on" + capitalizeString(propKey) + "Change";

    if (props[handler] && changes[propKey] !== undefined && changes[propKey] !== state[propKey]) {
      props[handler](changes);
    }
  }

  function callOnChangeProps(props, state, changes) {
    Object.keys(state).forEach(function (stateKey) {
      invokeOnChangeHandler(stateKey, props, state, changes);
    });

    if (props.onStateChange && changes !== undefined) {
      props.onStateChange(changes);
    }
  }

  function useEnhancedReducer(reducer, initialState, props) {
    var enhancedReducer = react.useCallback(function (state, action) {
      state = getState(state, action.props);
      var stateReducer = action.props.stateReducer;
      var changes = reducer(state, action);
      var newState = stateReducer(state, _extends({}, action, {
        changes: changes
      }));
      callOnChangeProps(action.props, state, newState);
      return newState;
    }, [reducer]);

    var _useReducer = react.useReducer(enhancedReducer, initialState),
        state = _useReducer[0],
        dispatch = _useReducer[1];

    return [getState(state, props), dispatch];
  }

  var lastId = 0; // istanbul ignore next

  var genId = function () {
    return ++lastId;
  };
  /**
   * Autogenerate IDs to facilitate WAI-ARIA and server rendering.
   * Taken from @reach/auto-id
   * @see https://github.com/reach/reach-ui/blob/6e9dbcf716d5c9a3420e062e5bac1ac4671d01cb/packages/auto-id/src/index.js
   */
  // istanbul ignore next


  function useId() {
    var _useState = react.useState(null),
        id = _useState[0],
        setId = _useState[1];

    react.useEffect(function () {
      return setId(genId());
    }, []);
    return id;
  }
  /**
   * Checks if nextElement receives focus after the blur event.
   *
   * @param {FocusEvent} event The blur event.
   * @param {Element} nextElement The element to check that receive focus next.
   * @returns {boolean} If the focus lands on nextElement.
   */


  function focusLandsOnElement(event, nextElement) {
    return event.relatedTarget === nextElement || // https://github.com/downshift-js/downshift/issues/832 - workaround for Firefox.
    event.nativeEvent && (nextElement === event.nativeEvent.explicitOriginalTarget || nextElement.contains(event.nativeEvent.explicitOriginalTarget));
  }

  var defaultStateValues = {
    highlightedIndex: -1,
    isOpen: false,
    selectedItem: null
  };

  function getA11yStatusMessage$1(_ref) {
    var isOpen = _ref.isOpen,
        items = _ref.items;

    if (!items) {
      return '';
    }

    var resultCount = items.length;

    if (isOpen) {
      if (resultCount === 0) {
        return 'No results are available';
      }

      return resultCount + " result" + (resultCount === 1 ? ' is' : 's are') + " available, use up and down arrow keys to navigate. Press Enter key to select.";
    }

    return '';
  }

  function getA11ySelectionMessage(_ref2) {
    var selectedItem = _ref2.selectedItem,
        itemToString = _ref2.itemToString;
    return itemToString(selectedItem) + " has been selected.";
  }

  function getHighlightedIndexOnOpen(props, state, offset) {
    var items = props.items,
        initialHighlightedIndex = props.initialHighlightedIndex,
        defaultHighlightedIndex = props.defaultHighlightedIndex;
    var selectedItem = state.selectedItem,
        highlightedIndex = state.highlightedIndex; // initialHighlightedIndex will give value to highlightedIndex on initial state only.

    if (initialHighlightedIndex !== undefined && highlightedIndex > -1) {
      return initialHighlightedIndex;
    }

    if (defaultHighlightedIndex !== undefined) {
      return defaultHighlightedIndex;
    }

    if (selectedItem) {
      if (offset === 0) {
        return items.indexOf(selectedItem);
      }

      return getNextWrappingIndex$1(offset, items.indexOf(selectedItem), items.length, false);
    }

    if (offset === 0) {
      return -1;
    }

    return offset < 0 ? items.length - 1 : 0;
  }

  function getDefaultValue(props, propKey) {
    var defaultPropKey = "default" + capitalizeString(propKey);

    if (defaultPropKey in props) {
      return props[defaultPropKey];
    }

    return defaultStateValues[propKey];
  }

  function getInitialValue(props, propKey) {
    if (propKey in props) {
      return props[propKey];
    }

    var initialPropKey = "initial" + capitalizeString(propKey);

    if (initialPropKey in props) {
      return props[initialPropKey];
    }

    return getDefaultValue(props, propKey);
  }

  function getInitialState(props) {
    var selectedItem = getInitialValue(props, 'selectedItem');
    var highlightedIndex = getInitialValue(props, 'highlightedIndex');
    var isOpen = getInitialValue(props, 'isOpen');
    return {
      highlightedIndex: highlightedIndex < 0 && selectedItem ? props.items.indexOf(selectedItem) : highlightedIndex,
      isOpen: isOpen,
      selectedItem: selectedItem,
      keysSoFar: ''
    };
  }

  var propTypes$1 = {
    items: propTypes.array.isRequired,
    itemToString: propTypes.func,
    getA11yStatusMessage: propTypes.func,
    getA11ySelectionMessage: propTypes.func,
    circularNavigation: propTypes.bool,
    highlightedIndex: propTypes.number,
    defaultHighlightedIndex: propTypes.number,
    initialHighlightedIndex: propTypes.number,
    isOpen: propTypes.bool,
    defaultIsOpen: propTypes.bool,
    initialIsOpen: propTypes.bool,
    selectedItem: propTypes.any,
    initialSelectedItem: propTypes.any,
    defaultSelectedItem: propTypes.any,
    id: propTypes.string,
    labelId: propTypes.string,
    menuId: propTypes.string,
    getItemId: propTypes.func,
    toggleButtonId: propTypes.string,
    stateReducer: propTypes.func,
    onSelectedItemChange: propTypes.func,
    onHighlightedIndexChange: propTypes.func,
    onStateChange: propTypes.func,
    onIsOpenChange: propTypes.func,
    environment: propTypes.shape({
      addEventListener: propTypes.func,
      removeEventListener: propTypes.func,
      document: propTypes.shape({
        getElementById: propTypes.func,
        activeElement: propTypes.any,
        body: propTypes.any
      })
    })
  };

  var MenuKeyDownArrowDown = '__menu_keydown_arrow_down__';
  var MenuKeyDownArrowUp = '__menu_keydown_arrow_up__';
  var MenuKeyDownEscape = '__menu_keydown_escape__';
  var MenuKeyDownHome = '__menu_keydown_home__';
  var MenuKeyDownEnd = '__menu_keydown_end__';
  var MenuKeyDownEnter = '__menu_keydown_enter__';
  var MenuKeyDownCharacter = '__menu_keydown_character__';
  var MenuBlur = '__menu_blur__';
  var MenuMouseLeave = '__menu_mouse_leave__';
  var ItemMouseMove = '__item_mouse_move__';
  var ItemClick = '__item_click__';
  var ToggleButtonKeyDownCharacter = '__togglebutton_keydown_character__';
  var ToggleButtonKeyDownArrowDown = '__togglebutton_keydown_arrow_down__';
  var ToggleButtonKeyDownArrowUp = '__togglebutton_keydown_arrow_up__';
  var ToggleButtonClick = '__togglebutton_click__';
  var FunctionToggleMenu = '__function_toggle_menu__';
  var FunctionOpenMenu = '__function_open_menu__';
  var FunctionCloseMenu = '__function_close_menu__';
  var FunctionSetHighlightedIndex = '__function_set_highlighted_index__';
  var FunctionSelectItem = '__function_select_item__';
  var FunctionClearKeysSoFar = '__function_clear_keys_so_far__';
  var FunctionReset = '__function_reset__';

  var stateChangeTypes$1 = /*#__PURE__*/Object.freeze({
    __proto__: null,
    MenuKeyDownArrowDown: MenuKeyDownArrowDown,
    MenuKeyDownArrowUp: MenuKeyDownArrowUp,
    MenuKeyDownEscape: MenuKeyDownEscape,
    MenuKeyDownHome: MenuKeyDownHome,
    MenuKeyDownEnd: MenuKeyDownEnd,
    MenuKeyDownEnter: MenuKeyDownEnter,
    MenuKeyDownCharacter: MenuKeyDownCharacter,
    MenuBlur: MenuBlur,
    MenuMouseLeave: MenuMouseLeave,
    ItemMouseMove: ItemMouseMove,
    ItemClick: ItemClick,
    ToggleButtonKeyDownCharacter: ToggleButtonKeyDownCharacter,
    ToggleButtonKeyDownArrowDown: ToggleButtonKeyDownArrowDown,
    ToggleButtonKeyDownArrowUp: ToggleButtonKeyDownArrowUp,
    ToggleButtonClick: ToggleButtonClick,
    FunctionToggleMenu: FunctionToggleMenu,
    FunctionOpenMenu: FunctionOpenMenu,
    FunctionCloseMenu: FunctionCloseMenu,
    FunctionSetHighlightedIndex: FunctionSetHighlightedIndex,
    FunctionSelectItem: FunctionSelectItem,
    FunctionClearKeysSoFar: FunctionClearKeysSoFar,
    FunctionReset: FunctionReset
  });

  /* eslint-disable complexity */

  function downshiftSelectReducer(state, action) {
    var type = action.type,
        props = action.props,
        shiftKey = action.shiftKey;
    var changes;

    switch (type) {
      case ItemMouseMove:
        changes = {
          highlightedIndex: action.index
        };
        break;

      case ItemClick:
        changes = {
          isOpen: getDefaultValue(props, 'isOpen'),
          highlightedIndex: getDefaultValue(props, 'highlightedIndex'),
          selectedItem: props.items[action.index]
        };
        break;

      case MenuBlur:
        changes = {
          isOpen: false,
          highlightedIndex: -1
        };
        break;

      case MenuKeyDownArrowDown:
        changes = {
          highlightedIndex: getNextWrappingIndex$1(shiftKey ? 5 : 1, state.highlightedIndex, props.items.length, props.circularNavigation)
        };
        break;

      case MenuKeyDownArrowUp:
        changes = {
          highlightedIndex: getNextWrappingIndex$1(shiftKey ? -5 : -1, state.highlightedIndex, props.items.length, props.circularNavigation)
        };
        break;

      case MenuKeyDownHome:
        changes = {
          highlightedIndex: 0
        };
        break;

      case MenuKeyDownEnd:
        changes = {
          highlightedIndex: props.items.length - 1
        };
        break;

      case MenuKeyDownEscape:
        changes = {
          isOpen: false,
          highlightedIndex: -1
        };
        break;

      case MenuKeyDownEnter:
        changes = _extends({
          isOpen: getDefaultValue(props, 'isOpen'),
          highlightedIndex: getDefaultValue(props, 'highlightedIndex')
        }, state.highlightedIndex >= 0 && {
          selectedItem: props.items[state.highlightedIndex]
        });
        break;

      case MenuKeyDownCharacter:
        {
          var lowercasedKey = action.key;
          var keysSoFar = "" + state.keysSoFar + lowercasedKey;
          var highlightedIndex = getItemIndexByCharacterKey(keysSoFar, state.highlightedIndex, props.items, props.itemToString);
          changes = _extends({
            keysSoFar: keysSoFar
          }, highlightedIndex >= 0 && {
            highlightedIndex: highlightedIndex
          });
        }
        break;

      case MenuMouseLeave:
        changes = {
          highlightedIndex: -1
        };
        break;

      case ToggleButtonKeyDownCharacter:
        {
          var _lowercasedKey = action.key;

          var _keysSoFar = "" + state.keysSoFar + _lowercasedKey;

          var itemIndex = getItemIndexByCharacterKey(_keysSoFar, state.selectedItem ? props.items.indexOf(state.selectedItem) : -1, props.items, props.itemToString);
          changes = _extends({
            keysSoFar: _keysSoFar
          }, itemIndex >= 0 && {
            selectedItem: props.items[itemIndex]
          });
        }
        break;

      case ToggleButtonKeyDownArrowDown:
        {
          changes = {
            isOpen: true,
            highlightedIndex: getHighlightedIndexOnOpen(props, state, 1)
          };
          break;
        }

      case ToggleButtonKeyDownArrowUp:
        changes = {
          isOpen: true,
          highlightedIndex: getHighlightedIndexOnOpen(props, state, -1)
        };
        break;

      case ToggleButtonClick:
      case FunctionToggleMenu:
        changes = {
          isOpen: !state.isOpen,
          highlightedIndex: state.isOpen ? -1 : getHighlightedIndexOnOpen(props, state, 0)
        };
        break;

      case FunctionOpenMenu:
        changes = {
          isOpen: true,
          highlightedIndex: getHighlightedIndexOnOpen(props, state, 0)
        };
        break;

      case FunctionCloseMenu:
        changes = {
          isOpen: false
        };
        break;

      case FunctionSetHighlightedIndex:
        changes = {
          highlightedIndex: action.highlightedIndex
        };
        break;

      case FunctionSelectItem:
        changes = {
          selectedItem: action.selectedItem
        };
        break;

      case FunctionClearKeysSoFar:
        changes = {
          keysSoFar: ''
        };
        break;

      case FunctionReset:
        changes = {
          highlightedIndex: getDefaultValue(props, 'highlightedIndex'),
          isOpen: getDefaultValue(props, 'isOpen'),
          selectedItem: getDefaultValue(props, 'selectedItem')
        };
        break;

      default:
        throw new Error('Reducer called without proper action type.');
    }

    return _extends({}, state, {}, changes);
  }
  /* eslint-enable complexity */

  var validatePropTypes = getPropTypesValidator(useSelect, propTypes$1);
  var defaultProps = {
    itemToString: itemToString,
    stateReducer: function stateReducer(s, a) {
      return a.changes;
    },
    getA11yStatusMessage: getA11yStatusMessage$1,
    getA11ySelectionMessage: getA11ySelectionMessage,
    scrollIntoView: scrollIntoView,
    environment: typeof window === 'undefined'
    /* istanbul ignore next (ssr) */
    ? {} : window
  };
  useSelect.stateChangeTypes = stateChangeTypes$1;

  function useSelect(userProps) {
    if (userProps === void 0) {
      userProps = {};
    }

    /* istanbul ignore else */
    validatePropTypes(userProps); // Props defaults and destructuring.

    var props = _extends({}, defaultProps, {}, userProps);

    var items = props.items,
        itemToString = props.itemToString,
        getA11yStatusMessage = props.getA11yStatusMessage,
        getA11ySelectionMessage = props.getA11ySelectionMessage,
        initialIsOpen = props.initialIsOpen,
        defaultIsOpen = props.defaultIsOpen,
        scrollIntoView = props.scrollIntoView,
        environment = props.environment; // Initial state depending on controlled props.

    var initialState = getInitialState(props); // Reducer init.

    var _useEnhancedReducer = useEnhancedReducer(downshiftSelectReducer, initialState, props),
        _useEnhancedReducer$ = _useEnhancedReducer[0],
        isOpen = _useEnhancedReducer$.isOpen,
        highlightedIndex = _useEnhancedReducer$.highlightedIndex,
        selectedItem = _useEnhancedReducer$.selectedItem,
        keysSoFar = _useEnhancedReducer$.keysSoFar,
        dispatchWithoutProps = _useEnhancedReducer[1];

    var dispatch = function (action) {
      return dispatchWithoutProps(_extends({
        props: props
      }, action));
    }; // IDs generation.


    var _getElementIds = getElementIds(useId, props),
        labelId = _getElementIds.labelId,
        getItemId = _getElementIds.getItemId,
        menuId = _getElementIds.menuId,
        toggleButtonId = _getElementIds.toggleButtonId;
    /* Refs */


    var toggleButtonRef = react.useRef(null);
    var menuRef = react.useRef(null);
    var itemRefs = react.useRef();
    itemRefs.current = [];
    var isInitialMount = react.useRef(true);
    var shouldScroll = react.useRef(true);
    var clearTimeout = react.useRef(null);
    /* Effects */

    /* Sets a11y status message on changes in isOpen. */

    react.useEffect(function () {
      if (isInitialMount.current) {
        return;
      }

      setStatus(getA11yStatusMessage({
        isOpen: isOpen,
        items: items,
        selectedItem: selectedItem,
        itemToString: itemToString
      }), environment.document); // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [isOpen]);
    /* Sets a11y status message on changes in selectedItem. */

    react.useEffect(function () {
      if (isInitialMount.current) {
        return;
      }

      setStatus(getA11ySelectionMessage({
        isOpen: isOpen,
        items: items,
        selectedItem: selectedItem,
        itemToString: itemToString
      }), environment.document); // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [selectedItem]);
    /* Sets cleanup for the keysSoFar after 500ms. */

    react.useEffect(function () {
      // init the clean function here as we need access to dispatch.
      if (isInitialMount.current) {
        clearTimeout.current = debounce(function () {
          dispatch({
            type: FunctionClearKeysSoFar
          });
        }, 500);
      }

      if (!keysSoFar) {
        return;
      }

      clearTimeout.current(); // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [keysSoFar]);
    /* Controls the focus on the menu or the toggle button. */

    react.useEffect(function () {
      // Don't focus menu on first render.
      if (isInitialMount.current) {
        // Unless it was initialised as open.
        if (initialIsOpen || defaultIsOpen || isOpen) {
          menuRef.current.focus();
        }

        return;
      } // Focus menu on open.
      // istanbul ignore next


      if (isOpen) {
        menuRef.current.focus(); // Focus toggleButton on close.
      } else if (environment.document.activeElement === menuRef.current) {
        toggleButtonRef.current.focus();
      } // eslint-disable-next-line react-hooks/exhaustive-deps

    }, [isOpen]);
    /* Scroll on highlighted item if change comes from keyboard. */

    react.useEffect(function () {
      if (highlightedIndex < 0 || !isOpen || !itemRefs.current.length) {
        return;
      }

      if (shouldScroll.current === false) {
        shouldScroll.current = true;
      } else {
        scrollIntoView(itemRefs.current[highlightedIndex], menuRef.current);
      } // eslint-disable-next-line react-hooks/exhaustive-deps

    }, [highlightedIndex]);
    /* Make initial ref false. */

    react.useEffect(function () {
      isInitialMount.current = false;
    }, []);
    /* Event handler functions */

    var menuKeyDownHandlers = {
      ArrowDown: function ArrowDown(event) {
        event.preventDefault();
        dispatch({
          type: MenuKeyDownArrowDown,
          shiftKey: event.shiftKey
        });
      },
      ArrowUp: function ArrowUp(event) {
        event.preventDefault();
        dispatch({
          type: MenuKeyDownArrowUp,
          shiftKey: event.shiftKey
        });
      },
      Home: function Home(event) {
        event.preventDefault();
        dispatch({
          type: MenuKeyDownHome
        });
      },
      End: function End(event) {
        event.preventDefault();
        dispatch({
          type: MenuKeyDownEnd
        });
      },
      Escape: function Escape() {
        dispatch({
          type: MenuKeyDownEscape
        });
      },
      Enter: function Enter(event) {
        event.preventDefault();
        dispatch({
          type: MenuKeyDownEnter
        });
      },
      Tab: function Tab(event) {
        // The exception that calls MenuBlur.
        // istanbul ignore next
        if (event.shiftKey) {
          dispatch({
            type: MenuBlur
          });
        }
      }
    };
    var toggleButtonKeyDownHandlers = {
      ArrowDown: function ArrowDown(event) {
        event.preventDefault();
        dispatch({
          type: ToggleButtonKeyDownArrowDown
        });
      },
      ArrowUp: function ArrowUp(event) {
        event.preventDefault();
        dispatch({
          type: ToggleButtonKeyDownArrowUp
        });
      }
    }; // Event handlers.

    var menuHandleKeyDown = function (event) {
      var key = normalizeArrowKey(event);

      if (key && menuKeyDownHandlers[key]) {
        menuKeyDownHandlers[key](event);
      } else if (isAcceptedCharacterKey(key)) {
        dispatch({
          type: MenuKeyDownCharacter,
          key: key
        });
      }
    }; // Focus going back to the toggleButton is something we control (Escape, Enter, Click).
    // We are toggleing special actions for these cases in reducer, not MenuBlur.
    // Since Shift-Tab also lands focus on toggleButton, we will handle it as exception and call MenuBlur.


    var menuHandleBlur = function (event) {
      if (!focusLandsOnElement(event, toggleButtonRef.current)) {
        dispatch({
          type: MenuBlur
        });
      }
    };

    var menuHandleMouseLeave = function () {
      dispatch({
        type: MenuMouseLeave
      });
    };

    var toggleButtonHandleClick = function () {
      dispatch({
        type: ToggleButtonClick
      });
    };

    var toggleButtonHandleKeyDown = function (event) {
      var key = normalizeArrowKey(event);

      if (key && toggleButtonKeyDownHandlers[key]) {
        toggleButtonKeyDownHandlers[key](event);
      } else if (isAcceptedCharacterKey(key)) {
        dispatch({
          type: ToggleButtonKeyDownCharacter,
          key: key
        });
      }
    };

    var itemHandleMouseMove = function (index) {
      if (index === highlightedIndex) {
        return;
      }

      shouldScroll.current = false;
      dispatch({
        type: ItemMouseMove,
        index: index
      });
    };

    var itemHandleClick = function (index) {
      dispatch({
        type: ItemClick,
        index: index
      });
    }; // returns


    return {
      // prop getters.
      getToggleButtonProps: function getToggleButtonProps(_temp2) {
        var _extends3;

        var _ref2 = _temp2 === void 0 ? {} : _temp2,
            onClick = _ref2.onClick,
            onKeyDown = _ref2.onKeyDown,
            _ref2$refKey = _ref2.refKey,
            refKey = _ref2$refKey === void 0 ? 'ref' : _ref2$refKey,
            ref = _ref2.ref,
            rest = _objectWithoutPropertiesLoose(_ref2, ["onClick", "onKeyDown", "refKey", "ref"]);

        var toggleProps = _extends((_extends3 = {}, _extends3[refKey] = handleRefs(ref, function (toggleButtonNode) {
          toggleButtonRef.current = toggleButtonNode;
        }), _extends3.id = toggleButtonId, _extends3['aria-haspopup'] = 'listbox', _extends3['aria-expanded'] = isOpen, _extends3['aria-labelledby'] = labelId + " " + toggleButtonId, _extends3), rest);

        if (!rest.disabled) {
          toggleProps.onClick = callAllEventHandlers(onClick, toggleButtonHandleClick);
          toggleProps.onKeyDown = callAllEventHandlers(onKeyDown, toggleButtonHandleKeyDown);
        }

        return toggleProps;
      },
      getLabelProps: function getLabelProps(labelProps) {
        return _extends({
          id: labelId,
          htmlFor: toggleButtonId
        }, labelProps);
      },
      getMenuProps: function getMenuProps(_temp) {
        var _extends2;

        var _ref = _temp === void 0 ? {} : _temp,
            onKeyDown = _ref.onKeyDown,
            onBlur = _ref.onBlur,
            onMouseLeave = _ref.onMouseLeave,
            _ref$refKey = _ref.refKey,
            refKey = _ref$refKey === void 0 ? 'ref' : _ref$refKey,
            ref = _ref.ref,
            rest = _objectWithoutPropertiesLoose(_ref, ["onKeyDown", "onBlur", "onMouseLeave", "refKey", "ref"]);

        return _extends((_extends2 = {}, _extends2[refKey] = handleRefs(ref, function (menuNode) {
          menuRef.current = menuNode;
        }), _extends2.id = menuId, _extends2.role = 'listbox', _extends2['aria-labelledby'] = labelId, _extends2.tabIndex = -1, _extends2), highlightedIndex > -1 && {
          'aria-activedescendant': getItemId(highlightedIndex)
        }, {
          onKeyDown: callAllEventHandlers(onKeyDown, menuHandleKeyDown),
          onBlur: callAllEventHandlers(onBlur, menuHandleBlur),
          onMouseLeave: callAllEventHandlers(onMouseLeave, menuHandleMouseLeave)
        }, rest);
      },
      getItemProps: function getItemProps(_temp3) {
        var _extends4;

        var _ref3 = _temp3 === void 0 ? {} : _temp3,
            item = _ref3.item,
            index = _ref3.index,
            _ref3$refKey = _ref3.refKey,
            refKey = _ref3$refKey === void 0 ? 'ref' : _ref3$refKey,
            ref = _ref3.ref,
            onMouseMove = _ref3.onMouseMove,
            onClick = _ref3.onClick,
            rest = _objectWithoutPropertiesLoose(_ref3, ["item", "index", "refKey", "ref", "onMouseMove", "onClick"]);

        var itemIndex = getItemIndex(index, item, items);

        if (itemIndex < 0) {
          throw new Error('Pass either item or item index in getItemProps!');
        }

        var itemProps = _extends((_extends4 = {}, _extends4[refKey] = handleRefs(ref, function (itemNode) {
          if (itemNode) {
            itemRefs.current.push(itemNode);
          }
        }), _extends4.role = 'option', _extends4), itemIndex === highlightedIndex && {
          'aria-selected': true
        }, {
          id: getItemId(itemIndex)
        }, rest);

        if (!rest.disabled) {
          itemProps.onMouseMove = callAllEventHandlers(onMouseMove, function () {
            return itemHandleMouseMove(itemIndex);
          });
          itemProps.onClick = callAllEventHandlers(onClick, function () {
            return itemHandleClick(itemIndex);
          });
        }

        return itemProps;
      },
      // actions.
      toggleMenu: function toggleMenu() {
        dispatch({
          type: FunctionToggleMenu
        });
      },
      openMenu: function openMenu() {
        dispatch({
          type: FunctionOpenMenu
        });
      },
      closeMenu: function closeMenu() {
        dispatch({
          type: FunctionCloseMenu
        });
      },
      setHighlightedIndex: function setHighlightedIndex(newHighlightedIndex) {
        dispatch({
          type: FunctionSetHighlightedIndex,
          highlightedIndex: newHighlightedIndex
        });
      },
      selectItem: function selectItem(newSelectedItem) {
        dispatch({
          type: FunctionSelectItem,
          selectedItem: newSelectedItem
        });
      },
      reset: function reset() {
        dispatch({
          type: FunctionReset
        });
      },
      // state.
      highlightedIndex: highlightedIndex,
      isOpen: isOpen,
      selectedItem: selectedItem
    };
  }

  exports.default = Downshift;
  exports.resetIdCounter = resetIdCounter;
  exports.useSelect = useSelect;

  Object.defineProperty(exports, '__esModule', { value: true });

})));
//# sourceMappingURL=downshift.umd.js.map
