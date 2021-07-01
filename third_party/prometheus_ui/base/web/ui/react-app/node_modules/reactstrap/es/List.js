import _extends from "@babel/runtime/helpers/esm/extends";
import _objectWithoutPropertiesLoose from "@babel/runtime/helpers/esm/objectWithoutPropertiesLoose";
import React, { forwardRef } from 'react';
import PropTypes from 'prop-types';
import classNames from 'classnames';
import { mapToCssModules, tagPropType } from './utils';
var propTypes = {
  tag: tagPropType,
  className: PropTypes.string,
  cssModule: PropTypes.object,
  type: PropTypes.string
};
var defaultProps = {
  tag: 'ul'
};
var List = /*#__PURE__*/forwardRef(function (props, ref) {
  var className = props.className,
      cssModule = props.cssModule,
      Tag = props.tag,
      type = props.type,
      attributes = _objectWithoutPropertiesLoose(props, ["className", "cssModule", "tag", "type"]);

  var classes = mapToCssModules(classNames(className, type ? "list-" + type : false), cssModule);
  return /*#__PURE__*/React.createElement(Tag, _extends({}, attributes, {
    className: classes,
    ref: ref
  }));
});
List.propTypes = propTypes;
List.defaultProps = defaultProps;
export default List;