"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});
exports.cleanup = cleanup;
exports.addCleanup = addCleanup;
exports.removeCleanup = removeCleanup;
exports.autoRegisterCleanup = autoRegisterCleanup;
let cleanupCallbacks = [];

async function cleanup() {
  for (const callback of cleanupCallbacks) {
    await callback();
  }

  cleanupCallbacks = [];
}

function addCleanup(callback) {
  cleanupCallbacks = [callback, ...cleanupCallbacks];
  return () => removeCleanup(callback);
}

function removeCleanup(callback) {
  cleanupCallbacks = cleanupCallbacks.filter(cb => cb !== callback);
}

function skipAutoCleanup() {
  try {
    return !!process.env.RHTL_SKIP_AUTO_CLEANUP;
  } catch {
    // falling back in the case that process.env.RHTL_SKIP_AUTO_CLEANUP cannot be accessed (e.g. browser environment)
    return false;
  }
}

function autoRegisterCleanup() {
  // Automatically registers cleanup in supported testing frameworks
  if (typeof afterEach === 'function' && !skipAutoCleanup()) {
    afterEach(async () => {
      await cleanup();
    });
  }
}