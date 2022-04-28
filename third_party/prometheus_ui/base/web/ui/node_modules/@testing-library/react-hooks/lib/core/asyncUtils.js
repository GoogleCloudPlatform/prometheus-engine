"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});
exports.asyncUtils = asyncUtils;

var _createTimeoutController = require("../helpers/createTimeoutController");

var _error = require("../helpers/error");

const DEFAULT_INTERVAL = 50;
const DEFAULT_TIMEOUT = 1000;

function asyncUtils(act, addResolver) {
  const wait = async (callback, {
    interval,
    timeout
  }) => {
    const checkResult = () => {
      const callbackResult = callback();
      return callbackResult != null ? callbackResult : callbackResult === undefined;
    };

    const timeoutSignal = (0, _createTimeoutController.createTimeoutController)(timeout);

    const waitForResult = async () => {
      while (true) {
        const intervalSignal = (0, _createTimeoutController.createTimeoutController)(interval);
        timeoutSignal.onTimeout(() => intervalSignal.cancel());
        await intervalSignal.wrap(new Promise(addResolver));

        if (checkResult() || timeoutSignal.timedOut) {
          return;
        }
      }
    };

    if (!checkResult()) {
      await act(() => timeoutSignal.wrap(waitForResult()));
    }

    return !timeoutSignal.timedOut;
  };

  const waitFor = async (callback, {
    interval = DEFAULT_INTERVAL,
    timeout = DEFAULT_TIMEOUT
  } = {}) => {
    const safeCallback = () => {
      try {
        return callback();
      } catch (error) {
        return false;
      }
    };

    const result = await wait(safeCallback, {
      interval,
      timeout
    });

    if (!result && timeout) {
      throw new _error.TimeoutError(waitFor, timeout);
    }
  };

  const waitForValueToChange = async (selector, {
    interval = DEFAULT_INTERVAL,
    timeout = DEFAULT_TIMEOUT
  } = {}) => {
    const initialValue = selector();
    const result = await wait(() => selector() !== initialValue, {
      interval,
      timeout
    });

    if (!result && timeout) {
      throw new _error.TimeoutError(waitForValueToChange, timeout);
    }
  };

  const waitForNextUpdate = async ({
    timeout = DEFAULT_TIMEOUT
  } = {}) => {
    let updated = false;
    addResolver(() => {
      updated = true;
    });
    const result = await wait(() => updated, {
      interval: false,
      timeout
    });

    if (!result && timeout) {
      throw new _error.TimeoutError(waitForNextUpdate, timeout);
    }
  };

  return {
    waitFor,
    waitForValueToChange,
    waitForNextUpdate
  };
}