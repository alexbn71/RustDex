'use strict';

const I18N = {
  en: {
    loginSubtitle: 'RustDesk address book',
    username: 'Username',
    password: 'Password',
    login: 'Log in',
    logout: 'Log out',
    search: 'Search name, ID, company, contact, tag…',
    newPeer: '+ New',
    manageTags: 'Tags',
    name: 'Name',
    company: 'Company',
    contact: 'Contact person',
    email: 'Email',
    phone: 'Phone',
    address: 'Address',
    note: 'Notes',
    tags: 'Tags',
    status: 'Status',
    all: 'All',
    connect: 'Connect',
    copyId: 'Copy ID',
    edit: 'Edit',
    delete: 'Delete',
    cancel: 'Cancel',
    save: 'Save',
    add: 'Add',
    close: 'Close',
    addTag: 'Add a tag and press Enter',
    newTag: 'New tag',
    showPassword: 'Show password',
    copyPassword: 'Copy password',
    newTitle: 'New computer',
    editTitle: 'Edit {name}',
    passwordKept: '•••••••• (unchanged)',
    passwordNone: 'No password saved',
    online: 'Online',
    neverSeen: '—',
    statusHelp: 'Only computers whose RustDesk uses this API server report their status',
    hostname: 'Computer name',
    osUser: 'User',
    platform: 'Platform',
    os: 'Operating system',
    rdVersion: 'RustDesk version',
    lastSeen: 'Last seen',
    deviceInfo: 'Reported by RustDesk',
    count: '{n} computers',
    countOne: '1 computer',
    empty: 'The address book is empty. Add the first computer with “+ New”.',
    noMatch: 'No computer matches the search.',
    saved: 'Saved',
    deleted: 'Deleted',
    copied: 'Copied',
    copyFailed: 'Copy failed',
    confirmDelete: 'Delete {name} from the address book?',
    confirmDeleteTag: 'Delete the tag “{name}”? It is removed from every computer.',
    tagsHelp: 'Tags group computers, here and in the RustDesk client.',
    idRequired: 'The ID is required',
    idExists: 'This ID is already in the address book',
    badLogin: 'Wrong username or password',
    tooMany: 'Too many failed attempts, try again later',
    sessionExpired: 'Session expired, log in again',
    networkError: 'Cannot reach the server',
  },
  it: {
    loginSubtitle: 'Rubrica per RustDesk',
    username: 'Utente',
    password: 'Password',
    login: 'Accedi',
    logout: 'Esci',
    search: 'Cerca nome, ID, azienda, referente, tag…',
    newPeer: '+ Nuovo',
    manageTags: 'Tag',
    name: 'Nome',
    company: 'Azienda',
    contact: 'Referente',
    email: 'Email',
    phone: 'Telefono',
    address: 'Indirizzo',
    note: 'Note',
    tags: 'Tag',
    status: 'Stato',
    all: 'Tutti',
    connect: 'Connetti',
    copyId: 'Copia ID',
    edit: 'Modifica',
    delete: 'Elimina',
    cancel: 'Annulla',
    save: 'Salva',
    add: 'Aggiungi',
    close: 'Chiudi',
    addTag: 'Aggiungi un tag e premi Invio',
    newTag: 'Nuovo tag',
    showPassword: 'Mostra password',
    copyPassword: 'Copia password',
    newTitle: 'Nuovo computer',
    editTitle: 'Modifica {name}',
    passwordKept: '•••••••• (invariata)',
    passwordNone: 'Nessuna password salvata',
    online: 'Online',
    neverSeen: '—',
    statusHelp: 'Lo stato è disponibile solo per i computer il cui RustDesk usa questo server API',
    hostname: 'Nome computer',
    osUser: 'Utente',
    platform: 'Piattaforma',
    os: 'Sistema operativo',
    rdVersion: 'Versione RustDesk',
    lastSeen: 'Ultimo contatto',
    deviceInfo: 'Dati inviati da RustDesk',
    count: '{n} computer',
    countOne: '1 computer',
    empty: 'La rubrica è vuota. Aggiungi il primo computer con “+ Nuovo”.',
    noMatch: 'Nessun computer corrisponde alla ricerca.',
    saved: 'Salvato',
    deleted: 'Eliminato',
    copied: 'Copiato',
    copyFailed: 'Copia non riuscita',
    confirmDelete: 'Eliminare {name} dalla rubrica?',
    confirmDeleteTag: 'Eliminare il tag “{name}”? Viene tolto da tutti i computer.',
    tagsHelp: 'I tag raggruppano i computer, qui e nel client RustDesk.',
    idRequired: 'L’ID è obbligatorio',
    idExists: 'Questo ID è già in rubrica',
    badLogin: 'Utente o password errati',
    tooMany: 'Troppi tentativi falliti, riprova più tardi',
    sessionExpired: 'Sessione scaduta, accedi di nuovo',
    networkError: 'Server non raggiungibile',
  },
};

