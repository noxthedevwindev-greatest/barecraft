(() => {
  'use strict';

  const BASE = location.href.replace(/[^/]*$/, '');
  const MAGIC = '__BARECRAFT_CONFIG__='; // unused; overlay uses binary magic
  // binary overlay magic must match Go: "BRCFG1\x00" + u64 LE length + cfg bytes
  const OVERLAY_MAGIC = 'BRCFG1\x00';

  const state = {
    name: '',
    player: 'Player',
    mc: '',
    loader: 'vanilla',
    loaderV: '',
    ram: 4096,
    mods: [], // modrinth slugs
  };

  // ---------- helpers ----------
  const $ = (s) => document.querySelector(s);
  const $$ = (s) => Array.from(document.querySelectorAll(s));

  function toast(msg) {
    const t = $('#toast');
    t.textContent = msg;
    t.classList.add('show');
    clearTimeout(toast._t);
    toast._t = setTimeout(() => t.classList.remove('show'), 1400);
  }

  async function copyText(s) {
    try { await navigator.clipboard.writeText(s); toast('copied'); }
    catch (e) {
      const ta = document.createElement('textarea');
      ta.value = s; document.body.appendChild(ta); ta.select();
      document.execCommand('copy'); ta.remove(); toast('copied');
    }
  }

  function b64url(str) {
    const bytes = new TextEncoder().encode(str);
    let bin = '';
    for (const b of bytes) bin += String.fromCharCode(b);
    return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
  }

  function buildCfg() {
    const cfg = {
      n: state.name.trim() || autoName(),
      p: state.player.trim() || 'Player',
      mc: state.mc,
      ld: state.loader,
      ram: state.ram,
    };
    if (state.loader !== 'vanilla' && state.loaderV.trim()) cfg.lv = state.loaderV.trim();
    if (state.mods.length) cfg.mods = state.mods.slice();
    return cfg;
  }

  function autoName() {
    const L = state.loader === 'vanilla' ? '' : ` (${state.loader})`;
    return `Minecraft ${state.mc}${L}`;
  }

  function slugify(s) {
    return (s || '').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 48) || 'instance';
  }

  function cfgB64() { return b64url(JSON.stringify(buildCfg())); }

  function cfgSlug() { return slugify(buildCfg().n); }

  // ---------- versions ----------
  async function loadVersions() {
    let list = null;
    // try live manifest first (CORS is open on piston-meta)
    try {
      const r = await fetch('https://piston-meta.mojang.com/mc/game/version_manifest_v2.json');
      if (r.ok) {
        const j = await r.json();
        list = j.versions || [];
      }
    } catch (e) { /* fall back */ }
    if (!list) {
      try {
        const r = await fetch(BASE + 'versions.json');
        list = await r.json();
      } catch (e) { list = []; }
    }
    const sel = $('#mc');
    sel.innerHTML = '';
    const releases = list.filter(v => v.type === 'release');
    const snaps = list.filter(v => v.type === 'snapshot');
    // releases: newest first (manifest is already newest-first)
    const relGroup = document.createElement('optgroup');
    relGroup.label = 'releases';
    releases.forEach(v => {
      const o = document.createElement('option');
      o.value = v.id; o.textContent = v.id;
      relGroup.appendChild(o);
    });
    sel.appendChild(relGroup);
    const snapGroup = document.createElement('optgroup');
    snapGroup.label = 'snapshots (recent)';
    snaps.slice(0, 40).forEach(v => {
      const o = document.createElement('option');
      o.value = v.id; o.textContent = v.id;
      snapGroup.appendChild(o);
    });
    sel.appendChild(snapGroup);

    // default: latest release
    if (releases.length) state.mc = releases[0].id;
    sel.value = state.mc;
    sel.addEventListener('change', () => { state.mc = sel.value; refreshLoaderAvailability(); render(); });
  }

  // ---------- support (which loaders exist for which MC) ----------
  let support = { fabric: [], forge: [], neoforge: [] };
  async function loadSupport() {
    try {
      const r = await fetch(BASE + 'support.json');
      if (r.ok) support = await r.json();
    } catch (e) { /* leave empty => all enabled */ }
  }

  function loaderSupported(loader, mc) {
    if (loader === 'vanilla') return true;
    const arr = support[loader];
    if (!arr || !arr.length) return true; // unknown => allow, setup will error
    return arr.indexOf(mc) !== -1;
  }

  // ---------- loader buttons ----------
  function refreshLoaderAvailability() {
    $$('.loader-btn').forEach(btn => {
      const l = btn.dataset.loader;
      const ok = loaderSupported(l, state.mc);
      btn.classList.toggle('disabled', !ok);
      btn.title = ok ? '' : `no ${l} build for Minecraft ${state.mc}`;
    });
    if (!loaderSupported(state.loader, state.mc)) {
      state.loader = 'vanilla';
      $$('.loader-btn').forEach(b => b.classList.toggle('active', b.dataset.loader === state.loader));
      $('#loaderVField').style.display = 'none';
      $('#modsField').style.display = state.loader === 'vanilla' ? 'none' : 'block';
      $('#modHint').textContent = state.loader === 'vanilla'
        ? 'pick a loader to add mods. vanilla + mods isn\'t a thing.'
        : 'search modrinth. results filtered to this version + loader.';
      $('#modSearch').disabled = state.loader === 'vanilla';
    }
  }

  // ---------- mods ----------
  let searchTimer = null;
  async function searchMods(q) {
    if (!q || q.length < 2) { closeResults(); return; }
    const facets = [['project_type:mod'], [`versions:${state.mc}`]];
    if (state.loader !== 'vanilla') facets.push([`categories:${state.loader}`]);
    const url = 'https://api.modrinth.com/v2/search?query=' + encodeURIComponent(q) +
      '&facets=' + encodeURIComponent(JSON.stringify(facets)) + '&limit=8';
    let data;
    try { const r = await fetch(url); if (!r.ok) throw 0; data = await r.json(); }
    catch (e) { return; }
    const box = $('#modResults');
    box.innerHTML = '';
    (data.hits || []).forEach(h => {
      const row = document.createElement('div');
      row.className = 'mod-result';
      row.innerHTML = `
        <img src="${h.icon_url || ''}" alt="" onerror="this.style.visibility='hidden'">
        <div>
          <div class="title">${escapeHtml(h.title)}</div>
          <div class="slug">${escapeHtml(h.slug)}${h.author ? ' · ' + escapeHtml(h.author) : ''}</div>
        </div>`;
      row.addEventListener('click', () => { addMod(h.slug); closeResults(); $('#modSearch').value = ''; });
      box.appendChild(row);
    });
    box.classList.toggle('open', (data.hits || []).length > 0);
  }

  function closeResults() { $('#modResults').classList.remove('open'); }

  function addMod(slug) {
    if (state.mods.indexOf(slug) !== -1) return;
    state.mods.push(slug);
    renderChips();
    render();
  }

  function removeMod(slug) {
    state.mods = state.mods.filter(s => s !== slug);
    renderChips();
    render();
  }

  function renderChips() {
    const c = $('#chips');
    c.innerHTML = '';
    state.mods.forEach(s => {
      const chip = document.createElement('span');
      chip.className = 'chip';
      chip.innerHTML = `${escapeHtml(s)} <button title="remove" aria-label="remove ${escapeHtml(s)}">×</button>`;
      chip.querySelector('button').addEventListener('click', () => removeMod(s));
      c.appendChild(chip);
    });
  }

  function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  }

  // ---------- render ----------
  function render() {
    state.name = $('#name').value;
    state.player = $('#player').value;
    state.ram = parseInt($('#ram').value, 10) || 4096;
    state.loaderV = $('#loaderV').value;

    const cfg = buildCfg();
    const b64 = cfgB64();
    const slug = cfgSlug();

    // summary
    $('#summary').textContent =
      `${cfg.n}  ·  MC ${state.mc}  ·  ${state.loader}` +
      (state.loader !== 'vanilla' ? `  ·  ${state.ram/1024}GB` : `  ·  ${state.ram/1024}GB`) +
      (state.mods.length ? `  ·  ${state.mods.length} mod(s)` : '');

    // lines
    $('#line-windows').textContent =
      `curl.exe -fsSL "${BASE}bin/barecraft-windows-amd64.exe" -o $env:TEMP\\barecraft.exe; & $env:TEMP\\barecraft.exe ${b64}`;
    $('#line-linux').textContent =
      `a=$(uname -m); case $a in x86_64) a=amd64;; aarch64) a=arm64;; esac; curl -fsSL "${BASE}bin/barecraft-linux-$a" -o /tmp/barecraft && chmod +x /tmp/barecraft && /tmp/barecraft ${b64}`;
    $('#line-linux-run').textContent =
      `chmod +x ~/Downloads/${slug}-setup && ~/Downloads/${slug}-setup`;
    $('#line-macos').textContent =
      `a=$(uname -m); [ "$a" = arm64 ] || a=amd64; curl -fsSL "${BASE}bin/barecraft-darwin-$a" -o /tmp/barecraft && chmod +x /tmp/barecraft && /tmp/barecraft ${b64}`;
    $('#line-mac-run').textContent =
      `xattr -dr com.apple.quarantine ~/Downloads/${slug}-setup && chmod +x ~/Downloads/${slug}-setup && ~/Downloads/${slug}-setup`;
  }

  // ---------- download (config baked into the binary via overlay) ----------
  async function download(platform) {
    let binName, fileName, arch;
    if (platform === 'windows') {
      binName = 'barecraft-windows-amd64.exe';
      fileName = `${cfgSlug()}-setup.exe`;
    } else if (platform === 'macos') {
      arch = await macArch();
      binName = `barecraft-darwin-${arch}`;
      fileName = `${cfgSlug()}-setup`;
    } else {
      arch = linuxArchFromUA();
      binName = `barecraft-linux-${arch}`;
      fileName = `${cfgSlug()}-setup`;
    }
    const cfg = cfgB64();
    let buf;
    try {
      const r = await fetch(BASE + 'bin/' + binName);
      if (!r.ok) throw new Error('HTTP ' + r.status);
      buf = new Uint8Array(await r.arrayBuffer());
    } catch (e) {
      toast('download failed: ' + e.message);
      return;
    }
    // overlay: cfg bytes + u64 LE length + magic "BRCFG1\0"
    const cfgBytes = new TextEncoder().encode(cfg);
    const out = new Uint8Array(buf.length + cfgBytes.length + 8 + 7);
    out.set(buf, 0);
    out.set(cfgBytes, buf.length);
    const lenOff = buf.length + cfgBytes.length;
    const dv = new DataView(out.buffer);
    dv.setBigUint64(lenOff, BigInt(cfgBytes.length), true);
    const magic = new TextEncoder().encode('BRCFG1\x00');
    out.set(magic, lenOff + 8);
    const blob = new Blob([out], { type: 'application/octet-stream' });
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = fileName;
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(a.href), 10000);
    toast('downloaded ' + fileName);
  }

  function linuxArchFromUA() {
    const ua = navigator.userAgent || '';
    if (/aarch64|arm64/i.test(ua)) return 'arm64';
    return 'amd64';
  }

  async function macArch() {
    try {
      if (navigator.userAgentData && navigator.userAgentData.getHighEntropyValues) {
        const v = await navigator.userAgentData.getHighEntropyValues(['architecture']);
        const a = (v.architecture || '').toLowerCase();
        if (a === 'arm' || a === 'arm64' || a === 'aarch64') return 'arm64';
        if (a === 'x86' || a === 'x86_64' || a === 'amd64') return 'amd64';
      }
    } catch (e) {}
    const ua = navigator.userAgent || '';
    // Safari on Apple Silicon still reports Intel in many UAs; default to arm64
    // for modern macs but let the user override via the terminal line if wrong.
    if (/Intel Mac OS X/i.test(ua) && !/AppleWebKit.*Version\/1[6-9]/i.test(ua)) {
      // heuristic: older Safari on Intel
    }
    return 'arm64';
  }

  // ---------- init ----------
  async function init() {
    await Promise.all([loadVersions(), loadSupport()]);
    refreshLoaderAvailability();
    render();

    // loader buttons
    $$('.loader-btn').forEach(btn => {
      btn.addEventListener('click', () => {
        if (btn.classList.contains('disabled')) return;
        state.loader = btn.dataset.loader;
        $$('.loader-btn').forEach(b => b.classList.toggle('active', b === btn));
        $('#loaderVField').style.display = state.loader === 'vanilla' ? 'none' : 'block';
        const modsOn = state.loader !== 'vanilla';
        $('#modsField').style.display = modsOn ? 'block' : 'none';
        $('#modHint').textContent = modsOn
          ? 'search modrinth. results filtered to this version + loader.'
          : 'pick a loader to add mods. vanilla + mods isn\'t a thing.';
        $('#modSearch').disabled = !modsOn;
        if (!modsOn) { state.mods = []; renderChips(); }
        refreshLoaderAvailability();
        render();
      });
    });
    // default active
    $$('.loader-btn')[0].classList.add('active');

    // inputs
    ['name','player','ram','loaderV'].forEach(id => {
      const el = $('#' + id);
      el.addEventListener('input', () => {
        if (id === 'name') state.name = el.value;
        if (id === 'player') state.player = el.value;
        if (id === 'ram') state.ram = parseInt(el.value, 10) || 4096;
        if (id === 'loaderV') state.loaderV = el.value;
        render();
      });
    });

    // mod search
    const ms = $('#modSearch');
    ms.addEventListener('input', () => {
      clearTimeout(searchTimer);
      const q = ms.value.trim();
      if (q.length < 2) { closeResults(); return; }
      searchTimer = setTimeout(() => searchMods(q), 300);
    });
    ms.addEventListener('blur', () => setTimeout(closeResults, 150));
    document.addEventListener('click', (e) => {
      if (!e.target.closest('.mod-search-wrap')) closeResults();
    });

    // tabs
    $$('.tab').forEach(t => {
      t.addEventListener('click', () => {
        $$('.tab').forEach(x => x.classList.toggle('active', x === t));
        $$('.platform').forEach(p => p.classList.toggle('active', p.id === 'plat-' + t.dataset.tab));
      });
    });

    // download buttons
    $('#dl-windows').addEventListener('click', () => download('windows'));
    $('#dl-macos').addEventListener('click', () => download('macos'));
    $('#dl-linux').addEventListener('click', () => download('linux'));

    // copy buttons
    $$('[data-copy]').forEach(b => {
      b.addEventListener('click', () => copyText(document.getElementById(b.dataset.copy).textContent));
    });

    // re-render on mc change handled in loadVersions
    render();
  }

  init();
})();
