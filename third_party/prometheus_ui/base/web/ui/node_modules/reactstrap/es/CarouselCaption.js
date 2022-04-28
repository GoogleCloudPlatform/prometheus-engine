import React from 'react';
import PropTypes from 'prop-types';
import classNames from 'classnames';
import { mapToCssModules } from './utils';

var CarouselCaption = function CarouselCaption(props) {
  var captionHeader = props.captionHeader,
      captionText = props.captionText,
      cssModule = props.cssModule,
      className = props.className;
  var classes = mapToCssModules(classNames(className, 'carousel-caption', 'd-none', 'd-md-block'), cssModule);
  return /*#__PURE__*/React.createElement("div", {
    className: classes
  }, /*#__PURE__*/React.createElement("h3", null, captionHeader), /*#__PURE__*/React.createElement("p", null, captionText));
};

CarouselCaption.propTypes = {
  captionHeader: PropTypes.node,
  captionText: PropTypes.node.isRequired,
  cssModule: PropTypes.object,
  className: PropTypes.string
};
export default CarouselCaption;