const lang = (navigator.language || 'en').toLowerCase().startsWith('it') ? 'it' : 'en';
document.documentElement.lang = lang;

function t(key, vars) {
  let s = I18N[lang][key] ?? I18N.en[key] ?? key;
  for (const k in vars || {}) s = s.replace(`{${k}}`, vars[k]);
  return s;
}

// localStorage can be missing or throw (private mode, blocked storage).
const storage = {
  get(k) { try { return localStorage.getItem(k); } catch { return null; } },
  set(k, v) {
    try { v == null ? localStorage.removeItem(k) : localStorage.setItem(k, v); } catch { /* ignore */ }
  },
};

const state = {
  token: storage.get('rustdex.token'),
  peers: [],
  tags: [],
  filterTag: null,
  query: '',
  sort: { key: 'name', dir: 1 },
  editing: null,     // peer being edited, null for a new one
  formTags: [],
  pwLoaded: false,
  pwDirty: false,
};

const $ = (sel, root = document) => root.querySelector(sel);

function el(tag, attrs, ...children) {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v == null || v === false) continue;
    if (k === 'class') node.className = v;
    else if (k.startsWith('on')) node.addEventListener(k.slice(2), v);
    else node.setAttribute(k, v === true ? '' : v);
  }
  for (const c of children.flat()) {
    if (c == null || c === false) continue;
    node.append(c instanceof Node ? c : document.createTextNode(String(c)));
  }
  return node;
}

const ICONS = {
  connect: 'M3 4h18v12H3z M8 20h8 M12 16v4',
  copy: 'M9 9h11v11H9z M5 15H4V4h11v1',
  edit: 'M4 20h4L19 9l-4-4L4 16z M13 7l4 4',
  eye: 'M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z M12 9a3 3 0 1 0 0 6a3 3 0 1 0 0-6z',
  eyeOff: 'M3 3l18 18 M10.6 5.1A10 10 0 0 1 12 5c6.5 0 10 7 10 7a17 17 0 0 1-3.2 4.2 M6.6 6.6A17 17 0 0 0 2 12s3.5 7 10 7a9.7 9.7 0 0 0 5.4-1.6 M9.9 9.9a3 3 0 0 0 4.2 4.2',
  trash: 'M4 7h16 M9 7V4h6v3 M6 7l1 13h10l1-13',
};

function icon(name) {
  const NS = 'http://www.w3.org/2000/svg';
  const svg = document.createElementNS(NS, 'svg');
  svg.setAttribute('viewBox', '0 0 24 24');
  svg.setAttribute('class', 'ico');
  svg.setAttribute('aria-hidden', 'true');
  const path = document.createElementNS(NS, 'path');
  path.setAttribute('d', ICONS[name]);
  svg.append(path);
  return svg;
}

class ApiError extends Error {
  constructor(status, message) { super(message); this.status = status; }
}

async function api(method, path, body) {
  const headers = {};
  if (state.token) headers.Authorization = 'Bearer ' + state.token;
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  let res;
  try {
    res = await fetch(path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
  } catch {
    throw new ApiError(0, t('networkError'));
  }
  const text = await res.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { /* not JSON */ }
  if (res.status === 401 && path !== '/api/login') {
    showLogin(t('sessionExpired'));
    throw new ApiError(401, t('sessionExpired'));
  }
  if (!res.ok || (data && data.error)) {
    throw new ApiError(res.status, (data && data.error) || res.statusText);
  }
  return data;
}

function errorText(err) {
  switch (err.status) {
    case 401: return t('badLogin');
    case 409: return t('idExists');
    case 429: return t('tooMany');
  }
  return err.message;
}

let toastTimer;
function toast(msg) {
  const node = $('#toast');
  node.textContent = msg;
  node.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { node.hidden = true; }, 2200);
}

function showError(node, msg) {
  node.textContent = msg || '';
  node.hidden = !msg;
}

