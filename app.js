(() => {
  'use strict';

  // repo root = site root (Pages serves from the repo root)
  const BASE = location.href.replace(/[^/]*$/, '');
  // binary overlay: [cfg bytes][u64 LE length]["BRCFG1\0"] — must match Go
  const OVERLAY_MAGIC = 'BRCFG1\x00';

  const state = {
    name: '',
    player: 'Player',
    mc: '',
    loader: 'vanilla',
    loaderV: '',
    ram: 4096,
    mods: [], // {slug, title, icon, bad}
  };

  // ---------- helpers ----------
  const $ = (s) => document.querySelector(s);
  const $$ = (s) => Array.from(document.querySelectorAll(s));

  function toast(msg) {
    const t = $('#toast');
    t.textContent = msg;
    t.classList.add('show');
    clearTimeout(toast._t);
    toast._t = setTimeout(() => t.classList.remove('show'), 1600);
  }

  async function copyText(s) {
    try { await navigator.clipboard.writeText(s); toast('copied'); }
    catch (e) {
      const ta = document.createElement('textarea');
      ta.value = s;
      document.body.appendChild(ta);
      ta.select();
      document.execCommand('copy');
      ta.remove();
      toast('copied');
    }
  }

  function b64url(str) {
    const bytes = new TextEncoder().encode(str);
    let bin = '';
    for (const b of bytes) bin += String.fromCharCode(b);
    return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
  }

  function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, (c) => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
    }[c]));
  }

  function fmtN(n) {
    if (n >= 1e6) return (n / 1e6).toFixed(1).replace(/\.0$/, '') + 'M';
    if (n >= 1e3) return (n / 1e3).toFixed(1).replace(/\.0$/, '') + 'k';
    return String(n);
  }

  function slugify(s) {
    return (s || '').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 48) || 'instance';
  }

  function autoName() {
    const L = state.loader === 'vanilla' ? '' : ` (${state.loader})`;
    return `Minecraft ${state.mc}${L}`;
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
    if (state.mods.length) cfg.mods = state.mods.map((m) => m.slug);
    return cfg;
  }

  const cfgB64 = () => b64url(JSON.stringify(buildCfg()));
  const cfgSlug = () => slugify(buildCfg().n);

  // ---------- mc versions ----------
  function fillVersions(list) {
    const sel = $('#mc');
    const releases = list.filter((v) => v.type === 'release');
    const snaps = list.filter((v) => v.type === 'snapshot');
    sel.innerHTML = '';
    const g1 = document.createElement('optgroup');
    g1.label = 'releases';
    releases.forEach((v) => {
      const o = document.createElement('option');
      o.value = v.id;
      o.textContent = v.id;
      g1.appendChild(o);
    });
    sel.appendChild(g1);
    const g2 = document.createElement('optgroup');
    g2.label = 'snapshots (recent)';
    snaps.slice(0, 40).forEach((v) => {
      const o = document.createElement('option');
      o.value = v.id;
      o.textContent = v.id;
      g2.appendChild(o);
    });
    sel.appendChild(g2);
    // keep the current choice if it still exists (incl. snapshots), else newest release
    const ids = new Set(list.map((v) => v.id));
    if (!ids.has(state.mc) && releases.length) state.mc = releases[0].id;
    sel.value = state.mc;
    render(); // badges/lines recompute with the version known
  }

  async function loadVersions() {
    // baked same-origin list first: fills the select instantly so the UI is
    // usable even when mojang's manifest is slow, blocked or CORS-less
    try { fillVersions(await (await fetch(BASE + 'versions.json')).json()); } catch (e) { /* cached page */ }
    // then refresh from the live manifest (timeout-guarded), preserving the pick
    try {
      const ctl = new AbortController();
      const timer = setTimeout(() => ctl.abort(), 4000);
      const r = await fetch('https://piston-meta.mojang.com/mc/game/version_manifest_v2.json', { signal: ctl.signal });
      clearTimeout(timer);
      if (r.ok) fillVersions((await r.json()).versions || []);
    } catch (e) { /* baked list stands */ }
    $('#mc').addEventListener('change', () => {
      state.mc = $('#mc').value;
      refreshLoaders();
      scheduleCompatCheck();
      render();
    });
  }

  // ---------- loader support matrix ----------
  let support = { fabric: [], forge: [], neoforge: [] };
  async function loadSupport() {
    try {
      const r = await fetch(BASE + 'support.json');
      if (r.ok) support = await r.json();
    } catch (e) { /* unknown => allow, setup reports errors */ }
  }

  function loaderSupported(loader, mc) {
    if (loader === 'vanilla') return true;
    const arr = support[loader];
    if (!arr || !arr.length) return true;
    return arr.indexOf(mc) !== -1;
  }

  function refreshLoaders() {
    $$('.loader-btn').forEach((btn) => {
      const l = btn.dataset.loader;
      const ok = loaderSupported(l, state.mc);
      btn.disabled = !ok;
      btn.title = ok ? '' : `no ${l} build for Minecraft ${state.mc}`;
    });
    if (!loaderSupported(state.loader, state.mc)) setLoader('vanilla');
  }

  function setLoader(l) {
    state.loader = l;
    $$('.loader-btn').forEach((b) => b.classList.toggle('on', b.dataset.loader === l));
    const modded = l !== 'vanilla';
    $('#loaderVField').style.display = modded ? '' : 'none';
    $('#modsHeader').style.display = modded ? '' : 'none';
    $('#modsWrap').style.display = modded ? '' : 'none';
    $('#vanillaHint').style.display = modded ? 'none' : '';
    if (!modded) {
      state.mods = [];
      renderChips();
      closeResults();
    }
  }

  // ---------- mod search ----------
  let searchTimer = null;
  let resultRows = [];
  let kbIndex = -1;

  function closeResults() {
    $('#modResults').classList.remove('open');
    kbIndex = -1;
  }

  function fmtFacets() {
    const facets = [['project_type:mod'], [`versions:${state.mc}`]];
    if (state.loader !== 'vanilla') facets.push([`categories:${state.loader}`]);
    return facets;
  }

  async function searchMods(q) {
    if (!q || q.length < 2) { closeResults(); return; }
    const url = 'https://api.modrinth.com/v2/search?query=' + encodeURIComponent(q) +
      '&facets=' + encodeURIComponent(JSON.stringify(fmtFacets())) + '&limit=8';
    let data;
    try {
      const r = await fetch(url);
      if (!r.ok) throw 0;
      data = await r.json();
    } catch (e) {
      const box = $('#modResults');
      box.innerHTML = '<div class="mod-empty">search failed — offline?</div>';
      box.classList.add('open');
      return;
    }
    const box = $('#modResults');
    const hits = data.hits || [];
    box.innerHTML = '';
    resultRows = [];

    if (!hits.length) {
      box.innerHTML = '<div class="mod-empty">no mods match</div>';
      box.classList.add('open');
      return;
    }

    hits.forEach((h) => {
      const row = document.createElement('div');
      row.className = 'mod-result';
      row.setAttribute('role', 'option');
      row.innerHTML = `
        <img src="${escapeHtml(h.icon_url || '')}" alt="" onerror="this.style.visibility='hidden'">
        <div class="mi">
          <div class="title">${escapeHtml(h.title)}</div>
          <div class="sub">${escapeHtml(h.slug)}${h.author ? ' · ' + escapeHtml(h.author) : ''}</div>
        </div>
        <div class="dl">▼ ${fmtN(h.downloads || 0)}</div>`;
      row.addEventListener('click', () => {
        addMod(h.slug, h.title, h.icon_url);
        closeResults();
        $('#modSearch').value = '';
      });
      box.appendChild(row);
      resultRows.push(row);
    });
    box.classList.add('open');
  }

  function addMod(slug, title, icon) {
    if (state.mods.some((m) => m.slug === slug)) return;
    state.mods.push({ slug, title, icon: icon || '', bad: null });
    renderChips();
    scheduleCompatCheck();
    render();
  }

  function removeMod(slug) {
    state.mods = state.mods.filter((m) => m.slug !== slug);
    renderChips();
    render();
  }

  function renderChips() {
    const c = $('#chips');
    c.innerHTML = '';
    state.mods.forEach((m) => {
      const chip = document.createElement('span');
      chip.className = 'chip' + (m.bad ? ' bad' : '');
      chip.title = m.bad || m.slug;
      chip.innerHTML = `
        ${m.icon ? `<img src="${escapeHtml(m.icon)}" alt="" onerror="this.remove()">` : ''}
        <span class="mname">${escapeHtml(m.title)}</span>
        <button class="rm" title="remove ${escapeHtml(m.title)}" aria-label="remove ${escapeHtml(m.title)}">×</button>`;
      chip.querySelector('.rm').addEventListener('click', () => removeMod(m.slug));
      c.appendChild(chip);
    });
  }

  // ---------- mod compat flags ----------
  let compatTimer = null;
  function scheduleCompatCheck() {
    clearTimeout(compatTimer);
    compatTimer = setTimeout(checkCompat, 350);
  }

  async function checkCompat() {
    if (state.loader === 'vanilla' || !state.mods.length) return;
    const ids = state.mods.map((m) => m.slug);
    const url = 'https://api.modrinth.com/v2/projects?ids=' + encodeURIComponent(JSON.stringify(ids));
    let data;
    try {
      const r = await fetch(url);
      if (!r.ok) return;
      data = await r.json();
    } catch (e) { return; }
    const bySlug = {};
    data.forEach((p) => { bySlug[p.slug] = p; });
    state.mods.forEach((m) => {
      const p = bySlug[m.slug];
      if (!p) { m.bad = null; return; }
      const gv = p.game_versions || [];
      const ld = (p.loaders || []).map((x) => String(x).toLowerCase());
      if (!gv.includes(state.mc)) m.bad = `no Minecraft ${state.mc} build`;
      else if (state.loader !== 'vanilla' && !ld.includes(state.loader)) m.bad = `no ${state.loader} build — needs ${ld.join('/') || 'other loader'}`;
      else m.bad = null;
    });
    renderChips();
  }

  // ---------- render ----------
  function render() {
    state.name = $('#name').value;
    state.player = $('#player').value;
    state.loaderV = $('#loaderV').value;

    const cfg = buildCfg();
    const b64 = cfgB64();
    const slug = cfgSlug();

    // summary badges
    const badges = $('#summary');
    const parts = [
      [cfg.n, true],
      ['MC ' + state.mc, false],
      [state.loader, false],
      [(state.ram / 1024) + ' GB', false],
    ];
    if (state.mods.length) parts.push([state.mods.length + (state.mods.length === 1 ? ' mod' : ' mods'), true]);
    badges.innerHTML = parts
      .map(([t, hl]) => `<span class="${hl ? 'nb' : ''}">${escapeHtml(t)}</span>`)
      .join('');

    // terminal / run lines (kept short — the helper script does arch+os pickup)
    $('#line-windows').textContent =
      `$u='https://noxthedevwindev-greatest.github.io/barecraft/bin'\ncurl.exe -fsSL "$u/barecraft-windows-amd64.exe" -o b.exe; ./b.exe ${b64}`;
    $('#line-linux').textContent =
      `curl -fsSL "${BASE}i.sh" | bash -s -- ${b64}`;
    $('#line-linux-run').textContent =
      `chmod +x ~/Downloads/${slug}-setup && ~/Downloads/${slug}-setup`;
    $('#line-macos').textContent =
      `curl -fsSL "${BASE}i.sh" | bash -s -- ${b64}`;
    $('#line-mac-run').textContent =
      `xattr -dr com.apple.quarantine ~/Downloads/${slug}-setup && chmod +x ~/Downloads/${slug}-setup && ~/Downloads/${slug}-setup`;
  }

  // ---------- download (config baked into the binary via overlay) ----------
  function linuxArchFromUA() {
    return /aarch64|arm64/i.test(navigator.userAgent || '') ? 'arm64' : 'amd64';
  }

  async function macArch() {
    try {
      if (navigator.userAgentData && navigator.userAgentData.getHighEntropyValues) {
        const v = await navigator.userAgentData.getHighEntropyValues(['architecture']);
        const a = (v.architecture || '').toLowerCase();
        if (/^(arm|arm64|aarch64)$/.test(a)) return 'arm64';
        if (/^(x86|x86_64|amd64)$/.test(a)) return 'amd64';
      }
    } catch (e) { /* fall through */ }
    // Safari on Apple Silicon often reports Intel — default to arm64 (the
    // terminal one-liner uses uname -m and is always correct)
    return 'arm64';
  }

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

    const btn = $('#dl-' + platform);
    if (btn.dataset.busy) return;
    btn.dataset.busy = '1';
    const orig = btn.innerHTML;
    btn.innerHTML = '<span class="spin"></span> packaging…';
    btn.disabled = true;

    try {
      const r = await fetch(BASE + 'bin/' + binName);
      if (!r.ok) throw new Error('HTTP ' + r.status);
      const buf = new Uint8Array(await r.arrayBuffer());
      const cfgBytes = new TextEncoder().encode(cfgB64());
      const out = new Uint8Array(buf.length + cfgBytes.length + 8 + 7);
      out.set(buf, 0);
      out.set(cfgBytes, buf.length);
      const lenOff = buf.length + cfgBytes.length;
      new DataView(out.buffer).setBigUint64(lenOff, BigInt(cfgBytes.length), true);
      out.set(new TextEncoder().encode(OVERLAY_MAGIC), lenOff + 8);

      const blob = new Blob([out], { type: 'application/octet-stream' });
      const a = document.createElement('a');
      a.href = URL.createObjectURL(blob);
      a.download = fileName;
      document.body.appendChild(a);
      a.click();
      a.remove();
      setTimeout(() => URL.revokeObjectURL(a.href), 10000);
      toast('downloaded ' + fileName);
    } catch (e) {
      toast('download failed: ' + e.message);
    } finally {
      btn.dataset.busy = '';
      btn.innerHTML = orig;
      btn.disabled = false;
    }
  }

  // ---------- init ----------
  async function init() {
    // random splash, and render immediately so the page never sits half-empty
    // while the version/support fetches finish
    const SPLASHES = [
      'NO LAUNCHER!', 'ALSO TRY PRISM!', 'JUST JAVA, DUDE!', 'DOUBLE-CLICK IT!',
      'RUNS ON YOUR POTATO!', 'NOW WITH MORE COBBLE!', 'ZERO GUIS USED!',
      'OFFLINE MODE!', 'EMERALDS NOT INCLUDED!', 'MOJANG-PROOF!',
    ];
    const splash = $('#splash');
    if (splash) splash.textContent = SPLASHES[(Math.random() * SPLASHES.length) | 0];
    render();

    await Promise.all([loadVersions(), loadSupport()]);
    setLoader('vanilla');
    refreshLoaders();
    render();

    // loader buttons
    $$('.loader-btn').forEach((btn) => {
      btn.addEventListener('click', () => {
        if (btn.disabled) return;
        setLoader(btn.dataset.loader);
        scheduleCompatCheck();
        render();
      });
    });

    // inputs
    ['name', 'player', 'loaderV'].forEach((id) => {
      $('#' + id).addEventListener('input', render);
    });

    // ram segmented control
    $$('#ramSeg button').forEach((b) => {
      b.addEventListener('click', () => {
        $$('#ramSeg button').forEach((x) => x.classList.toggle('on', x === b));
        state.ram = parseInt(b.dataset.ram, 10) || 4096;
        render();
      });
    });

    // mod search
    const ms = $('#modSearch');
    ms.addEventListener('input', () => {
      clearTimeout(searchTimer);
      const q = ms.value.trim();
      if (q.length < 2) { closeResults(); return; }
      searchTimer = setTimeout(() => searchMods(q), 280);
    });
    ms.addEventListener('keydown', (e) => {
      if (!$('#modResults').classList.contains('open')) return;
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        e.preventDefault();
        if (!resultRows.length) return;
        kbIndex = e.key === 'ArrowDown'
          ? (kbIndex + 1) % resultRows.length
          : (kbIndex - 1 + resultRows.length) % resultRows.length;
        resultRows.forEach((r, i) => r.classList.toggle('kb', i === kbIndex));
        resultRows[kbIndex].scrollIntoView({ block: 'nearest' });
      } else if (e.key === 'Enter') {
        e.preventDefault();
        (resultRows[kbIndex] && resultRows[kbIndex].classList.contains('kb')
          ? resultRows[kbIndex]
          : resultRows[0])?.click();
        closeResults();
        ms.value = '';
      } else if (e.key === 'Escape') {
        closeResults();
      }
    });
    ms.addEventListener('blur', () => setTimeout(closeResults, 150));
    document.addEventListener('click', (e) => {
      if (!e.target.closest('.mod-search-wrap')) closeResults();
    });

    // tabs
    $$('.tab').forEach((t) => {
      t.addEventListener('click', () => {
        $$('.tab').forEach((x) => x.classList.toggle('on', x === t));
        $$('.platform').forEach((p) => p.classList.toggle('on', p.id === 'plat-' + t.dataset.tab));
      });
    });

    // download + copy
    $('#dl-windows').addEventListener('click', () => download('windows'));
    $('#dl-macos').addEventListener('click', () => download('macos'));
    $('#dl-linux').addEventListener('click', () => download('linux'));
    $$('[data-copy]').forEach((b) => {
      b.addEventListener('click', () => copyText(document.getElementById(b.dataset.copy).textContent));
    });

    render();
  }

  init();
})();
