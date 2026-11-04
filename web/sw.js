// Service worker of the installed app. There is no offline mode: pages
// always come from the server. Only when it can't be reached does a
// navigation get the cached offline page instead of the browser's error.
// The server prepends `const VERSION = "...";`, so every release gets a
// fresh cache.
"use strict";

const CACHE = "spokebook-" + VERSION;
const OFFLINE = "/offline";
// The offline page and what it needs to render.
const ASSETS = [OFFLINE, "/static/vendor/pico-2.1.1.min.css", "/static/app.css", "/static/favicon.svg"];

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches
      .open(CACHE)
      .then((cache) => cache.addAll(ASSETS))
      .then(() => self.skipWaiting()),
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    (async () => {
      const keys = await caches.keys();
      await Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k)));
      if (self.registration.navigationPreload) {
        await self.registration.navigationPreload.enable();
      }
      await self.clients.claim();
    })(),
  );
});

self.addEventListener("fetch", (event) => {
  const req = event.request;
  if (req.mode === "navigate") {
    event.respondWith(
      (async () => {
        try {
          return (await event.preloadResponse) || (await fetch(req));
        } catch {
          return (await caches.match(OFFLINE)) || Response.error();
        }
      })(),
    );
    return;
  }
  const url = new URL(req.url);
  if (url.origin === self.location.origin && ASSETS.includes(url.pathname)) {
    event.respondWith(fetch(req).catch(async () => (await caches.match(req)) || Response.error()));
  }
});
