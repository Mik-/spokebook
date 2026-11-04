// Passkeys (WebAuthn): login via [data-passkey-login], registration via
// form[data-passkey-register]. Both fetch options from data-begin, ask the
// browser and post its answer to data-finish, which replies with
// {redirect} or {error}. Without WebAuthn support nothing is shown.
"use strict";

(() => {
  if (!window.PublicKeyCredential || !navigator.credentials) return;

  const toBytes = (s) => {
    const b64 = s.replace(/-/g, "+").replace(/_/g, "/");
    return Uint8Array.from(atob(b64 + "=".repeat((4 - (b64.length % 4)) % 4)), (c) => c.charCodeAt(0));
  };
  const toBase64URL = (buf) => {
    let s = "";
    for (const b of new Uint8Array(buf)) s += String.fromCharCode(b);
    return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  };

  const post = async (url, body) => {
    const res = await fetch(url, { method: "POST", body, credentials: "same-origin" });
    const data = await res.json().catch(() => ({}));
    if (!res.ok || data.error) {
      const err = new Error(data.error || "");
      err.fromServer = true;
      throw err;
    }
    return data;
  };

  const showError = (el, msg) => {
    const box = document.querySelector("[data-passkey-error]");
    if (!box) return;
    box.textContent = msg || el.dataset.error;
    box.hidden = false;
  };

  // credentialJSON encodes the browser's answer like
  // PublicKeyCredential.toJSON, which not all browsers have yet.
  const credentialJSON = (cred) => {
    const r = cred.response;
    const response = { clientDataJSON: toBase64URL(r.clientDataJSON) };
    if (r.attestationObject) {
      response.attestationObject = toBase64URL(r.attestationObject);
      if (r.getTransports) response.transports = r.getTransports();
    } else {
      response.authenticatorData = toBase64URL(r.authenticatorData);
      response.signature = toBase64URL(r.signature);
      if (r.userHandle) response.userHandle = toBase64URL(r.userHandle);
    }
    return JSON.stringify({
      id: cred.id,
      rawId: toBase64URL(cred.rawId),
      type: cred.type,
      authenticatorAttachment: cred.authenticatorAttachment || undefined,
      response,
      clientExtensionResults: cred.getClientExtensionResults(),
    });
  };

  const run = async (el, body, ceremony) => {
    const box = document.querySelector("[data-passkey-error]");
    if (box) box.hidden = true;
    try {
      const { publicKey } = await post(el.dataset.begin, body);
      publicKey.challenge = toBytes(publicKey.challenge);
      for (const c of publicKey.excludeCredentials || []) c.id = toBytes(c.id);
      for (const c of publicKey.allowCredentials || []) c.id = toBytes(c.id);
      if (publicKey.user) publicKey.user.id = toBytes(publicKey.user.id);
      const cred = await ceremony({ publicKey });
      const { redirect } = await post(el.dataset.finish, credentialJSON(cred));
      window.location.assign(redirect);
    } catch (err) {
      // Cancelled by the user: nothing to report.
      if (err.name === "NotAllowedError" || err.name === "AbortError") return;
      // Other errors (network, browser) have untranslated messages.
      showError(el, err.fromServer ? err.message : "");
    }
  };

  for (const el of document.querySelectorAll("[data-passkey-supported]")) el.hidden = false;
  for (const el of document.querySelectorAll("[data-passkey-unsupported]")) el.hidden = true;

  for (const btn of document.querySelectorAll("[data-passkey-login]")) {
    btn.addEventListener("click", () => run(btn, null, (o) => navigator.credentials.get(o)));
  }
  for (const form of document.querySelectorAll("form[data-passkey-register]")) {
    form.hidden = false;
    form.addEventListener("submit", (e) => {
      e.preventDefault();
      run(form, new FormData(form), (o) => navigator.credentials.create(o));
    });
  }
})();
