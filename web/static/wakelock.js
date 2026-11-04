// Keeps the screen on while the page is visible (Screen Wake Lock API), so a
// phone does not lock during a maintenance. The browser releases the lock
// when the page is hidden, so it is requested again when it becomes visible.
// Without support (or outside a secure context) nothing happens.
"use strict";

(() => {
  if (!("wakeLock" in navigator)) return;

  let lock = null;
  const request = async () => {
    if (lock || document.visibilityState !== "visible") return;
    try {
      lock = await navigator.wakeLock.request("screen");
      lock.addEventListener("release", () => {
        lock = null;
      });
    } catch {
      // Denied, e.g. in battery saver mode; the screen just locks as usual.
    }
  };

  document.addEventListener("visibilitychange", request);
  request();
})();
