"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});

var _pure = require("./pure");

Object.keys(_pure).forEach(function (key) {
  if (key === "default" || key === "__esModule") return;
  if (key in exports && exports[key] === _pure[key]) return;
  Object.defineProperty(exports, key, {
    enumerable: true,
    get: function () {
      return _pure[key];
    }
  });
});

// Automatically registers cleanup in supported testing frameworks
if (typeof afterEach === 'function' && !process.env.RHTL_SKIP_AUTO_CLEANUP) {
  afterEach(async () => {
    await (0, _pure.cleanup)();
  });
}