// navigator.clipboard only works on https or localhost; the fallback covers
// the plain-http LAN address most installs use.
async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text);
    toast(t('copied'));
    return;
  } catch { /* fall back */ }
  const area = el('textarea', { readonly: true });
  area.value = text;
  area.style.position = 'fixed';
  area.style.opacity = '0';
  document.body.append(area);
  area.select();
  let ok = false;
  try { ok = document.execCommand('copy'); } catch { /* ignore */ }
  area.remove();
  toast(ok ? t('copied') : t('copyFailed'));
}

/* ---------- Tags ---------- */

function tagColor(name) {
  const tag = state.tags.find((x) => x.name === name);
  return tag ? argbToHex(tag.color) : '#888888';
}

function argbToHex(color) {
  return '#' + (Number(color) & 0xffffff).toString(16).padStart(6, '0');
}

function hexToArgb(hex) {
  return 0xff000000 + parseInt(hex.slice(1), 16);
}

function textOn(hex) {
  const n = parseInt(hex.slice(1), 16);
  const lum = 0.299 * (n >> 16) + 0.587 * ((n >> 8) & 255) + 0.114 * (n & 255);
  return lum > 150 ? '#111111' : '#ffffff';
}

function tagBadge(name, ...extra) {
  const bg = tagColor(name);
  const node = el('span', { class: 'tag' }, name, ...extra);
  node.style.background = bg;
  node.style.color = textOn(bg);
  return node;
}

/* ---------- List ---------- */

function displayName(p) { return p.alias || p.id; }

function isOnline(p) { return p.lastSeen && Date.now() / 1000 - p.lastSeen < 60; }

function relativeTime(unix) {
  const diff = unix - Date.now() / 1000;
  const rtf = new Intl.RelativeTimeFormat(lang, { numeric: 'auto' });
  const steps = [[60, 'second'], [3600, 'minute'], [86400, 'hour'], [604800, 'day'], [2629800, 'week'], [31557600, 'month']];
  let unit = 'year';
  let div = 31557600;
  let prev = 1;
  for (const [limit, u] of steps) {
    if (Math.abs(diff) < limit) { unit = u; div = prev; break; }
    prev = limit;
  }
  return rtf.format(Math.round(diff / div), unit);
}

function matches(p, terms) {
  const hay = [p.alias, p.id, p.company, p.contact, p.email, p.phone, p.address, p.note,
    p.hostname, p.username, ...p.tags].join(' ').toLowerCase();
  return terms.every((term) => hay.includes(term));
}

function sortValue(p, key) {
  switch (key) {
    case 'name': return displayName(p).toLowerCase();
    case 'lastSeen': return -(p.lastSeen || 0);
    default: return String(p[key] || '').toLowerCase();
  }
}

function visiblePeers() {
  const terms = state.query.toLowerCase().split(/\s+/).filter(Boolean);
  const { key, dir } = state.sort;
  return state.peers
    .filter((p) => !state.filterTag || p.tags.includes(state.filterTag))
    .filter((p) => matches(p, terms))
    .sort((a, b) => {
      const va = sortValue(a, key);
      const vb = sortValue(b, key);
      if (va === vb) return displayName(a).localeCompare(displayName(b));
      if (va === '') return 1;
      if (vb === '') return -1;
      return (va < vb ? -1 : 1) * dir;
    });
}

function renderTagFilter() {
  const nav = $('#tag-filter');
  nav.replaceChildren();
  if (!state.tags.length) return;
  const chip = (label, value, color, count) => {
    const node = el('button', {
      type: 'button',
      class: 'chip',
      'aria-pressed': String(state.filterTag === value),
      onclick: () => { state.filterTag = state.filterTag === value ? null : value; render(); },
    });
    if (color) {
      const dot = el('span', { class: 'dot' });
      dot.style.background = color;
      node.append(dot);
    }
    node.append(label);
    if (count != null) node.append(el('span', { class: 'count' }, count));
    return node;
  };
  nav.append(chip(t('all'), null, null, state.peers.length));
  for (const tag of state.tags) {
    const count = state.peers.filter((p) => p.tags.includes(tag.name)).length;
    nav.append(chip(tag.name, tag.name, argbToHex(tag.color), count));
  }
}

function renderStatus(p) {
  if (isOnline(p)) {
    return el('span', { class: 'status online', title: t('statusHelp') }, el('span', { class: 'dot' }), t('online'));
  }
  if (p.lastSeen) {
    return el('span', { class: 'status', title: new Date(p.lastSeen * 1000).toLocaleString(lang) }, relativeTime(p.lastSeen));
  }
  return el('span', { class: 'status', title: t('statusHelp') }, t('neverSeen'));
}

