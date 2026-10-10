// Small native web components. No build step, no framework.

// <auto-save> saves its form in the background while you edit.
//   POST to the form's action with "X-Autosave: 1" (150ms after a click, 600ms after typing stops)
//            (204 = saved, otherwise the body is a short error message)
customElements.define('auto-save', class extends HTMLElement {
  connectedCallback() {
    const form = this.querySelector('form');
    const status = this.querySelector('[data-autosave-status]');
    const say = (state, text) => { status.dataset.state = state; status.textContent = text ?? this.getAttribute(state) ?? ''; };
    let timer = null, chain = Promise.resolve();
    const save = (keepalive = false) => {
      clearTimeout(timer);
      timer = null;
      chain = chain.then(async () => {
        const bad = [...form.elements].find((el) => el.willValidate && !el.checkValidity());
        if (bad) { say('error', bad.validationMessage); return; }
        say('saving');
        try {
          const res = await fetch(form.action, { method: 'POST', body: new URLSearchParams(new FormData(form)), headers: { 'X-Autosave': '1' }, keepalive });
          if (res.ok) say('saved', `${this.getAttribute('saved')} ${new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}`);
          else say('error', (await res.text()).trim());
        } catch { say('error', this.getAttribute('failed')); }
      });
    };
    const schedule = (e) => {
      if (!e.target.name) return; // unnamed inputs aren't part of the form data
      // Clicks save almost at once; typing waits for a short pause.
      const click = e.target.matches('input[type=checkbox], input[type=radio], select');
      clearTimeout(timer);
      timer = setTimeout(save, click ? 150 : 600);
    };
    form.addEventListener('input', schedule);
    form.addEventListener('change', schedule);
    addEventListener('pagehide', () => { if (timer) save(true); });
  }
});

// Theme picker on the poll form: switch the theme stylesheet live and keep the choice
// in the URL (?theme=…), so the preview survives a reload and can be shared.
for (const picker of document.querySelectorAll('[data-theme-picker]')) {
  picker.addEventListener('change', () => {
    document.querySelector('[data-theme-css]').href = `/theme.css?theme=${encodeURIComponent(picker.value)}`;
    const url = new URL(location.href);
    url.searchParams.set('theme', picker.value);
    history.replaceState(null, '', url);
  });
}

// <copy-link text="..." copied="Copied!"><button>Copy link</button></copy-link>
customElements.define('copy-link', class extends HTMLElement {
  connectedCallback() {
    const btn = this.querySelector('button');
    const text = btn.firstChild; // keep other children (e.g. a key hint) untouched
    const label = text.nodeValue;
    btn.setAttribute('aria-live', 'polite');
    btn.addEventListener('click', async () => {
      await navigator.clipboard.writeText(this.getAttribute('text'));
      text.nodeValue = this.getAttribute('copied');
      setTimeout(() => { text.nodeValue = label; }, 2000);
    });
  }
});

// <pow-form difficulty="18"><form>…</form></pow-form>
// On submit, finds a nonce so that sha256(challenge+nonce) has `difficulty` leading zero bits.
const zeroBits = (buf) => {
  const b = new Uint8Array(buf);
  let n = 0;
  for (const x of b) {
    if (x === 0) { n += 8; continue; }
    return n + Math.clz32(x) - 24;
  }
  return n;
};

async function solve(challenge, bits) {
  const enc = new TextEncoder();
  const batch = 64; // crypto.subtle.digest is async per hash; run many in parallel
  for (let n = 0; ; n += batch) {
    const hashes = await Promise.all(Array.from({ length: batch },
      (_, i) => crypto.subtle.digest('SHA-256', enc.encode(challenge + (n + i)))));
    const i = hashes.findIndex((h) => zeroBits(h) >= bits);
    if (i >= 0) return String(n + i);
  }
}

customElements.define('pow-form', class extends HTMLElement {
  connectedCallback() {
    const form = this.querySelector('form');
    const status = this.querySelector('.pow-status');
    let busy = false;
    form.addEventListener('submit', async (e) => {
      if (form.elements.nonce.value) return; // solved, submit for real
      e.preventDefault();
      if (busy) return;
      busy = true;
      form.querySelector('[type=submit]').disabled = true;
      status.textContent = this.getAttribute('working');
      const t0 = performance.now();
      form.elements.nonce.value = await solve(form.elements.pow.value, +this.getAttribute('difficulty'));
      console.debug(`pow solved in ${Math.round(performance.now() - t0)} ms`);
      form.submit();
    });
  }
});

// Keyboard shortcuts. Templates declare them with data-key="x" on the element to
// activate; each one shows its key as a hint next to it. Single keys work while not
// typing; Esc leaves a field (then acts as "back"), Ctrl/⌘+Enter submits the form.
// Only text entry counts as typing; a focused checkbox or button must not swallow shortcuts.
const typing = (el) => el instanceof Element && el.matches('input:not([type=checkbox], [type=radio], [type=button], [type=submit], [type=reset]), textarea, select, [contenteditable]');
const visible = (el) => (el.checkVisibility ? el.checkVisibility() : el.offsetParent !== null);
const keyName = (k) => (k === 'Escape' ? 'Esc' : k);

function activate(el) {
  if (el.matches('fieldset')) { el.querySelector('input[type=radio]:not(:checked)')?.click(); return; } // toggle
  if (el.matches('input[type=checkbox], input[type=radio]:not(:checked), button, a, summary')) el.click();
  if (el.matches('input, textarea, select')) {
    el.focus();
    try { el.setSelectionRange(el.value.length, el.value.length); } catch {} // caret at the end; not all input types support it
  }
}

document.addEventListener('keydown', (e) => {
  if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
    const form = e.target.closest?.('form');
    if (form) { e.preventDefault(); form.requestSubmit(); }
    return;
  }
  if (e.ctrlKey || e.metaKey || e.altKey || e.isComposing) return;
  if (typing(e.target)) {
    if (e.key === 'Escape') e.target.blur();
    return;
  }
  if (document.querySelector(':popover-open')) return; // the popover handles Esc itself
  const el = [...document.querySelectorAll('[data-key]')].find((x) => x.dataset.key === e.key && visible(x));
  if (el) { e.preventDefault(); activate(el); }
});

// Visible hints and aria-keyshortcuts for every declared key.
for (const el of document.querySelectorAll('[data-key]')) {
  el.setAttribute('aria-keyshortcuts', el.dataset.key);
  const kbd = document.createElement('kbd');
  kbd.className = 'key-hint';
  kbd.setAttribute('aria-hidden', 'true');
  kbd.textContent = keyName(el.dataset.key);
  if (el.matches('fieldset')) el.querySelector('legend').append(' ', kbd);
  else if (el.matches('input, textarea, select')) el.closest('label').append(kbd);
  else el.append(kbd);
}
