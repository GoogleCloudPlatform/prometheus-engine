"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});
exports.createTimeoutController = createTimeoutController;

function createTimeoutController(timeout) {
  let timeoutId;
  const timeoutCallbacks = [];
  const timeoutController = {
    onTimeout(callback) {
      timeoutCallbacks.push(callback);
    },

    wrap(promise) {
      return new Promise((resolve, reject) => {
        timeoutController.timedOut = false;
        timeoutController.onTimeout(resolve);

        if (timeout) {
          timeoutId = setTimeout(() => {
            timeoutController.timedOut = true;
            timeoutCallbacks.forEach(callback => callback());
            resolve();
          }, timeout);
        }

        promise.then(resolve).catch(reject).finally(() => timeoutController.cancel());
      });
    },

    cancel() {
      clearTimeout(timeoutId);
    },

    timedOut: false
  };
  return timeoutController;
}