function renderRow(p) {
  const stop = (fn) => (e) => { e.stopPropagation(); fn(); };
  const sub = [p.hostname, p.username].filter(Boolean).join(' · ');
  return el('tr', { onclick: () => openPeer(p) },
    el('td', { class: 'c-name' },
      el('div', { class: 'peer-name' }, displayName(p)),
      sub && el('div', { class: 'peer-sub' }, sub)),
    el('td', { class: 'c-id mono' }, p.id),
    el('td', { class: 'c-company' }, p.company),
    el('td', { class: 'c-contact' }, p.contact),
    el('td', { class: 'c-tags' }, p.tags.map((name) => tagBadge(name))),
    el('td', { class: 'c-status' }, renderStatus(p)),
    el('td', { class: 'c-actions' },
      el('div', { class: 'row-actions' },
        el('button', { type: 'button', class: 'btn primary small', onclick: stop(() => connect(p)) },
          icon('connect'), t('connect')),
        el('button', { type: 'button', class: 'btn icon', title: t('copyId'), 'aria-label': t('copyId'), onclick: stop(() => copyText(p.id)) },
          icon('copy')),
        el('button', { type: 'button', class: 'btn icon', title: t('edit'), 'aria-label': t('edit'), onclick: stop(() => openPeer(p)) },
          icon('edit')))));
}

function render() {
  renderTagFilter();
  const peers = visiblePeers();
  $('#peer-table tbody').replaceChildren(...peers.map(renderRow));
  $('#peer-table').hidden = peers.length === 0;
  const empty = $('#empty');
  empty.hidden = peers.length > 0;
  empty.textContent = state.peers.length ? t('noMatch') : t('empty');
  $('#count').textContent = state.peers.length === 1 ? t('countOne') : t('count', { n: state.peers.length });
  for (const th of document.querySelectorAll('th[data-sort]')) {
    if (th.dataset.sort === state.sort.key) th.setAttribute('aria-sort', state.sort.dir > 0 ? 'ascending' : 'descending');
    else th.removeAttribute('aria-sort');
  }
}

async function load() {
  const data = await api('GET', '/rx/api/peers');
  state.peers = data.peers;
  state.tags = data.tags;
  if (state.filterTag && !state.tags.some((x) => x.name === state.filterTag)) state.filterTag = null;
  render();
  if ($('#tags-dialog').open) renderTagsManager();
}

async function connect(p) {
  let url = 'rustdesk://connect/' + encodeURIComponent(p.id);
  if (p.hasPassword) {
    try {
      const { password } = await api('GET', `/rx/api/peers/${encodeURIComponent(p.id)}/password`);
      if (password) url += '?password=' + encodeURIComponent(password);
    } catch (err) {
      toast(errorText(err));
      return;
    }
  }
  window.location.href = url;
}

/* ---------- Peer dialog ---------- */

function renderFormTags() {
  const editor = $('#tag-editor');
  const input = $('#tag-input');
  editor.replaceChildren(...state.formTags.map((name) => tagBadge(name,
    el('button', {
      type: 'button',
      'aria-label': '×',
      onclick: () => { state.formTags = state.formTags.filter((x) => x !== name); renderFormTags(); },
    }, '×'))), input);
  $('#tag-list').replaceChildren(...state.tags
    .filter((x) => !state.formTags.includes(x.name))
    .map((x) => el('option', { value: x.name })));
}

function addFormTag() {
  const input = $('#tag-input');
  const name = input.value.replace(/,/g, ' ').trim();
  if (name && !state.formTags.includes(name)) state.formTags.push(name);
  input.value = '';
  renderFormTags();
}

function setPasswordVisible(visible) {
  const input = $('#peer-password');
  input.type = visible ? 'text' : 'password';
  const btn = $('#pw-toggle');
  btn.replaceChildren(icon(visible ? 'eyeOff' : 'eye'));
}

async function loadPassword() {
  if (state.pwLoaded || !state.editing || !state.editing.hasPassword) return;
  const { password } = await api('GET', `/rx/api/peers/${encodeURIComponent(state.editing.id)}/password`);
  const input = $('#peer-password');
  if (!state.pwDirty) input.value = password;
  state.pwLoaded = true;
}

function renderDeviceInfo(p) {
  const dl = $('#device-info');
  const rows = p ? [
    [t('hostname'), p.hostname],
    [t('osUser'), p.username],
    [t('platform'), p.os || p.platform],
    [t('rdVersion'), p.version],
    [t('lastSeen'), p.lastSeen ? new Date(p.lastSeen * 1000).toLocaleString(lang) : ''],
  ].filter(([, v]) => v) : [];
  dl.replaceChildren(...rows.flatMap(([k, v]) => [el('dt', {}, k), el('dd', {}, v)]));
  dl.hidden = rows.length === 0;
}

