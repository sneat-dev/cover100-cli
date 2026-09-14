/* cover100 — interactive coverage treemap front-end.
 *
 * Deliberately a classic (non-module) IIFE: the Go CLI string-replaces the
 * <script src="app.js"> tag with this file inlined, so it must not use ES
 * module syntax. D3 v7 is loaded from a CDN by index.html *before* this
 * script runs, hence `d3` is read from the global scope. When the CLI runs in
 * `--file` mode it also sets `window.__COVER100_DATA__` before this script.
 */
(function () {
  'use strict';

  /* ================================================================== *
   * 1. Bootstrap                                                       *
   * ================================================================== */

  function fatalD3() {
    var chart = document.getElementById('chart');
    if (chart) chart.hidden = true;
    var panel = document.getElementById('error');
    if (!panel) return;
    panel.hidden = false;
    var title = document.createElement('h2');
    title.textContent = 'D3 could not be loaded';
    var body = document.createElement('p');
    body.textContent = 'The treemap needs D3 v7, loaded from ' +
      'https://cdn.jsdelivr.net/npm/d3@7. Check the network connection (or a ' +
      'Content-Security-Policy blocking that host) and reload the page.';
    panel.appendChild(title);
    panel.appendChild(body);
  }

  if (!window.d3) {
    fatalD3();
    return;
  }

  /* ================================================================== *
   * 2. Constants                                                       *
   * ================================================================== */

  var METRIC_NAMES = ['lines', 'functions', 'files', 'packages', 'repositories'];
  var METRIC_SET = Object.create(null);
  METRIC_NAMES.forEach(function (name) { METRIC_SET[name] = true; });

  var MODE_SET = Object.create(null);
  MODE_SET.percent = true;
  MODE_SET.count = true;

  // Colour is always keyed on the *percentage* of the selected metric, never
  // on the sizing value, so the legend built from this scale matches exactly.
  var COLOR_SCALE = d3.scaleLinear()
    .domain([0, 0.5, 1])
    .range(['#e74c3c', '#f1c40f', '#2ecc71'])
    .clamp(true);
  var UNMEASURED_FILL = '#4a4a4a';

  var SHOW_LABEL_MIN_W = 56;
  var SHOW_LABEL_MIN_H = 16;
  var DETAIL_MIN_W = 110;
  var DETAIL_MIN_H = 30;
  var CHAR_W = 6.8;             // approximate advance width of the 12px UI font
  var HEADER_PAD_TOP = 16;
  var FALLBACK_WIDTH = 800;
  var FALLBACK_HEIGHT = 600;
  var TOOLTIP_HIDE_MS = 180;
  var TOAST_MS = 1500;
  var COPY_FEEDBACK_MS = 1100;

  /* ================================================================== *
   * 3. State and DOM handles                                           *
   * ================================================================== */

  var state = {
    data: null,          // normalized report (tree + warnings)
    langs: [],           // every switchable language present in the tree
    checked: null,       // Set of enabled languages
    metric: 'lines',
    mode: 'percent',
    sizeBy: 'total',
    query: '',
    focusId: '',
    visible: null,       // pruned + rolled-up copy of the tree
    byId: Object.create(null),
    nodeEls: Object.create(null),
    highlighted: [],
    boxCount: 0,
    matchCount: 0,
    usingFallback: false,
    fallbackKind: null,  // 'lines' | 'uniform'
    tooltipNode: null
  };

  var dom = {};
  var rafId = 0;
  var tooltipHideTimer = 0;
  var toastTimer = 0;

  /* ================================================================== *
   * 4. Small helpers                                                   *
   * ================================================================== */

  // num() is the single guard that keeps NaN/Infinity out of the whole UI:
  // every metric value read from the report passes through it.
  function num(value) {
    var n = Number(value);
    if (!isFinite(n) || n <= 0) return 0;
    return Math.floor(n);
  }

  function metric(covered, total) {
    return { covered: num(covered), total: num(total) };
  }

  function addMetric(a, b) {
    return { covered: a.covered + b.covered, total: a.total + b.total };
  }

  // cov() mirrors model.Metric.Any(): a package/file/function counts as one
  // covered unit when at least one unit inside it is covered.
  function cov(m) {
    return (m.total > 0 && m.covered > 0) ? 1 : 0;
  }

  function ownMetric(node, name) {
    return metric(node[name] && node[name].covered, node[name] && node[name].total);
  }

  function sumMetric(nodes, name) {
    var acc = { covered: 0, total: 0 };
    nodes.forEach(function (node) { acc = addMetric(acc, node.m[name]); });
    return acc;
  }

  function countCovered(nodes) {
    var n = 0;
    nodes.forEach(function (node) { if (node.m.lines.covered > 0) n++; });
    return n;
  }

  // Fraction in [0,1], or null when the dimension was never measured.
  function percentOf(m) {
    if (!m || m.total <= 0) return null;
    return Math.max(0, Math.min(1, m.covered / m.total));
  }

  function formatPercent(p) {
    return (p * 100).toFixed(1) + '%';
  }

  function formatCoverage(m) {
    var p = percentOf(m);
    if (p === null) return 'not measured';
    return m.covered + ' / ' + m.total + ' (' + formatPercent(p) + ')';
  }

  function truncate(text, budget) {
    var s = String(text == null ? '' : text);
    var max = Math.max(1, budget);
    if (s.length <= max) return s;
    if (max === 1) return '…';
    return s.slice(0, max - 1) + '…';
  }

  function parseColor(color) {
    if (!color) return null;
    var rgb = /^rgb\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*\)$/.exec(color);
    if (rgb) return [Number(rgb[1]), Number(rgb[2]), Number(rgb[3])];
    var hex = /^#([0-9a-f]{6})$/i.exec(color);
    if (hex) {
      var n = parseInt(hex[1], 16);
      return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
    }
    return null;
  }

  // Text must stay readable on the red end (white) and the yellow/green end
  // (near-black). Pick by relative luminance of the actual box fill.
  function textColorFor(fill) {
    var rgb = parseColor(fill);
    if (!rgb) return '#ffffff';
    var luminance = (0.2126 * rgb[0] + 0.7152 * rgb[1] + 0.0722 * rgb[2]) / 255;
    return luminance > 0.55 ? '#0d0d0d' : '#ffffff';
  }

  function fillFor(node) {
    var p = percentOf(node.m[state.metric]);
    return p === null ? UNMEASURED_FILL : COLOR_SCALE(p);
  }

  function labelPercent(node) {
    var p = percentOf(node.m[state.metric]);
    return p === null ? 'n/a' : formatPercent(p);
  }

  /* ================================================================== *
   * 5. Report loading and normalization                                *
   * ================================================================== */

  function normalizeNode(raw) {
    if (!raw || typeof raw !== 'object') return null;
    var node = {
      id: (typeof raw.id === 'string' && raw.id) ? raw.id : 'node',
      name: typeof raw.name === 'string' ? raw.name : '',
      type: typeof raw.type === 'string' ? raw.type : 'file',
      language: (typeof raw.language === 'string' && raw.language) ? raw.language : null,
      path: typeof raw.path === 'string' ? raw.path : '',
      line: (typeof raw.line === 'number' && isFinite(raw.line)) ? raw.line : null,
      hits: (typeof raw.hits === 'number' && isFinite(raw.hits)) ? raw.hits : null,
      covered: typeof raw.covered === 'boolean' ? raw.covered : null,
      functionsAvailable: raw.functionsAvailable === false ? false
        : (raw.functionsAvailable === true ? true : null),
      branches: raw.branches ? metric(raw.branches.covered, raw.branches.total) : null,
      lines: metric(raw.lines && raw.lines.covered, raw.lines && raw.lines.total),
      functions: metric(raw.functions && raw.functions.covered, raw.functions && raw.functions.total),
      files: metric(raw.files && raw.files.covered, raw.files && raw.files.total),
      packages: metric(raw.packages && raw.packages.covered, raw.packages && raw.packages.total),
      repositories: metric(raw.repositories && raw.repositories.covered,
        raw.repositories && raw.repositories.total),
      children: []
    };
    if (!node.name) node.name = node.id;
    if (Array.isArray(raw.children)) {
      raw.children.forEach(function (child) {
        var normalized = normalizeNode(child);
        if (normalized) node.children.push(normalized);
      });
    }
    return node;
  }

  function validateReport(json) {
    if (!json || typeof json !== 'object' || Array.isArray(json)) {
      return 'The payload is not a JSON object.';
    }
    if (!json.tree || typeof json.tree !== 'object' || Array.isArray(json.tree)) {
      return 'The payload has no "tree" object.';
    }
    if (typeof json.tree.id !== 'string' || json.tree.id === '') {
      return 'The root tree node has no non-empty string "id".';
    }
    return null;
  }

  function normalizeReport(json) {
    return {
      tree: normalizeNode(json.tree),
      root: typeof json.root === 'string' ? json.root : '',
      generatedAt: typeof json.generatedAt === 'string' ? json.generatedAt : '',
      tool: typeof json.tool === 'string' ? json.tool : '',
      warnings: Array.isArray(json.warnings)
        ? json.warnings.filter(function (w) { return typeof w === 'string' && w; })
        : []
    };
  }

  function loadFromUrl(url) {
    fetch(url, { cache: 'no-store' })
      .then(function (response) {
        if (!response.ok) {
          throw new Error('HTTP ' + response.status +
            (response.statusText ? ' ' + response.statusText : ''));
        }
        return response.json();
      })
      .then(function (json) {
        start(json, url);
      })
      .catch(function (err) {
        if (window.console && console.error) {
          console.error('cover100: could not load ' + url, err);
        }
        showLoadError(url, err);
      });
  }

  function start(json, source) {
    var problem = validateReport(json);
    if (problem) {
      showError('Invalid coverage report', [
        { text: 'Source: ' + source, cls: 'url' },
        { text: problem },
        { text: 'The report must match the cover100 JSON contract: a top-level ' +
            '"tree" object whose nodes carry lines/functions/files/packages/repositories pairs.' }
      ]);
      return;
    }

    state.data = normalizeReport(json);
    state.langs = collectLanguages(json.languages, json.tree);
    state.checked = new Set(state.langs);
    state.focusId = state.data.tree.id;

    buildLanguageControls();
    applyQueryState();
    rebuildTree();
    render();
  }

  // 'mixed' is a derived label, not a switchable language, so it never becomes
  // a checkbox; it is kept only while one of its children survives pruning.
  function collectLanguages(declared, tree) {
    var out = [];
    var seen = Object.create(null);
    function add(lang) {
      if (typeof lang !== 'string' || !lang || lang === 'mixed') return;
      if (seen[lang]) return;
      seen[lang] = true;
      out.push(lang);
    }
    (Array.isArray(declared) ? declared : []).forEach(add);
    (function walk(node) {
      if (!node) return;
      add(node.language);
      (Array.isArray(node.children) ? node.children : []).forEach(walk);
    })(tree);
    return out;
  }

  /* ================================================================== *
   * 6. Language pruning                                                *
   * ================================================================== */

  function leafLangVisible(lang) {
    if (lang == null) return true;
    if (lang === 'mixed') return true;   // a mixed leaf has no single switch
    return state.checked.has(lang);
  }

  function nodeLangChecked(lang) {
    return lang != null && state.checked.has(lang);
  }

  // Pruning walks the report and returns a fresh tree containing only nodes
  // that survive the language filter. Copies are shallow: the reported metric
  // objects are shared read-only and never mutated (rollup writes node.m), so
  // rebuilding from the original report always starts clean.
  function prune(node, parentCopy) {
    var copy = Object.assign({}, node);
    copy.__parent = parentCopy || null;
    copy.children = [];

    var source = node.children || [];
    for (var i = 0; i < source.length; i++) {
      var child = prune(source[i], copy);
      if (child) copy.children.push(child);
    }
    if (copy.children.length > 0) return copy;

    if (node.type === 'root') return copy;  // the root always survives
    if (node.type === 'file' || node.type === 'function') {
      return leafLangVisible(node.language) ? copy : null;
    }
    return nodeLangChecked(node.language) ? copy : null;
  }

  function indexTree(node) {
    state.byId[node.id] = node;
    (node.children || []).forEach(indexTree);
  }

  function rebuildTree() {
    state.visible = prune(state.data.tree, null);
    rollup(state.visible);
    state.byId = Object.create(null);
    indexTree(state.visible);
    if (!state.byId[state.focusId]) state.focusId = state.visible.id;
  }

  /* ================================================================== *
   * 7. Metric rollup (mirror of internal/aggregate)                    *
   * ================================================================== *
   *
   * Because the language filter prunes the tree we recompute every metric
   * over the *visible* subtree instead of trusting the JSON numbers. With no
   * filter active this reproduces the report exactly: e.g. the contract's
   * cart.ts reports functions 6/7 while its single function child reports
   * 1/1, so a file keeps its own reported lines/functions (summing function
   * children would yield 1/1 and break the round-trip).
   */
  function rollup(node) {
    var kids = node.children || [];

    // Post-order: recurse first so *every* node gets an `m`, including the
    // function children of a file (a file reports its own lines/functions
    // rather than summing them, but its children are still treemap leaves).
    kids.forEach(rollup);

    var out;
    if (node.type === 'function') {
      out = rollupFunction(node);
    } else if (node.type === 'file') {
      out = rollupFile(node);
    } else if (kids.length === 0) {
      out = rollupEmptyGroup(node);
    } else {
      out = {
        lines: sumMetric(kids, 'lines'),
        functions: sumMetric(kids, 'functions'),
        files: sumMetric(kids, 'files'),
        packages: { covered: 0, total: 0 },
        repositories: { covered: 0, total: 0 }
      };
      rollupGroupMetrics(node, kids, out);
    }

    node.m = out;
    return out;
  }

  function rollupFunction(node) {
    var lines = ownMetric(node, 'lines');
    var any = cov(lines);
    return {
      lines: lines,
      functions: metric(num(node.hits) > 0 ? 1 : 0, 1),
      files: metric(any, 1),
      packages: metric(any, 1),
      repositories: metric(any, 1)
    };
  }

  function rollupFile(node) {
    var lines = ownMetric(node, 'lines');
    var any = cov(lines);
    return {
      lines: lines,
      functions: ownMetric(node, 'functions'),
      files: metric(any, 1),
      packages: metric(any, 1),
      repositories: metric(any, 1)
    };
  }

  // A package/repository/root whose children were all pruned away but which
  // itself matched the filter: keep its reported numbers rather than reporting
  // an empty (and misleading) zero.
  function rollupEmptyGroup(node) {
    var lines = ownMetric(node, 'lines');
    var any = cov(lines);
    var out = {
      lines: lines,
      functions: ownMetric(node, 'functions'),
      files: metric(any, 1),
      packages: { covered: 0, total: 0 },
      repositories: { covered: 0, total: 0 }
    };
    if (node.type === 'package') {
      out.packages = metric(any, 1);
      out.repositories = metric(any, 1);
    } else if (node.type === 'repository') {
      out.repositories = metric(any, 1);
    }
    return out;
  }

  function rollupGroupMetrics(node, kids, out) {
    if (node.type === 'package') {
      var pkgAny = cov(out.lines);
      out.packages = metric(pkgAny, 1);
      out.repositories = metric(pkgAny, 1);
    } else if (node.type === 'repository') {
      var packages = kids.filter(function (kid) { return kid.type === 'package'; });
      out.packages = metric(countCovered(packages), packages.length);
      out.repositories = metric(cov(out.lines), 1);
    } else if (node.type === 'root') {
      out.packages = sumMetric(kids, 'packages');
      var repositories = kids.filter(function (kid) { return kid.type === 'repository'; });
      out.repositories = metric(countCovered(repositories), repositories.length);
    } else {
      var any = cov(out.lines);
      out.packages = metric(any, 1);
      out.repositories = metric(any, 1);
    }
  }

  /* ================================================================== *
   * 8. Hierarchical layout                                             *
   * ================================================================== */

  // NOTE ON D3 ACCESSOR CONVENTIONS (they differ, and mixing them up throws):
  //   * hierarchy.sum(fn)      -> fn receives the node's *datum*   (node.data)
  //   * treemap.padding*(fn)   -> fn receives the *hierarchy node* (has .children)
  // Both callbacks below respect their own convention.
  function isLeafDatum(datum) {
    return !(datum.children && datum.children.length);
  }

  function leafRawValue(datum) {
    var m = datum.m[state.metric];
    if (state.mode === 'count') return m.total;
    if (state.sizeBy === 'total') return m.total;
    return Math.max(0, m.total - m.covered);
  }

  function buildHierarchy() {
    var focus = state.byId[state.focusId] || state.visible;
    var root = d3.hierarchy(focus, function (node) {
      return (node.children && node.children.length) ? node.children : null;
    });

    state.usingFallback = false;
    state.fallbackKind = null;

    root.sum(function (datum) {
      return isLeafDatum(datum) ? leafRawValue(datum) : 0;
    });

    // Zero-value fallback: a Go-only tree has no function data at all, and
    // "uncovered" sizing is zero for a fully covered tree. Re-size by line
    // totals and tell the user, rather than rendering an empty canvas.
    if (!(root.value > 0)) {
      state.usingFallback = true;
      root.sum(function (datum) {
        return isLeafDatum(datum) ? datum.m.lines.total : 0;
      });
      state.fallbackKind = root.value > 0 ? 'lines' : 'uniform';
      if (!(root.value > 0)) {
        root.sum(function (datum) {
          return isLeafDatum(datum) ? 1 : 0;
        });
      }
    }
    return root;
  }

  function layoutHierarchy(root, width, height) {
    d3.treemap()
      .size([width, height])
      .tile(d3.treemapSquarify)
      .round(true)
      .paddingInner(2)
      .paddingOuter(3)
      // D3 hands padding callbacks the hierarchy node itself, so `.children`
      // here is the hierarchy's child list (see the accessor note above).
      .paddingTop(function (hierarchyNode) {
        return (hierarchyNode.children && hierarchyNode.children.length) ? HEADER_PAD_TOP : 0;
      })(root);
  }

  /* ================================================================== *
   * 9. Rendering                                                       *
   * ================================================================== */

  function render() {
    if (!state.visible) return;

    var width = dom.chart.clientWidth > 0 ? dom.chart.clientWidth : FALLBACK_WIDTH;
    var height = dom.chart.clientHeight > 0 ? dom.chart.clientHeight : FALLBACK_HEIGHT;

    var root = buildHierarchy();
    layoutHierarchy(root, width, height);
    drawTreemap(root, width, height);

    updateBreadcrumb();
    updateSummary();
    updateNotices();
    applySearch();
  }

  function drawTreemap(root, width, height) {
    dom.svg.attr('width', width).attr('height', height);
    dom.svg.selectAll('*').remove();
    state.nodeEls = Object.create(null);
    state.highlighted = [];

    var defs = dom.svg.append('defs');
    var layer = dom.svg.append('g').attr('class', 'treemap');
    var drawn = 0;

    // d3.hierarchy().descendants() is breadth-first, i.e. parents come first
    // and children paint on top of them.
    root.descendants().forEach(function (d, i) {
      var node = d.data;
      var w = Math.max(0, d.x1 - d.x0);
      var h = Math.max(0, d.y1 - d.y0);
      if (w <= 0 || h <= 0) return;
      drawn++;

      var fill = fillFor(node);
      // .datum(d) is required: d3 passes the bound datum as the second argument
      // to every event listener below (click/hover/contextmenu).
      var g = layer.append('g')
        .datum(d)
        .attr('class', 'node')
        .attr('data-id', node.id)
        .attr('transform', 'translate(' + d.x0 + ',' + d.y0 + ')');

      var rect = g.append('rect')
        .attr('class', 'box')
        .attr('width', w)
        .attr('height', h)
        .attr('rx', 2)
        .attr('fill', fill);

      drawLabel(g, defs, node, fill, w, h, i);

      state.nodeEls[node.id] = { g: g.node(), rect: rect.node() };

      g.on('mouseenter', function (event, hd) { highlightAncestors(hd); })
        .on('mousemove', function (event, hd) { showTooltip(event, hd.data); })
        .on('mouseleave', function () {
          scheduleHideTooltip();
          clearAncestorHighlight();
        })
        .on('click', function (event, hd) {
          event.stopPropagation();
          focusNode(hd.data.id);
        })
        .on('contextmenu', function (event, hd) {
          event.preventDefault();
          rightClickCopy(hd.data);
        });
    });

    state.boxCount = drawn;
  }

  function drawLabel(g, defs, node, fill, w, h, index) {
    if (w <= SHOW_LABEL_MIN_W || h <= SHOW_LABEL_MIN_H) return;

    var clipId = 'c100-clip-' + index;
    defs.append('clipPath').attr('id', clipId)
      .append('rect').attr('width', w).attr('height', h);
    var label = g.append('g').attr('clip-path', 'url(#' + clipId + ')');
    var color = textColorFor(fill);

    var detailed = w > DETAIL_MIN_W && h > DETAIL_MIN_H;
    var pctText = detailed ? labelPercent(node) : '';
    var reserved = pctText ? pctText.length * CHAR_W + 10 : 0;
    var budget = Math.floor((w - 8 - reserved) / CHAR_W);

    label.append('text')
      .attr('class', 'label-name')
      .attr('x', 4)
      .attr('y', 12)
      .attr('fill', color)
      .text(truncate(node.name, Math.max(2, budget)));

    if (pctText) {
      label.append('text')
        .attr('class', 'label-pct')
        .attr('x', w - 4)
        .attr('y', 12)
        .attr('text-anchor', 'end')
        .attr('fill', color)
        .text(pctText);
    }
  }

  function updateBreadcrumb() {
    var chain = [];
    var node = state.byId[state.focusId] || state.visible;
    while (node) {
      chain.push(node);
      node = node.__parent;
    }
    chain.reverse();

    dom.breadcrumb.textContent = '';
    chain.forEach(function (crumbNode, i) {
      if (i > 0) {
        var sep = document.createElement('span');
        sep.className = 'crumb-sep';
        sep.setAttribute('aria-hidden', 'true');
        sep.textContent = '▸';
        dom.breadcrumb.appendChild(sep);
      }
      // The spec's breadcrumb reads "root ▸ my-app ▸ src/checkout ▸ cart.ts":
      // packages are identified by their path, every other node by its name.
      var label = (crumbNode.type === 'package' && crumbNode.path)
        ? crumbNode.path
        : (crumbNode.name || crumbNode.id);
      var button = document.createElement('button');
      button.type = 'button';
      button.className = 'crumb';
      button.textContent = label;
      button.title = crumbNode.path || label;
      button.setAttribute('aria-label', 'Zoom to ' + label);
      if (i === chain.length - 1) button.setAttribute('aria-current', 'true');
      button.addEventListener('click', function () { focusNode(crumbNode.id); });
      dom.breadcrumb.appendChild(button);
    });
  }

  function updateSummary() {
    var focus = state.byId[state.focusId] || state.visible;
    var m = focus.m[state.metric];
    var boxes = state.boxCount + (state.boxCount === 1 ? ' box' : ' boxes');
    dom.summary.textContent = state.metric + ': ' + formatCoverage(m) + ' · ' + boxes;
  }

  function updateNotices() {
    if (state.usingFallback) {
      dom.notice.textContent = state.fallbackKind === 'uniform'
        ? 'No measured coverage volume here — boxes sized equally.'
        : 'No measured "' + state.metric + '" volume here — boxes sized by line totals.';
      dom.notice.hidden = false;
    } else {
      dom.notice.textContent = '';
      dom.notice.hidden = true;
    }

    dom.warnings.textContent = '';
    state.data.warnings.forEach(function (warning) {
      var chip = document.createElement('span');
      chip.className = 'notice warn';
      chip.textContent = '⚠ ' + warning;
      dom.warnings.appendChild(chip);
    });
  }

  /* ================================================================== *
   * 10. Search highlighting and ancestor hover                         *
   * ================================================================== */

  function applySearch() {
    var query = (state.query || '').trim().toLowerCase();
    var matches = 0;

    Object.keys(state.nodeEls).forEach(function (id) {
      var record = state.nodeEls[id];
      var node = state.byId[id];
      if (!record || !node) return;

      var hit = false;
      if (query) {
        var name = (node.name || '').toLowerCase();
        var path = (node.path || '').toLowerCase();
        hit = name.indexOf(query) >= 0 || path.indexOf(query) >= 0;
      }
      if (hit) matches++;
      record.g.classList.toggle('match', hit);
      record.g.classList.toggle('dim', !!query && !hit);
    });

    state.matchCount = matches;
    dom.matches.textContent = query
      ? matches + (matches === 1 ? ' match' : ' matches')
      : '';
  }

  function highlightAncestors(hierarchyNode) {
    clearAncestorHighlight();
    var node = hierarchyNode.parent;
    while (node) {
      var record = state.nodeEls[node.data.id];
      if (record) {
        record.rect.classList.add('ancestor');
        state.highlighted.push(record.rect);
      }
      node = node.parent;
    }
  }

  function clearAncestorHighlight() {
    state.highlighted.forEach(function (rect) {
      rect.classList.remove('ancestor');
    });
    state.highlighted = [];
  }

  /* ================================================================== *
   * 11. Tooltip, toast and clipboard                                   *
   * ================================================================== */

  // Path relative to the repository root, without the function line suffix.
  function relativePath(node) {
    if (!node || node.type === 'root') return '';
    var path = node.path;
    if (!path && node.__parent) path = node.__parent.path;
    return path || '';
  }

  function displayPath(node) {
    var path = relativePath(node);
    if (node.type === 'function' && node.line != null) {
      return path ? path + ':' + node.line : 'line ' + node.line;
    }
    return path;
  }

  function showTooltip(event, node) {
    cancelHideTooltip();
    state.tooltipNode = node;
    renderTooltip(node);
    dom.tooltip.hidden = false;
    positionTooltip(event.clientX, event.clientY);
  }

  function positionTooltip(x, y) {
    var pad = 8;
    var offset = 14;
    var w = dom.tooltip.offsetWidth;
    var h = dom.tooltip.offsetHeight;
    var left = x + offset;
    var top = y + offset;

    if (left + w + pad > window.innerWidth) left = Math.max(pad, x - offset - w);
    if (top + h + pad > window.innerHeight) top = Math.max(pad, y - offset - h);

    dom.tooltip.style.left = left + 'px';
    dom.tooltip.style.top = top + 'px';
  }

  function renderTooltip(node) {
    dom.ttName.textContent = node.name || node.id;
    dom.ttType.textContent = node.type;
    dom.ttLang.textContent = 'language: ' + (node.language || 'unknown');

    var path = displayPath(node);
    dom.ttPath.textContent = path;
    dom.ttPath.hidden = !path;

    dom.ttMetrics.textContent = '';
    if (node.branches && node.branches.total > 0) {
      dom.ttMetrics.appendChild(metricRow('branches', node.branches));
    }
    METRIC_NAMES.forEach(function (name) {
      dom.ttMetrics.appendChild(metricRow(name, node.m[name]));
    });

    if (node.type === 'function') {
      var hits = node.hits == null ? 0 : node.hits;
      var stateText = node.covered === true ? 'covered'
        : (node.covered === false ? 'uncovered' : (hits > 0 ? 'covered' : 'uncovered'));
      dom.ttFn.textContent = 'line ' + (node.line == null ? '?' : node.line) +
        ' · ' + hits + (hits === 1 ? ' hit' : ' hits') + ' · ' + stateText;
      dom.ttFn.hidden = false;
    } else {
      dom.ttFn.textContent = '';
      dom.ttFn.hidden = true;
    }

    if (node.functionsAvailable === false) {
      dom.ttNote.textContent = 'Function coverage is not available for Go files.';
      dom.ttNote.hidden = false;
    } else {
      dom.ttNote.textContent = '';
      dom.ttNote.hidden = true;
    }

    dom.ttCopy.hidden = !relativePath(node);
    dom.ttCopy.textContent = 'Copy path';
    dom.ttCopy.classList.remove('ok');
  }

  function metricRow(name, m) {
    var row = document.createElement('div');
    row.className = 'metric-row';

    var label = document.createElement('span');
    label.className = 'm-name';
    label.textContent = name;
    row.appendChild(label);

    var value = document.createElement('span');
    value.className = 'm-val';
    if (!m || m.total <= 0) {
      value.classList.add('not-measured');
      value.textContent = 'not measured';
    } else {
      // In count mode the absolute pair carries the emphasis, in percent mode
      // the ratio does. Both stay visible, exactly one is bold/coloured.
      var absolute = document.createElement('span');
      if (state.mode === 'count') absolute.className = 'emph';
      absolute.textContent = m.covered + ' / ' + m.total;
      var ratio = document.createElement('span');
      if (state.mode === 'percent') ratio.className = 'emph';
      ratio.textContent = ' (' + formatPercent(percentOf(m)) + ')';
      value.appendChild(absolute);
      value.appendChild(ratio);
    }
    row.appendChild(value);
    return row;
  }

  function scheduleHideTooltip() {
    cancelHideTooltip();
    tooltipHideTimer = window.setTimeout(hideTooltip, TOOLTIP_HIDE_MS);
  }

  function cancelHideTooltip() {
    if (tooltipHideTimer) {
      window.clearTimeout(tooltipHideTimer);
      tooltipHideTimer = 0;
    }
  }

  function hideTooltip() {
    cancelHideTooltip();
    dom.tooltip.hidden = true;
    state.tooltipNode = null;
  }

  function toast(message) {
    dom.toast.textContent = message;
    dom.toast.hidden = false;
    if (toastTimer) window.clearTimeout(toastTimer);
    toastTimer = window.setTimeout(function () {
      dom.toast.hidden = true;
      toastTimer = 0;
    }, TOAST_MS);
  }

  function legacyCopy(text) {
    var area = document.createElement('textarea');
    area.value = text;
    area.setAttribute('readonly', '');
    area.style.position = 'fixed';
    area.style.top = '-1000px';
    area.style.opacity = '0';
    document.body.appendChild(area);
    area.select();
    var ok = false;
    try {
      ok = document.execCommand('copy');
    } catch (err) {
      ok = false;
    }
    document.body.removeChild(area);
    return ok;
  }

  function copyText(text) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      return navigator.clipboard.writeText(text).then(function () {
        return true;
      }, function () {
        return legacyCopy(text);
      });
    }
    return Promise.resolve(legacyCopy(text));
  }

  function copyNodePath(node) {
    var path = relativePath(node);
    if (!path) return Promise.resolve(false);
    return Promise.resolve(copyText(path));
  }

  function rightClickCopy(node) {
    var path = relativePath(node);
    if (!path) {
      toast('No path to copy for this node');
      return;
    }
    copyNodePath(node).then(function (ok) {
      toast(ok ? 'Copied ' + path : 'Copy failed');
    });
  }

  function onCopyButtonClick() {
    var node = state.tooltipNode;
    if (!node || !relativePath(node)) return;
    copyNodePath(node).then(function (ok) {
      dom.ttCopy.textContent = ok ? 'Copied' : 'Copy failed';
      dom.ttCopy.classList.toggle('ok', ok);
      window.setTimeout(function () {
        dom.ttCopy.textContent = 'Copy path';
        dom.ttCopy.classList.remove('ok');
      }, COPY_FEEDBACK_MS);
    });
  }

  /* ================================================================== *
   * 12. Navigation                                                     *
   * ================================================================== */

  function focusNode(id) {
    if (!state.byId[id] || state.focusId === id) return;
    state.focusId = id;
    hideTooltip();
    render();
  }

  function zoomOut() {
    var focus = state.byId[state.focusId];
    if (!focus || !focus.__parent) return;  // already at the root
    state.focusId = focus.__parent.id;
    hideTooltip();
    render();
  }

  /* ================================================================== *
   * 13. Controls                                                       *
   * ================================================================== */

  function setMode(mode) {
    state.mode = MODE_SET[mode] ? mode : 'percent';
    var isPercent = state.mode === 'percent';

    dom.modePercent.classList.toggle('active', isPercent);
    dom.modePercent.setAttribute('aria-pressed', isPercent ? 'true' : 'false');
    dom.modeCount.classList.toggle('active', !isPercent);
    dom.modeCount.setAttribute('aria-pressed', isPercent ? 'false' : 'true');

    dom.sizeByCtl.hidden = !isPercent;
    render();
  }

  function applyQueryState() {
    var params = new URLSearchParams(window.location.search);

    var metric = params.get('metric');
    state.metric = METRIC_SET[metric] ? metric : 'lines';
    dom.metric.value = state.metric;

    var mode = params.get('mode');
    state.mode = MODE_SET[mode] ? mode : 'percent';

    var isPercent = state.mode === 'percent';
    dom.modePercent.classList.toggle('active', isPercent);
    dom.modePercent.setAttribute('aria-pressed', isPercent ? 'true' : 'false');
    dom.modeCount.classList.toggle('active', !isPercent);
    dom.modeCount.setAttribute('aria-pressed', isPercent ? 'false' : 'true');
    dom.sizeByCtl.hidden = !isPercent;
  }

  function buildLanguageControls() {
    dom.langs.textContent = '';
    state.langs.forEach(function (lang) {
      var label = document.createElement('label');
      label.className = 'lang';

      var box = document.createElement('input');
      box.type = 'checkbox';
      box.value = lang;
      box.checked = true;
      box.setAttribute('aria-label', 'Show ' + lang + ' coverage');
      box.addEventListener('change', onLanguageChange);

      var text = document.createElement('span');
      text.textContent = lang;

      label.appendChild(box);
      label.appendChild(text);
      dom.langs.appendChild(label);
    });
  }

  function onLanguageChange() {
    var inputs = dom.langs.querySelectorAll('input[type="checkbox"]');
    var next = new Set();
    Array.prototype.forEach.call(inputs, function (input) {
      if (input.checked) next.add(input.value);
    });
    state.checked = next;
    rebuildTree();
    render();
  }

  function resetView() {
    dom.search.value = '';
    state.query = '';

    var inputs = dom.langs.querySelectorAll('input[type="checkbox"]');
    Array.prototype.forEach.call(inputs, function (input) { input.checked = true; });
    state.checked = new Set(state.langs);

    state.focusId = state.data.tree.id;
    hideTooltip();
    clearAncestorHighlight();
    rebuildTree();
    render();
  }

  function buildLegend() {
    var stops = [];
    for (var i = 0; i <= 20; i++) {
      var p = i / 20;
      stops.push(COLOR_SCALE(p) + ' ' + (p * 100).toFixed(0) + '%');
    }
    dom.legendBar.style.background = 'linear-gradient(90deg, ' + stops.join(', ') + ')';
  }

  /* ================================================================== *
   * 14. Errors                                                         *
   * ================================================================== */

  function showError(title, blocks) {
    dom.chart.hidden = true;
    dom.error.hidden = false;
    dom.error.textContent = '';

    var heading = document.createElement('h2');
    heading.textContent = title;
    dom.error.appendChild(heading);

    blocks.forEach(function (block) {
      if (!block) return;
      var p = document.createElement('p');
      if (block.cls) p.className = block.cls;
      p.textContent = block.text;
      dom.error.appendChild(p);
    });
  }

  function showLoadError(url, err) {
    var detail = (err && err.message) ? err.message : String(err);
    showError('Could not load coverage data', [
      { text: 'Failed URL: ' + url, cls: 'url' },
      { text: 'Reason: ' + detail },
      { text: 'If you opened this page straight from disk (file://), the browser ' +
          'refuses to fetch() local JSON files.', cls: 'hint' },
      { text: 'Run the cover100 CLI with --file to emit a single self-contained ' +
          'HTML document with the report embedded instead.', cls: 'hint' }
    ]);
  }

  /* ================================================================== *
   * 15. Wiring and init                                                *
   * ================================================================== */

  function cacheDom() {
    dom.chart = document.getElementById('chart');
    dom.error = document.getElementById('error');
    dom.breadcrumb = document.getElementById('breadcrumb');
    dom.summary = document.getElementById('summary');
    dom.notice = document.getElementById('notice');
    dom.warnings = document.getElementById('warnings');
    dom.metric = document.getElementById('metric');
    dom.modePercent = document.getElementById('mode-percent');
    dom.modeCount = document.getElementById('mode-count');
    dom.sizeByCtl = document.getElementById('sizeby-ctl');
    dom.sizeBy = document.getElementById('sizeby');
    dom.langs = document.getElementById('langs');
    dom.search = document.getElementById('search');
    dom.matches = document.getElementById('matches');
    dom.reset = document.getElementById('reset');
    dom.legendBar = document.getElementById('legend-bar');
    dom.tooltip = document.getElementById('tooltip');
    dom.ttName = document.getElementById('tt-name');
    dom.ttType = document.getElementById('tt-type');
    dom.ttLang = document.getElementById('tt-lang');
    dom.ttPath = document.getElementById('tt-path');
    dom.ttMetrics = document.getElementById('tt-metrics');
    dom.ttFn = document.getElementById('tt-fn');
    dom.ttNote = document.getElementById('tt-note');
    dom.ttCopy = document.getElementById('tt-copy');
    dom.toast = document.getElementById('toast');

    dom.svg = d3.select(dom.chart).append('svg')
      .attr('class', 'treemap-svg')
      .attr('role', 'img')
      .attr('aria-label', 'Coverage treemap');
  }

  function scheduleRender() {
    if (rafId) return;
    rafId = window.requestAnimationFrame(function () {
      rafId = 0;
      render();
    });
  }

  function wireControls() {
    dom.metric.addEventListener('change', function () {
      state.metric = METRIC_SET[dom.metric.value] ? dom.metric.value : 'lines';
      render();
    });

    dom.modePercent.addEventListener('click', function () { setMode('percent'); });
    dom.modeCount.addEventListener('click', function () { setMode('count'); });

    dom.sizeBy.addEventListener('change', function () {
      state.sizeBy = dom.sizeBy.value === 'uncovered' ? 'uncovered' : 'total';
      render();
    });

    dom.search.addEventListener('input', function () {
      state.query = dom.search.value;
      applySearch();
    });

    dom.reset.addEventListener('click', resetView);

    dom.tooltip.addEventListener('mouseenter', cancelHideTooltip);
    dom.tooltip.addEventListener('mouseleave', hideTooltip);
    dom.ttCopy.addEventListener('click', onCopyButtonClick);

    document.addEventListener('keydown', function (event) {
      if (event.key === 'Escape') {
        event.preventDefault();
        zoomOut();
      }
    });

    window.addEventListener('resize', scheduleRender);
    if (window.ResizeObserver) {
      new ResizeObserver(scheduleRender).observe(dom.chart);
    }
  }

  function init() {
    cacheDom();
    wireControls();
    buildLegend();

    // Resolution order: inlined CLI data, then ?data=<url>, then coverage.json.
    var embedded = window.__COVER100_DATA__;
    if (embedded && typeof embedded === 'object' && !Array.isArray(embedded)) {
      start(embedded, 'window.__COVER100_DATA__ (embedded by the CLI)');
      return;
    }

    var params = new URLSearchParams(window.location.search);
    loadFromUrl(params.get('data') || 'coverage.json');
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
