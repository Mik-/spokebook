// Shows the notice in #maintenance-status when an autosave of the open
// maintenance can't reach the server (htmx:sendError). The next successful
// save replaces #maintenance-status and with it the notice; it is also
// hidden here in case a response doesn't swap.
"use strict";

(() => {
  const notice = (show) => {
    const el = document.querySelector("[data-save-error]");
    if (el) el.hidden = !show;
  };
  document.addEventListener("htmx:sendError", () => notice(true));
  document.addEventListener("htmx:afterRequest", (e) => {
    if (e.detail.successful) notice(false);
  });
})();