function openPeer(p) {
  state.editing = p || null;
  state.formTags = p ? [...p.tags] : (state.filterTag ? [state.filterTag] : []);
  state.pwLoaded = false;
  state.pwDirty = false;

  const form = $('#peer-form');
  form.reset();
  for (const name of ['alias', 'id', 'company', 'contact', 'email', 'phone', 'address', 'note']) {
    form.elements[name].value = p ? p[name] || '' : '';
  }
  const pw = $('#peer-password');
  pw.value = '';
  pw.placeholder = p ? (p.hasPassword ? t('passwordKept') : t('passwordNone')) : '';
  setPasswordVisible(false);
  $('#pw-copy').hidden = !(p && p.hasPassword);

  $('#peer-title').textContent = p ? t('editTitle', { name: displayName(p) }) : t('newTitle');
  $('#btn-delete').hidden = !p;
  showError($('#peer-error'), '');
  renderFormTags();
  renderDeviceInfo(p);
  $('#peer-dialog').showModal();
  (p ? form.elements.alias : form.elements.id).focus();
}

async function savePeer(e) {
  e.preventDefault();
  const form = $('#peer-form');
  if ($('#tag-input').value.trim()) addFormTag();
  const body = {};
  for (const name of ['alias', 'id', 'company', 'contact', 'email', 'phone', 'address', 'note']) {
    body[name] = form.elements[name].value;
  }
  body.tags = state.formTags;
  if (!body.id.trim()) {
    showError($('#peer-error'), t('idRequired'));
    form.elements.id.focus();
    return;
  }
  // Omitted password = keep the stored one.
  if (!state.editing || state.pwDirty) body.password = $('#peer-password').value;

  try {
    if (state.editing) {
      await api('PUT', `/rx/api/peers/${encodeURIComponent(state.editing.id)}`, body);
    } else {
      await api('POST', '/rx/api/peers', body);
    }
  } catch (err) {
    showError($('#peer-error'), errorText(err));
    return;
  }
  $('#peer-dialog').close();
  toast(t('saved'));
  await load();
}

async function deletePeer() {
  const p = state.editing;
  if (!p || !confirm(t('confirmDelete', { name: displayName(p) }))) return;
  try {
    await api('DELETE', `/rx/api/peers/${encodeURIComponent(p.id)}`);
  } catch (err) {
    showError($('#peer-error'), errorText(err));
    return;
  }
  $('#peer-dialog').close();
  toast(t('deleted'));
  await load();
}

/* ---------- Tags dialog ---------- */

function renderTagsManager() {
  const list = $('#tags-list');
  list.replaceChildren(...state.tags.map((tag) => {
    const color = el('input', { type: 'color', value: argbToHex(tag.color), 'aria-label': 'color' });
    color.addEventListener('change', () => updateTag(tag.name, { color: hexToArgb(color.value) }));
    const name = el('input', { value: tag.name, maxlength: '50', 'aria-label': t('name') });
    name.addEventListener('change', () => {
      const v = name.value.trim();
      if (v && v !== tag.name) updateTag(tag.name, { name: v });
      else name.value = tag.name;
    });
    const del = el('button', {
      type: 'button',
      class: 'btn icon danger',
      title: t('delete'),
      'aria-label': t('delete'),
      onclick: () => deleteTag(tag.name),
    }, icon('trash'));
    return el('li', {}, color, name, del);
  }));
}

async function updateTag(name, body) {
  try {
    await api('PUT', `/rx/api/tags/${encodeURIComponent(name)}`, body);
    if (state.filterTag === name && body.name) state.filterTag = body.name;
  } catch (err) {
    toast(errorText(err));
  }
  await load();
}

async function deleteTag(name) {
  if (!confirm(t('confirmDeleteTag', { name }))) return;
  try {
    await api('DELETE', `/rx/api/tags/${encodeURIComponent(name)}`);
  } catch (err) {
    toast(errorText(err));
  }
  await load();
}

async function createTag(e) {
  e.preventDefault();
  const form = e.target;
  const name = form.elements.name.value.trim();
  if (!name) return;
  try {
    await api('POST', '/rx/api/tags', { name, color: hexToArgb(form.elements.color.value) });
    form.elements.name.value = '';
  } catch (err) {
    toast(errorText(err));
  }
  await load();
}

