"use strict";

var _interopRequireDefault = require("@babel/runtime/helpers/interopRequireDefault");

exports.__esModule = true;
exports.default = void 0;

var _extends2 = _interopRequireDefault(require("@babel/runtime/helpers/extends"));

var _defineProperty2 = _interopRequireDefault(require("@babel/runtime/helpers/defineProperty"));

var _objectWithoutPropertiesLoose2 = _interopRequireDefault(require("@babel/runtime/helpers/objectWithoutPropertiesLoose"));

var _react = _interopRequireDefault(require("react"));

var _propTypes = _interopRequireDefault(require("prop-types"));

var _classnames = _interopRequireDefault(require("classnames"));

var _utils = require("./utils");

function ownKeys(object, enumerableOnly) { var keys = Object.keys(object); if (Object.getOwnPropertySymbols) { var symbols = Object.getOwnPropertySymbols(object); if (enumerableOnly) symbols = symbols.filter(function (sym) { return Object.getOwnPropertyDescriptor(object, sym).enumerable; }); keys.push.apply(keys, symbols); } return keys; }

function _objectSpread(target) { for (var i = 1; i < arguments.length; i++) { var source = arguments[i] != null ? arguments[i] : {}; if (i % 2) { ownKeys(Object(source), true).forEach(function (key) { (0, _defineProperty2.default)(target, key, source[key]); }); } else if (Object.getOwnPropertyDescriptors) { Object.defineProperties(target, Object.getOwnPropertyDescriptors(source)); } else { ownKeys(Object(source)).forEach(function (key) { Object.defineProperty(target, key, Object.getOwnPropertyDescriptor(source, key)); }); } } return target; }

var propTypes = {
  children: _propTypes.default.node,
  bar: _propTypes.default.bool,
  multi: _propTypes.default.bool,
  tag: _utils.tagPropType,
  value: _propTypes.default.oneOfType([_propTypes.default.string, _propTypes.default.number]),
  min: _propTypes.default.oneOfType([_propTypes.default.string, _propTypes.default.number]),
  max: _propTypes.default.oneOfType([_propTypes.default.string, _propTypes.default.number]),
  animated: _propTypes.default.bool,
  striped: _propTypes.default.bool,
  color: _propTypes.default.string,
  className: _propTypes.default.string,
  barClassName: _propTypes.default.string,
  cssModule: _propTypes.default.object,
  style: _propTypes.default.object,
  barStyle: _propTypes.default.object,
  barAriaValueText: _propTypes.default.string,
  barAriaLabelledBy: _propTypes.default.string
};
var defaultProps = {
  tag: 'div',
  value: 0,
  min: 0,
  max: 100,
  style: {},
  barStyle: {}
};

var Progress = function Progress(props) {
  var children = props.children,
      className = props.className,
      barClassName = props.barClassName,
      cssModule = props.cssModule,
      value = props.value,
      min = props.min,
      max = props.max,
      animated = props.animated,
      striped = props.striped,
      color = props.color,
      bar = props.bar,
      multi = props.multi,
      Tag = props.tag,
      style = props.style,
      barStyle = props.barStyle,
      barAriaValueText = props.barAriaValueText,
      barAriaLabelledBy = props.barAriaLabelledBy,
      attributes = (0, _objectWithoutPropertiesLoose2.default)(props, ["children", "className", "barClassName", "cssModule", "value", "min", "max", "animated", "striped", "color", "bar", "multi", "tag", "style", "barStyle", "barAriaValueText", "barAriaLabelledBy"]);
  var percent = (0, _utils.toNumber)(value) / (0, _utils.toNumber)(max) * 100;
  var progressClasses = (0, _utils.mapToCssModules)((0, _classnames.default)(className, 'progress'), cssModule);
  var progressBarClasses = (0, _utils.mapToCssModules)((0, _classnames.default)('progress-bar', bar ? className || barClassName : barClassName, animated ? 'progress-bar-animated' : null, color ? "bg-" + color : null, striped || animated ? 'progress-bar-striped' : null), cssModule);
  var progressBarProps = {
    className: progressBarClasses,
    style: _objectSpread(_objectSpread(_objectSpread({}, bar ? style : {}), barStyle), {}, {
      width: percent + "%"
    }),
    role: 'progressbar',
    'aria-valuenow': value,
    'aria-valuemin': min,
    'aria-valuemax': max,
    'aria-valuetext': barAriaValueText,
    'aria-labelledby': barAriaLabelledBy,
    children: children
  };

  if (bar) {
    return /*#__PURE__*/_react.default.createElement(Tag, (0, _extends2.default)({}, attributes, progressBarProps));
  }

  return /*#__PURE__*/_react.default.createElement(Tag, (0, _extends2.default)({}, attributes, {
    style: style,
    className: progressClasses
  }), multi ? children : /*#__PURE__*/_react.default.createElement("div", progressBarProps));
};

Progress.propTypes = propTypes;
Progress.defaultProps = defaultProps;
var _default = Progress;
exports.default = _default;