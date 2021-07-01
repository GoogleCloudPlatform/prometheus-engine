import _objectWithoutPropertiesLoose from '@babel/runtime/helpers/esm/objectWithoutPropertiesLoose';
import _extends from '@babel/runtime/helpers/esm/extends';
import _assertThisInitialized from '@babel/runtime/helpers/esm/assertThisInitialized';
import _inheritsLoose from '@babel/runtime/helpers/esm/inheritsLoose';
import { cloneElement, Component, useCallback, useReducer, useState, useEffect, useRef } from 'preact';
import { isForwardRef } from 'react-is';
import computeScrollIntoView from 'compute-scroll-into-view';
import PropTypes from 'prop-types';

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
  // then this is preact or preact X

  /* istanbul ignore if */
  return typeof element.nodeName === 'string' || typeof element.type === 'string'; // then we assume this is react
}
/**
 * @param {Object} element (P)react element
 * @return {Object} the props
 */


function getElementProps(element) {
  // props for react, attributes for preact

  /* istanbul ignore if */
  return element.attributes || element.props;
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

var unknown = process.env.NODE_ENV !== "production" ? '__autocomplete_unknown__' : 0;
var mouseUp = process.env.NODE_ENV !== "production" ? '__autocomplete_mouseup__' : 1;
var itemMouseEnter = process.env.NODE_ENV !== "production" ? '__autocomplete_item_mouseenter__' : 2;
var keyDownArrowUp = process.env.NODE_ENV !== "production" ? '__autocomplete_keydown_arrow_up__' : 3;
var keyDownArrowDown = process.env.NODE_ENV !== "production" ? '__autocomplete_keydown_arrow_down__' : 4;
var keyDownEscape = process.env.NODE_ENV !== "production" ? '__autocomplete_keydown_escape__' : 5;
var keyDownEnter = process.env.NODE_ENV !== "production" ? '__autocomplete_keydown_enter__' : 6;
var keyDownHome = process.env.NODE_ENV !== "production" ? '__autocomplete_keydown_home__' : 7;
var keyDownEnd = process.env.NODE_ENV !== "production" ? '__autocomplete_keydown_end__' : 8;
var clickItem = process.env.NODE_ENV !== "production" ? '__autocomplete_click_item__' : 9;
var blurInput = process.env.NODE_ENV !== "production" ? '__autocomplete_blur_input__' : 10;
var changeInput = process.env.NODE_ENV !== "production" ? '__autocomplete_change_input__' : 11;
var keyDownSpaceButton = process.env.NODE_ENV !== "production" ? '__autocomplete_keydown_space_button__' : 12;
var clickButton = process.env.NODE_ENV !== "production" ? '__autocomplete_click_button__' : 13;
var blurButton = process.env.NODE_ENV !== "production" ? '__autocomplete_blur_button__' : 14;
var controlledPropUpdatedSelectedItem = process.env.NODE_ENV !== "production" ? '__autocomplete_controlled_prop_updated_selected_item__' : 15;
var touchEnd = process.env.NODE_ENV !== "production" ? '__autocomplete_touchend__' : 16;

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


        if (process.env.NODE_ENV === 'test') {
          _this.toggleMenu({
            type: clickButton
          });
        } else {
          // Ensure that toggle of menu occurs after the potential blur event in iOS
          _this.internalSetTimeout(function () {
            return _this.toggleMenu({
              type: clickButton
            });
          });
        }
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

        onChangeKey = 'onInput';

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
            item = _ref7$item === void 0 ? process.env.NODE_ENV === 'production' ?
        /* istanbul ignore next */
        undefined : requiredProp('getItemProps', 'item') : _ref7$item,
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
      if (process.env.NODE_ENV !== 'production' && !false && this.getMenuProps.called && !this.getMenuProps.suppressRefError) {
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
      if (process.env.NODE_ENV !== 'production') {
        validateControlledUnchanged(prevProps, this.props);
        /* istanbul ignore if (react-native) */

        if ( this.getMenuProps.called && !this.getMenuProps.suppressRefError) {
          validateGetMenuPropsCalledCorrectly(this._menuNode, this.getMenuProps);
        }
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
        if (process.env.NODE_ENV !== 'production' && !this.getRootProps.suppressRefError && !this.props.suppressRefError) {
          validateGetRootPropsCalledCorrectly(element, this.getRootProps);
        }

        return element;
      } else if (isDOMElement(element)) {
        // they didn't apply the root props, but we can clone
        // this and apply the props ourselves
        return cloneElement(element, this.getRootProps(getElementProps(element)));
      }
      /* istanbul ignore else */


      if (process.env.NODE_ENV !== 'production') {
        // they didn't apply the root props, but they need to
        // otherwise we can't query around the autocomplete
        throw new Error('downshift: If you return a non-DOM element, you must apply the getRootProps function');
      }
      /* istanbul ignore next */


      return undefined;
    };

    return Downshift;
  }(Component);

  Downshift.defaultProps = {
    defaultHighlightedIndex: null,
    defaultIsOpen: false,
    getA11yStatusMessage: getA11yStatusMessage,
    itemToString: function itemToString(i) {
      if (i == null) {
        return '';
      }

      if (process.env.NODE_ENV !== 'production' && isPlainObject(i) && !i.hasOwnProperty('toString')) {
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

  if (isComposite && !refKeySpecified && !isForwardRef(element)) {
    // eslint-disable-next-line no-console
    console.error('downshift: You returned a non-DOM element. You must specify a refKey in getRootProps');
  } else if (!isComposite && refKeySpecified) {
    // eslint-disable-next-line no-console
    console.error("downshift: You returned a DOM element. You should not specify a refKey in getRootProps. You specified \"" + refKey + "\"");
  }

  if (!isForwardRef(element) && !getElementProps(element)[refKey]) {
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

function getPropTypesValidator(caller, propTypes) {
  // istanbul ignore next
  return function (options) {
    if (options === void 0) {
      options = {};
    }

    Object.entries(propTypes).forEach(function (_ref2) {
      var key = _ref2[0];
      PropTypes.checkPropTypes(propTypes, options, key, caller.name);
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
  var enhancedReducer = useCallback(function (state, action) {
    state = getState(state, action.props);
    var stateReducer = action.props.stateReducer;
    var changes = reducer(state, action);
    var newState = stateReducer(state, _extends({}, action, {
      changes: changes
    }));
    callOnChangeProps(action.props, state, newState);
    return newState;
  }, [reducer]);

  var _useReducer = useReducer(enhancedReducer, initialState),
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
  var _useState = useState(null),
      id = _useState[0],
      setId = _useState[1];

  useEffect(function () {
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

var propTypes = {
  items: PropTypes.array.isRequired,
  itemToString: PropTypes.func,
  getA11yStatusMessage: PropTypes.func,
  getA11ySelectionMessage: PropTypes.func,
  circularNavigation: PropTypes.bool,
  highlightedIndex: PropTypes.number,
  defaultHighlightedIndex: PropTypes.number,
  initialHighlightedIndex: PropTypes.number,
  isOpen: PropTypes.bool,
  defaultIsOpen: PropTypes.bool,
  initialIsOpen: PropTypes.bool,
  selectedItem: PropTypes.any,
  initialSelectedItem: PropTypes.any,
  defaultSelectedItem: PropTypes.any,
  id: PropTypes.string,
  labelId: PropTypes.string,
  menuId: PropTypes.string,
  getItemId: PropTypes.func,
  toggleButtonId: PropTypes.string,
  stateReducer: PropTypes.func,
  onSelectedItemChange: PropTypes.func,
  onHighlightedIndexChange: PropTypes.func,
  onStateChange: PropTypes.func,
  onIsOpenChange: PropTypes.func,
  environment: PropTypes.shape({
    addEventListener: PropTypes.func,
    removeEventListener: PropTypes.func,
    document: PropTypes.shape({
      getElementById: PropTypes.func,
      activeElement: PropTypes.any,
      body: PropTypes.any
    })
  })
};

var MenuKeyDownArrowDown = process.env.NODE_ENV !== "production" ? '__menu_keydown_arrow_down__' : 0;
var MenuKeyDownArrowUp = process.env.NODE_ENV !== "production" ? '__menu_keydown_arrow_up__' : 1;
var MenuKeyDownEscape = process.env.NODE_ENV !== "production" ? '__menu_keydown_escape__' : 2;
var MenuKeyDownHome = process.env.NODE_ENV !== "production" ? '__menu_keydown_home__' : 3;
var MenuKeyDownEnd = process.env.NODE_ENV !== "production" ? '__menu_keydown_end__' : 4;
var MenuKeyDownEnter = process.env.NODE_ENV !== "production" ? '__menu_keydown_enter__' : 5;
var MenuKeyDownCharacter = process.env.NODE_ENV !== "production" ? '__menu_keydown_character__' : 6;
var MenuBlur = process.env.NODE_ENV !== "production" ? '__menu_blur__' : 7;
var MenuMouseLeave = process.env.NODE_ENV !== "production" ? '__menu_mouse_leave__' : 8;
var ItemMouseMove = process.env.NODE_ENV !== "production" ? '__item_mouse_move__' : 9;
var ItemClick = process.env.NODE_ENV !== "production" ? '__item_click__' : 10;
var ToggleButtonKeyDownCharacter = process.env.NODE_ENV !== "production" ? '__togglebutton_keydown_character__' : 11;
var ToggleButtonKeyDownArrowDown = process.env.NODE_ENV !== "production" ? '__togglebutton_keydown_arrow_down__' : 12;
var ToggleButtonKeyDownArrowUp = process.env.NODE_ENV !== "production" ? '__togglebutton_keydown_arrow_up__' : 13;
var ToggleButtonClick = process.env.NODE_ENV !== "production" ? '__togglebutton_click__' : 14;
var FunctionToggleMenu = process.env.NODE_ENV !== "production" ? '__function_toggle_menu__' : 15;
var FunctionOpenMenu = process.env.NODE_ENV !== "production" ? '__function_open_menu__' : 16;
var FunctionCloseMenu = process.env.NODE_ENV !== "production" ? '__function_close_menu__' : 17;
var FunctionSetHighlightedIndex = process.env.NODE_ENV !== "production" ? '__function_set_highlighted_index__' : 18;
var FunctionSelectItem = process.env.NODE_ENV !== "production" ? '__function_select_item__' : 19;
var FunctionClearKeysSoFar = process.env.NODE_ENV !== "production" ? '__function_clear_keys_so_far__' : 20;
var FunctionReset = process.env.NODE_ENV !== "production" ? '__function_reset__' : 21;

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

var validatePropTypes = process.env.NODE_ENV === 'production' ?
/* istanbul ignore next */
null : getPropTypesValidator(useSelect, propTypes);
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
  if (process.env.NODE_ENV !== 'production') {
    validatePropTypes(userProps);
  } // Props defaults and destructuring.


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


  var toggleButtonRef = useRef(null);
  var menuRef = useRef(null);
  var itemRefs = useRef();
  itemRefs.current = [];
  var isInitialMount = useRef(true);
  var shouldScroll = useRef(true);
  var clearTimeout = useRef(null);
  /* Effects */

  /* Sets a11y status message on changes in isOpen. */

  useEffect(function () {
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

  useEffect(function () {
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

  useEffect(function () {
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

  useEffect(function () {
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

  useEffect(function () {
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

  useEffect(function () {
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

export default Downshift;
export { resetIdCounter, useSelect };