/* ---------- Views ---------- */

let refreshTimer;

function showLogin(message) {
  state.token = null;
  storage.set('rustdex.token', null);
  clearInterval(refreshTimer);
  for (const d of document.querySelectorAll('dialog[open]')) d.close();
  $('#app-view').hidden = true;
  $('#login-view').hidden = false;
  showError($('#login-error'), message || '');
  $('#login-form').elements.username.focus();
}

async function showApp() {
  $('#login-view').hidden = true;
  $('#app-view').hidden = false;
  try {
    const info = await api('GET', '/rx/api/info');
    $('#version').textContent = info.version;
    $('#book-name').textContent = info.bookName;
    document.title = info.bookName === 'RustDex' ? 'RustDex' : `${info.bookName} · RustDex`;
    await load();
  } catch (err) {
    if (err.status !== 401) toast(errorText(err));
    return;
  }
  // Keep the online status fresh while the page is visible.
  clearInterval(refreshTimer);
  refreshTimer = setInterval(() => {
    if (document.visibilityState === 'visible' && !document.querySelector('dialog[open]')) load().catch(() => {});
  }, 30000);
}

function applyI18n() {
  for (const node of document.querySelectorAll('[data-i18n]')) node.textContent = t(node.dataset.i18n);
  for (const node of document.querySelectorAll('[data-i18n-placeholder]')) node.placeholder = t(node.dataset.i18nPlaceholder);
  for (const node of document.querySelectorAll('[data-i18n-title]')) {
    node.title = t(node.dataset.i18nTitle);
    node.setAttribute('aria-label', node.title);
  }
}

function init() {
  applyI18n();
  $('#pw-copy').replaceChildren(icon('copy'));

  $('#login-form').addEventListener('submit', async (e) => {
    e.preventDefault();
    const form = e.target;
    try {
      const data = await api('POST', '/api/login', {
        username: form.elements.username.value,
        password: form.elements.password.value,
      });
      state.token = data.access_token;
      storage.set('rustdex.token', state.token);
      form.reset();
      showApp();
    } catch (err) {
      showError($('#login-error'), errorText(err));
    }
  });

  $('#btn-logout').addEventListener('click', async () => {
    try { await api('POST', '/api/logout'); } catch { /* ignore */ }
    showLogin();
  });

  $('#search').addEventListener('input', (e) => { state.query = e.target.value; render(); });
  document.addEventListener('keydown', (e) => {
    if (e.key === '/' && !document.querySelector('dialog[open]') && document.activeElement.tagName !== 'INPUT') {
      e.preventDefault();
      $('#search').focus();
    }
  });

  for (const th of document.querySelectorAll('th[data-sort]')) {
    th.addEventListener('click', () => {
      const key = th.dataset.sort;
      state.sort = { key, dir: state.sort.key === key ? -state.sort.dir : 1 };
      render();
    });
  }

  $('#btn-new').addEventListener('click', () => openPeer(null));
  $('#btn-tags').addEventListener('click', () => { renderTagsManager(); $('#tags-dialog').showModal(); });
  $('#tags-close').addEventListener('click', () => $('#tags-dialog').close());
  $('#tag-new').addEventListener('submit', createTag);

  $('#peer-form').addEventListener('submit', savePeer);
  $('#btn-cancel').addEventListener('click', () => $('#peer-dialog').close());
  $('#btn-delete').addEventListener('click', deletePeer);
  $('#peer-password').addEventListener('input', () => { state.pwDirty = true; });
  $('#pw-toggle').addEventListener('click', async () => {
    const input = $('#peer-password');
    const show = input.type === 'password';
    if (show) {
      try { await loadPassword(); } catch (err) { toast(errorText(err)); return; }
    }
    setPasswordVisible(show);
  });
  $('#pw-copy').addEventListener('click', async () => {
    try { await loadPassword(); } catch (err) { toast(errorText(err)); return; }
    copyText($('#peer-password').value);
  });
  $('#tag-input').addEventListener('keydown', (e) => {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault();
      addFormTag();
    } else if (e.key === 'Backspace' && !e.target.value && state.formTags.length) {
      state.formTags.pop();
      renderFormTags();
    }
  });
  $('#tag-input').addEventListener('change', (e) => {
    // Picking a suggestion from the datalist adds it straight away.
    if (state.tags.some((x) => x.name === e.target.value)) addFormTag();
  });

  if (state.token) showApp();
  else showLogin();
}

init();
