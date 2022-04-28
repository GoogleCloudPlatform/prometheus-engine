"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});
var _exportNames = {
  renderHook: true,
  act: true,
  cleanup: true,
  addCleanup: true,
  removeCleanup: true,
  suppressErrorOutput: true
};
Object.defineProperty(exports, "act", {
  enumerable: true,
  get: function () {
    return _testUtils.act;
  }
});
Object.defineProperty(exports, "cleanup", {
  enumerable: true,
  get: function () {
    return _core.cleanup;
  }
});
Object.defineProperty(exports, "addCleanup", {
  enumerable: true,
  get: function () {
    return _core.addCleanup;
  }
});
Object.defineProperty(exports, "removeCleanup", {
  enumerable: true,
  get: function () {
    return _core.removeCleanup;
  }
});
Object.defineProperty(exports, "suppressErrorOutput", {
  enumerable: true,
  get: function () {
    return _core.suppressErrorOutput;
  }
});
exports.renderHook = void 0;

var ReactDOMServer = _interopRequireWildcard(require("react-dom/server"));

var ReactDOM = _interopRequireWildcard(require("react-dom"));

var _testUtils = require("react-dom/test-utils");

var _core = require("../core");

var _createTestHarness = require("../helpers/createTestHarness");

var _react = require("../types/react");

Object.keys(_react).forEach(function (key) {
  if (key === "default" || key === "__esModule") return;
  if (Object.prototype.hasOwnProperty.call(_exportNames, key)) return;
  if (key in exports && exports[key] === _react[key]) return;
  Object.defineProperty(exports, key, {
    enumerable: true,
    get: function () {
      return _react[key];
    }
  });
});

function _getRequireWildcardCache(nodeInterop) { if (typeof WeakMap !== "function") return null; var cacheBabelInterop = new WeakMap(); var cacheNodeInterop = new WeakMap(); return (_getRequireWildcardCache = function (nodeInterop) { return nodeInterop ? cacheNodeInterop : cacheBabelInterop; })(nodeInterop); }

function _interopRequireWildcard(obj, nodeInterop) { if (!nodeInterop && obj && obj.__esModule) { return obj; } if (obj === null || typeof obj !== "object" && typeof obj !== "function") { return { default: obj }; } var cache = _getRequireWildcardCache(nodeInterop); if (cache && cache.has(obj)) { return cache.get(obj); } var newObj = {}; var hasPropertyDescriptor = Object.defineProperty && Object.getOwnPropertyDescriptor; for (var key in obj) { if (key !== "default" && Object.prototype.hasOwnProperty.call(obj, key)) { var desc = hasPropertyDescriptor ? Object.getOwnPropertyDescriptor(obj, key) : null; if (desc && (desc.get || desc.set)) { Object.defineProperty(newObj, key, desc); } else { newObj[key] = obj[key]; } } } newObj.default = obj; if (cache) { cache.set(obj, newObj); } return newObj; }

function createServerRenderer(rendererProps, {
  wrapper
}) {
  let renderProps;
  let container;
  let serverOutput = '';
  const testHarness = (0, _createTestHarness.createTestHarness)(rendererProps, wrapper, false);
  return {
    render(props) {
      renderProps = props;
      (0, _testUtils.act)(() => {
        try {
          serverOutput = ReactDOMServer.renderToString(testHarness(props));
        } catch (e) {
          rendererProps.setError(e);
        }
      });
    },

    hydrate() {
      if (container) {
        throw new Error('The component can only be hydrated once');
      } else {
        container = document.createElement('div');
        container.innerHTML = serverOutput;
        (0, _testUtils.act)(() => {
          ReactDOM.hydrate(testHarness(renderProps), container);
        });
      }
    },

    rerender(props) {
      if (!container) {
        throw new Error('You must hydrate the component before you can rerender');
      }

      (0, _testUtils.act)(() => {
        ReactDOM.render(testHarness(props), container);
      });
    },

    unmount() {
      if (container) {
        (0, _testUtils.act)(() => {
          ReactDOM.unmountComponentAtNode(container);
        });
      }
    },

    act: _testUtils.act
  };
}

const renderHook = (0, _core.createRenderHook)(createServerRenderer);
exports.renderHook = renderHook;