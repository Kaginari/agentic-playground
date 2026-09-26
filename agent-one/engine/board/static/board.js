/* The board's page script: theme, live patching over SSE, inline-SVG charts and the team graph.
   Vanilla, no build step. Every chart draws into its grid column and redraws on resize. */
(function () {
  'use strict';
  var agent = document.body;
  var prefix = agent.getAttribute('data-prefix') || '';

  // ---- theme: follows the system unless the reader picked one
  var mq = matchMedia('(prefers-color-scheme: dark)');
  function saved() { try { return localStorage.getItem('board-theme'); } catch (e) { return null; } }
  function applyTheme() {
    var pick = saved();
    var t = pick === 'dark' || pick === 'light' ? pick : (mq.matches ? 'dark' : 'light');
    document.documentElement.setAttribute('data-bs-theme', t);
    var btn = document.getElementById('themeToggle');
    if (btn) btn.textContent = pick === 'dark' || pick === 'light' ? pick : 'auto';
    drawAll();
  }
  var toggle = document.getElementById('themeToggle');
  if (toggle) toggle.addEventListener('click', function () {
    var cur = saved(), next = cur === 'dark' ? 'light' : cur === 'light' ? null : 'dark';
    try { next ? localStorage.setItem('board-theme', next) : localStorage.removeItem('board-theme'); } catch (e) {}
    applyTheme();
  });
  if (mq.addEventListener) mq.addEventListener('change', applyTheme);

  // ---- helpers
  function svgEl(name, attrs) {
    var el = document.createElementNS('http://www.w3.org/2000/svg', name);
    for (var k in attrs) if (attrs[k] !== undefined) el.setAttribute(k, attrs[k]);
    return el;
  }
  function compact(n) {
    if (n >= 1e9) return (n / 1e9).toFixed(1) + 'B';
    if (n >= 1e6) return (n / 1e6).toFixed(1) + 'M';
    if (n >= 1e4) return (n / 1e3).toFixed(1) + 'K';
    return String(Math.round(n)).replace(/\B(?=(\d{3})+(?!\d))/g, ',');
  }
  function usd(v) { return v === 0 ? '$0.00' : v < 0.01 ? '$' + v.toFixed(4) : '$' + v.toFixed(2); }
  function niceMax(v) {
    if (v <= 0) return 1;
    var p = Math.pow(10, Math.floor(Math.log10(v)));
    var m = v / p;
    var step = m <= 1 ? 1 : m <= 2 ? 2 : m <= 5 ? 5 : 10;
    return step * p;
  }
  // gridline count that keeps the ticks round: 1→5 steps of .2, 2→4 of .5, 5→5 of 1
  function divisionsOf(max) { var lead = max / Math.pow(10, Math.floor(Math.log10(max))); return lead === 2 ? 4 : 5; }
  function jsonOf(id) {
    var el = document.getElementById(id);
    if (!el) return null;
    try { return JSON.parse(el.textContent); } catch (e) { return null; }
  }
  var tip = document.getElementById('tip');
  function showTip(html, ev) {
    if (!tip) return;
    tip.innerHTML = html; tip.hidden = false;
    var x = ev.clientX + 12, y = ev.clientY + 12;
    if (x + tip.offsetWidth > innerWidth - 8) x = ev.clientX - tip.offsetWidth - 12;
    if (y + tip.offsetHeight > innerHeight - 8) y = ev.clientY - tip.offsetHeight - 12;
    tip.style.left = x + 'px'; tip.style.top = y + 'px';
  }
  function hideTip() { if (tip) tip.hidden = true; }
  function esc(s) { return String(s == null ? '' : s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;'); }

  // ---- stacked columns: tokens by day (input · output · cache read · cache write)
  function drawColumns(el, d) {
    el.innerHTML = '';
    if (!d || !d.days || !d.days.length) { el.innerHTML = '<div class="text-secondary small">no day in range</div>'; return; }
    var W = Math.max(280, el.clientWidth || 600), H = 220, padL = 44, padR = 8, padT = 10, padB = 28;
    var svg = svgEl('svg', { viewBox: '0 0 ' + W + ' ' + H, role: 'img', 'aria-label': 'tokens by day' });
    var n = d.days.length, totals = [], max = 0;
    for (var i = 0; i < n; i++) { var t = 0; d.series.forEach(function (s) { t += s.values[i] || 0; }); totals.push(t); if (t > max) max = t; }
    max = niceMax(max);
    var plotW = W - padL - padR, plotH = H - padT - padB;
    var slot = plotW / n, bw = Math.min(24, slot * 0.6);
    var div = divisionsOf(max);
    for (var g = 0; g <= div; g++) {
      var y = padT + plotH - plotH * g / div;
      svg.appendChild(svgEl('line', { x1: padL, x2: W - padR, y1: y, y2: y, 'class': 'grid' }));
      var tx = svgEl('text', { x: padL - 6, y: y + 3, 'class': 'ax', 'text-anchor': 'end' }); tx.textContent = compact(max * g / div); svg.appendChild(tx);
    }
    var gap = 2;
    for (var i2 = 0; i2 < n; i2++) {
      var x = padL + slot * i2 + (slot - bw) / 2, yTop = padT + plotH;
      var group = svgEl('g', {});
      var segs = [];
      d.series.forEach(function (s) {
        var v = s.values[i2] || 0; if (!v) return;
        var h = plotH * v / max; yTop -= h;
        segs.push({ s: s, v: v, y: yTop, h: h });
      });
      segs.forEach(function (sg, j) {
        var top = j === segs.length - 1;
        var hh = Math.max(0, sg.h - (j === 0 ? 0 : gap));
        var r = top ? Math.min(4, hh / 2) : 0;
        var y0 = sg.y, x0 = x, w = bw;
        var pth = top
          ? 'M' + x0 + ',' + (y0 + hh) + ' v' + (-(hh - r)) + ' a' + r + ',' + r + ' 0 0 1 ' + r + ',' + (-r) + ' h' + (w - 2 * r) + ' a' + r + ',' + r + ' 0 0 1 ' + r + ',' + r + ' v' + (hh - r) + ' z'
          : 'M' + x0 + ',' + (y0 + hh) + ' v' + (-hh) + ' h' + w + ' v' + hh + ' z';
        group.appendChild(svgEl('path', { d: pth, 'class': 's-' + sg.s.key }));
      });
      var lbl = svgEl('text', { x: x + bw / 2, y: H - padB + 14, 'class': 'ax', 'text-anchor': 'middle' });
      lbl.textContent = n > 10 ? d.days[i2].slice(5) : d.days[i2]; group.appendChild(lbl);
      var hit = svgEl('rect', { x: padL + slot * i2, y: padT, width: slot, height: plotH, 'class': 'hit' });
      (function (idx) {
        hit.addEventListener('mousemove', function (ev) {
          var rows = d.series.map(function (s) { return '<div><span class="sw s-' + s.key + '" style="background:var(--c-' + (d.series.indexOf(s) + 1) + ')"></span> ' + esc(s.label) + ' <b>' + compact(s.values[idx] || 0) + '</b></div>'; }).join('');
          showTip('<div class="text-secondary">' + esc(d.days[idx]) + '</div>' + rows + '<div>total <b>' + compact(totals[idx]) + '</b>' + (d.usd ? ' · ' + usd(d.usd[idx] || 0) : '') + '</div>', ev);
        });
        hit.addEventListener('mouseleave', hideTip);
      })(i2);
      group.appendChild(hit);
      svg.appendChild(group);
    }
    el.appendChild(svg);
    var lg = document.createElement('div'); lg.className = 'legend small mt-1';
    d.series.forEach(function (s, i3) { lg.innerHTML += '<span class="legend-item"><span class="sw" style="background:var(--c-' + (i3 + 1) + ')"></span>' + esc(s.label) + '</span>'; });
    el.appendChild(lg);
  }

  // ---- single-series columns: cost by day
  function drawCost(el, d) {
    el.innerHTML = '';
    if (!d || !d.days || !d.days.length) { el.innerHTML = '<div class="text-secondary small">no day in range</div>'; return; }
    var W = Math.max(240, el.clientWidth || 400), H = 220, padL = 48, padR = 8, padT = 10, padB = 28;
    var svg = svgEl('svg', { viewBox: '0 0 ' + W + ' ' + H, role: 'img', 'aria-label': 'cost by day' });
    var n = d.days.length, max = 0;
    d.usd.forEach(function (v) { if (v > max) max = v; });
    max = niceMax(max);
    var plotW = W - padL - padR, plotH = H - padT - padB, slot = plotW / n, bw = Math.min(24, slot * 0.6);
    var div = divisionsOf(max);
    for (var g = 0; g <= div; g++) {
      var y = padT + plotH - plotH * g / div;
      svg.appendChild(svgEl('line', { x1: padL, x2: W - padR, y1: y, y2: y, 'class': 'grid' }));
      var tx = svgEl('text', { x: padL - 6, y: y + 3, 'class': 'ax', 'text-anchor': 'end' }); tx.textContent = usd(max * g / div); svg.appendChild(tx);
    }
    d.usd.forEach(function (v, i) {
      var h = plotH * v / max, x = padL + slot * i + (slot - bw) / 2, y0 = padT + plotH - h, r = Math.min(4, h / 2);
      var pth = 'M' + x + ',' + (y0 + h) + ' v' + (-(h - r)) + ' a' + r + ',' + r + ' 0 0 1 ' + r + ',' + (-r) + ' h' + (bw - 2 * r) + ' a' + r + ',' + r + ' 0 0 1 ' + r + ',' + r + ' v' + (h - r) + ' z';
      svg.appendChild(svgEl('path', { d: pth, 'class': 's-usd' }));
      var lbl = svgEl('text', { x: x + bw / 2, y: H - padB + 14, 'class': 'ax', 'text-anchor': 'middle' }); lbl.textContent = n > 6 ? d.days[i].slice(5) : d.days[i]; svg.appendChild(lbl);
      if (i === n - 1) { var vl = svgEl('text', { x: x + bw / 2, y: y0 - 4, 'class': 'lbl', 'text-anchor': 'middle' }); vl.textContent = usd(v); svg.appendChild(vl); }
      var hit = svgEl('rect', { x: padL + slot * i, y: padT, width: slot, height: plotH, 'class': 'hit' });
      hit.addEventListener('mousemove', function (ev) { showTip('<div class="text-secondary">' + esc(d.days[i]) + '</div><div>cost <b>' + usd(v) + '</b></div>', ev); });
      hit.addEventListener('mouseleave', hideTip);
      svg.appendChild(hit);
    });
    el.appendChild(svg);
  }

  // ---- the team graph: lanes left to right, skills before the rank they serve
  var teamState = { focus: null };
  function drawTeam(el, d) {
    el.innerHTML = '';
    if (!d || !d.nodes || !d.nodes.length) { el.innerHTML = '<div class="text-secondary small">no node to draw</div>'; return; }
    var lanes = (d.labels || []).map(function (l) { return l.id; });
    var byLane = {};
    d.nodes.forEach(function (n) { var k = n.kind === 'skill' ? 'skill:' + n.lane : n.lane; (byLane[k] = byLane[k] || []).push(n); });
    lanes = lanes.filter(function (l) { return byLane[l] && byLane[l].length; });
    if (!lanes.length) lanes = Object.keys(byLane);
    var W = Math.max(320, el.clientWidth || 800), narrow = W < 640;
    var labelOf = function (l) { var x = (d.labels || []).filter(function (x) { return x.id === l; })[0]; return x ? x.label : l; };
    lanes.forEach(function (l) { byLane[l].sort(function (a, b) { return a.name < b.name ? -1 : 1; }); });
    var pos = {}, H, svg;
    if (!narrow) {
      // wide: lanes are columns left to right, members stacked down each lane
      var cols = lanes.length, laneW = W / cols, rowH = 54, top = 28, maxRows = 1;
      lanes.forEach(function (l) { if (byLane[l].length > maxRows) maxRows = byLane[l].length; });
      H = top + maxRows * rowH + 16;
      svg = svgEl('svg', { viewBox: '0 0 ' + W + ' ' + H, role: 'img', 'aria-label': 'team graph' });
      lanes.forEach(function (l, i) {
        if (i % 2 === 1) svg.appendChild(svgEl('rect', { x: i * laneW, y: 0, width: laneW, height: H, 'class': 'lane-bg' }));
        var t = svgEl('text', { x: i * laneW + laneW / 2, y: 14, 'class': 'lane-lbl', 'text-anchor': 'middle' });
        t.textContent = labelOf(l); svg.appendChild(t);
        byLane[l].forEach(function (n, j) { pos[n.id] = { x: i * laneW + laneW / 2, y: top + j * rowH + rowH / 2, n: n }; });
      });
    } else {
      // narrow (a phone): lanes are rows top to bottom, members spread across the row
      var rowH2 = 66, y0 = 0;
      H = lanes.length * rowH2;
      svg = svgEl('svg', { viewBox: '0 0 ' + W + ' ' + H, role: 'img', 'aria-label': 'team graph' });
      lanes.forEach(function (l, i) {
        if (i % 2 === 1) svg.appendChild(svgEl('rect', { x: 0, y: y0, width: W, height: rowH2, 'class': 'lane-bg' }));
        var t = svgEl('text', { x: 8, y: y0 + 12, 'class': 'lane-lbl', 'text-anchor': 'start' });
        t.textContent = labelOf(l); svg.appendChild(t);
        var count = byLane[l].length;
        byLane[l].forEach(function (n, j) { pos[n.id] = { x: W * (j + 0.5) / count, y: y0 + 32, n: n }; });
        y0 += rowH2;
      });
    }
    var near = {};
    var edges = svgEl('g', {});
    d.edges.forEach(function (e) {
      var a = pos[e.from], b = pos[e.to]; if (!a || !b) return;
      (near[e.from] = near[e.from] || []).push(e.to); (near[e.to] = near[e.to] || []).push(e.from);
      var pth;
      if (narrow) {
        var dy = (b.y - a.y) / 2;
        pth = 'M' + a.x + ',' + a.y + ' C' + a.x + ',' + (a.y + dy) + ' ' + b.x + ',' + (b.y - dy) + ' ' + b.x + ',' + b.y;
      } else {
        var dx = (b.x - a.x) / 2;
        pth = 'M' + a.x + ',' + a.y + ' C' + (a.x + dx) + ',' + a.y + ' ' + (b.x - dx) + ',' + b.y + ' ' + b.x + ',' + b.y;
      }
      var p = svgEl('path', { d: pth, 'class': 'edge b-' + e.bond, 'data-from': e.from, 'data-to': e.to });
      var title = svgEl('title', {}); title.textContent = e.from + ' ' + e.bond + ' ' + e.to; p.appendChild(title);
      edges.appendChild(p);
    });
    svg.appendChild(edges);
    var nodes = svgEl('g', {});
    Object.keys(pos).forEach(function (id) {
      var p = pos[id], n = p.n, r = n.kind === 'skill' ? 7 : 11;
      var g = svgEl('g', { 'class': 'node ' + (n.kind === 'skill' ? 'lane-' + n.lane : 'rank-' + (n.rank || 'none')), 'data-id': id, tabindex: 0, role: 'button' });
      g.appendChild(svgEl('circle', { cx: p.x, cy: p.y, r: r, fill: 'var(--rank, var(--r-plain))' }));
      var t = svgEl('text', { x: p.x, y: p.y + r + 11 }); t.textContent = n.name.length > 18 && narrow ? n.name.slice(0, 16) + '…' : n.name; g.appendChild(t);
      var title = svgEl('title', {}); title.textContent = n.name + (n.doc ? ' · ' + n.doc : ''); g.appendChild(title);
      g.addEventListener('click', function () { focusNode(id, d, near); });
      g.addEventListener('keydown', function (ev) { if (ev.key === 'Enter' || ev.key === ' ') { ev.preventDefault(); focusNode(id, d, near); } });
      nodes.appendChild(g);
    });
    svg.appendChild(nodes);
    el.appendChild(svg);
    if (!teamState.focus && /^#node=/.test(location.hash)) {
      var want = decodeURIComponent(location.hash.slice(6));
      if (pos[want]) { focusNode(want, d, near); return; }
    }
    if (teamState.focus && pos[teamState.focus]) paintFocus(el, teamState.focus, near);
  }
  function paintFocus(el, id, near) {
    el.classList.add('focused');
    var set = {}; (near[id] || []).forEach(function (x) { set[x] = true; });
    el.querySelectorAll('.node').forEach(function (g) {
      var nid = g.getAttribute('data-id');
      g.classList.toggle('focus', nid === id); g.classList.toggle('near', !!set[nid]);
    });
    el.querySelectorAll('.edge').forEach(function (p) {
      p.classList.toggle('lit', p.getAttribute('data-from') === id || p.getAttribute('data-to') === id);
    });
  }
  function focusNode(id, d, near) {
    var el = document.getElementById('team');
    teamState.focus = id;
    paintFocus(el, id, near);
    var n = d.nodes.filter(function (x) { return x.id === id; })[0]; if (!n) return;
    var title = document.getElementById('nodeTitle'), meta = document.getElementById('nodeMeta'), list = document.getElementById('nodeNear'), doc = document.getElementById('nodeDoc');
    title.textContent = n.name;
    meta.innerHTML = esc(n.kind) + (n.rank ? ' · ' + esc(n.rank) : '') + (n.kind === 'skill' ? ' · lane ' + esc(n.lane) + (n.shared ? ' · shared' : '') : '') + (n.owns && n.owns.length ? '<br>owns ' + n.owns.map(esc).join(', ') : '') + (n.doc ? '<br><code>' + esc(n.doc) + '</code>' : '');
    list.innerHTML = '';
    d.edges.filter(function (e) { return e.from === id || e.to === id; }).forEach(function (e) {
      var other = e.from === id ? e.to : e.from;
      var li = document.createElement('li'); var a = document.createElement('a'); a.href = '#'; a.textContent = other;
      a.addEventListener('click', function (ev) { ev.preventDefault(); focusNode(other, d, near); });
      li.appendChild(document.createTextNode((e.from === id ? '→ ' : '← ') + e.bond + ' ')); li.appendChild(a); list.appendChild(li);
    });
    if (!list.children.length) list.innerHTML = '<li class="text-secondary">no bond — nothing names it</li>';
    doc.textContent = n.doc ? '…' : 'no doc on disk';
    if (n.doc) fetch(prefix + '/api/doc?path=' + encodeURIComponent(n.doc)).then(function (r) { return r.ok ? r.text() : 'doc not readable'; }).then(function (t) { doc.textContent = t; }).catch(function () { doc.textContent = 'doc not readable'; });
    var oc = document.getElementById('nodePanel');
    if (oc && window.bootstrap) bootstrap.Offcanvas.getOrCreateInstance(oc).show();
  }
  var panel = document.getElementById('nodePanel');
  if (panel) panel.addEventListener('hidden.bs.offcanvas', function () {
    teamState.focus = null; var el = document.getElementById('team'); if (el) el.classList.remove('focused');
  });

  // ---- draw everything on the page (called on load, resize, theme change and live patches)
  function drawAll() {
    var cd = jsonOf('chartData');
    var a = document.getElementById('chartDays'); if (a && cd) drawColumns(a, cd);
    var b = document.getElementById('chartCost'); if (b && cd) drawCost(b, cd);
    var c = document.getElementById('team'); var col = jsonOf('teamData'); if (c && col) drawTeam(c, col);
    tickElapsed();
  }
  var resizeTimer;
  addEventListener('resize', function () { clearTimeout(resizeTimer); resizeTimer = setTimeout(drawAll, 120); });

  // ---- elapsed clocks on the subagent page
  function tickElapsed() {
    var now = Date.now();
    document.querySelectorAll('.elapsed[data-started]').forEach(function (el) {
      var t = Date.parse(el.getAttribute('data-started')); if (isNaN(t)) return;
      var s = Math.max(0, Math.floor((now - t) / 1000));
      el.textContent = s < 60 ? s + 's' : s < 3600 ? Math.floor(s / 60) + 'm' + ('0' + s % 60).slice(-2) + 's' : Math.floor(s / 3600) + 'h' + ('0' + Math.floor(s / 60) % 60).slice(-2) + 'm';
    });
  }
  setInterval(tickElapsed, 1000);

  // ---- log search filters in place; the form still works without script
  var search = document.getElementById('logSearch');
  if (search) search.addEventListener('input', function () {
    var q = search.value.toLowerCase();
    document.querySelectorAll('#logList .log-entry').forEach(function (e) { e.hidden = q && e.textContent.toLowerCase().indexOf(q) < 0; });
  });

  // ---- live: patch the page in place when an instrument it shows changes
  var live = (agent.getAttribute('data-live') || '').split(',').filter(Boolean);
  var dot = document.getElementById('liveDot');
  if (window.EventSource) {
    var es = new EventSource(prefix + '/events');
    es.onopen = function () { if (dot) dot.classList.add('on'); };
    es.onerror = function () { if (dot) dot.classList.remove('on'); };
    var pending;
    function patch() {
      clearTimeout(pending);
      pending = setTimeout(function () {
        var q = location.search ? location.search + '&partial=1' : '?partial=1';
        fetch(location.pathname + q).then(function (r) { return r.text(); }).then(function (html) {
          var target = document.getElementById('live'); if (!target) return;
          var open = {}; target.querySelectorAll('details[open]').forEach(function (d, i) { open[i] = true; });
          target.innerHTML = html;
          target.querySelectorAll('details').forEach(function (d, i) { if (open[i]) d.open = true; });
          drawAll();
        }).catch(function () {});
      }, 150);
    }
    live.forEach(function (name) { es.addEventListener(name, patch); });
  }

  applyTheme();
})();
