// Registers the service worker (/sw.js), which shows an offline page when
// the server can't be reached. Installing the app needs the manifest only.
"use strict";

if ("serviceWorker" in navigator) {
  navigator.serviceWorker.register("/sw.js").catch(() => {
    // Not over https (other than localhost) or blocked; the app works without.
  });
}
