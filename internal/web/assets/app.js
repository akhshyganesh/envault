/*
  envault — browser UI.

  The page builds every node with createElement and textContent. That is not a
  style preference: .env values flow straight into this DOM, and assigning them
  as markup would let a secret containing "<img onerror=…>" execute. No string
  in this file is ever parsed as markup, and a test enforces that.
*/

"use strict";

// ---------- session ----------

// The token arrives in the URL because that is the only thing a fresh browser
// tab can carry. It is lifted out immediately and the URL is cleaned, so the
// secret does not sit in the address bar, history, or a screenshot.
const TOKEN = new URLSearchParams(location.search).get("token") || "";
if (location.search) history.replaceState(null, "", location.pathname);

// A fixed-width mask: the number of dots must never track a secret's length.
const MASK = "••••••••••••";

const SIDE_MIN = 210;
const SIDE_MAX = 560;
const SIDE_DEFAULT = 300;

const state = {
  files: [], archives: [], config: {}, daemon: {}, vaultDir: "",
  filter: "",
  showArchived: false,
  selected: null,     // index within the currently visible file list
  detail: null,       // { display, path, snapshots, version, content, prev }
  reveal: new Set(),  // keys revealed one at a time in the current version
  revealAll: false,
  raw: false,
  peek: null,         // { path, files } while reading an archive
  busy: 0,
};

// ---------- DOM helpers ----------

const SVG_NS = "http://www.w3.org/2000/svg";

function $(id) {
  return document.getElementById(id);
}

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

// icon renders one symbol from the sprite in index.html.
function icon(name, size) {
  const svg = document.createElementNS(SVG_NS, "svg");
  svg.setAttribute("class", "icon");
  svg.setAttribute("width", size || 15);
  svg.setAttribute("height", size || 15);
  svg.setAttribute("viewBox", "0 0 16 16");
  svg.setAttribute("aria-hidden", "true");
  const use = document.createElementNS(SVG_NS, "use");
  use.setAttribute("href", "#i-" + name);
  svg.appendChild(use);
  return svg;
}

// iconButton is an icon-only control, so it always carries its own label.
function iconButton(name, label, onClick) {
  const b = el("button", "icon-btn");
  b.type = "button";
  b.title = label;
  b.setAttribute("aria-label", label);
  b.appendChild(icon(name, 14));
  b.onclick = onClick;
  return b;
}

function toast(message, bad) {
  const node = el("div", "toast" + (bad ? " bad" : ""));
  node.append(icon(bad ? "alert" : "check", 14), el("span", "text", message));
  node.onclick = () => node.remove();
  $("toasts").appendChild(node);
  setTimeout(() => node.remove(), bad ? 9000 : 5000);
}

// ask opens the modal prompt and resolves to the trimmed value, or null if the
// user cancelled or left it empty.
function ask(title, sub, value, okLabel) {
  const dlg = $("promptDialog");
  $("promptTitle").textContent = title;
  $("promptSub").textContent = sub || "";
  $("promptOk").textContent = okLabel || "Continue";

  const input = $("promptInput");
  input.value = value || "";
  dlg.showModal();
  input.focus();
  input.select();

  return new Promise(resolve => {
    dlg.addEventListener("close", () => {
      const text = input.value.trim();
      resolve(dlg.returnValue === "ok" && text ? text : null);
    }, { once: true });
  });
}

function copy(text, what) {
  navigator.clipboard.writeText(text)
    .then(() => toast("Copied " + what))
    .catch(err => toast("Copy failed: " + err.message, true));
}

// ---------- API ----------

async function api(path, method, body) {
  const bar = $("progress");
  state.busy++;
  bar.classList.add("on");
  try {
    const res = await fetch(path, {
      method: method || "GET",
      headers: Object.assign(
        { "X-Envault-Token": TOKEN },
        body ? { "Content-Type": "application/json" } : {},
      ),
      body: body ? JSON.stringify(body) : undefined,
    });
    if (!res.ok) {
      let message = res.status + " " + res.statusText;
      try { message = (await res.json()).error || message; } catch (e) { /* not JSON */ }
      throw new Error(message);
    }
    return res.json();
  } finally {
    if (--state.busy === 0) bar.classList.remove("on");
  }
}

// run is the single place a failed action turns into a visible message, so no
// handler needs its own try/catch.
async function run(fn) {
  try {
    await fn();
  } catch (err) {
    toast(err.message, true);
  }
}

// ---------- resizable sidebar ----------

const resizer = $("resizer");

function setSide(px) {
  const width = Math.min(SIDE_MAX, Math.max(SIDE_MIN, Math.round(px)));
  document.documentElement.style.setProperty("--side", width + "px");
  try { localStorage.setItem("envault.side", String(width)); } catch (e) { /* private mode */ }
}

(function restoreSide() {
  try {
    const saved = Number(localStorage.getItem("envault.side"));
    if (saved) setSide(saved);
  } catch (e) { /* private mode */ }
})();

resizer.addEventListener("pointerdown", e => {
  e.preventDefault();
  resizer.setPointerCapture(e.pointerId);
  resizer.classList.add("dragging");
  document.body.classList.add("resizing");

  const move = ev => setSide(ev.clientX - $("main").getBoundingClientRect().left);
  const stop = () => {
    resizer.classList.remove("dragging");
    document.body.classList.remove("resizing");
    resizer.removeEventListener("pointermove", move);
    resizer.removeEventListener("pointerup", stop);
    resizer.removeEventListener("pointercancel", stop);
  };
  resizer.addEventListener("pointermove", move);
  resizer.addEventListener("pointerup", stop);
  resizer.addEventListener("pointercancel", stop);
});

resizer.addEventListener("dblclick", () => setSide(SIDE_DEFAULT));

resizer.addEventListener("keydown", e => {
  const current = parseInt(getComputedStyle(document.documentElement).getPropertyValue("--side"), 10) || SIDE_DEFAULT;
  if (e.key === "ArrowLeft") { e.preventDefault(); setSide(current - 16); }
  else if (e.key === "ArrowRight") { e.preventDefault(); setSide(current + 16); }
  else if (e.key === "Home") { e.preventDefault(); setSide(SIDE_MIN); }
  else if (e.key === "End") { e.preventDefault(); setSide(SIDE_MAX); }
});

// ---------- .env parsing ----------

// parseEnv turns a dotenv file into ordered key/value pairs. Lines it cannot
// read are counted rather than dropped silently, so the ledger can point at the
// raw view instead of pretending the file was fully understood.
function parseEnv(text) {
  const pairs = [];
  let skipped = 0;

  for (const line of text.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) continue;

    const match = /^(?:export\s+)?([A-Za-z_][A-Za-z0-9_.]*)\s*=\s*([\s\S]*)$/.exec(trimmed);
    if (!match) { skipped++; continue; }

    let value = match[2].trim();
    const quoted = value.length > 1 &&
      ((value[0] === '"' && value.endsWith('"')) || (value[0] === "'" && value.endsWith("'")));
    if (quoted) value = value.slice(1, -1);

    pairs.push({ key: match[1], value });
  }
  return { pairs, skipped };
}

// ---------- selectors ----------

function activeFiles() {
  const list = state.peek ? state.peek.files : (state.showArchived ? state.archives : state.files);
  if (!state.filter) return list;
  const needle = state.filter.toLowerCase();
  return list.filter(f => f.display.toLowerCase().includes(needle));
}

function currentFile() {
  const list = activeFiles();
  return state.selected === null ? null : list[state.selected];
}

function splitPath(p) {
  const cut = p.lastIndexOf("/");
  return cut < 0 ? { dir: "", name: p } : { dir: p.slice(0, cut), name: p.slice(cut + 1) };
}

// ---------- render ----------

function renderHeader() {
  $("vaultPath").textContent = state.vaultDir;

  const running = !!state.daemon.running;
  $("daemonStatus").classList.toggle("live", running);
  $("daemonText").textContent = running ? "Watching · pid " + state.daemon.pid : "Not watching";

  const toggle = $("daemonToggle");
  toggle.textContent = "";
  toggle.append(icon(running ? "stop" : "play", 13), el("span", null, running ? "Stop" : "Start"));
}

// highlight writes text into node, marking the run that matched the filter.
function highlight(node, text, needle) {
  if (!needle) { node.textContent = text; return; }
  const at = text.toLowerCase().indexOf(needle.toLowerCase());
  if (at < 0) { node.textContent = text; return; }
  node.append(
    document.createTextNode(text.slice(0, at)),
    el("mark", null, text.slice(at, at + needle.length)),
    document.createTextNode(text.slice(at + needle.length)),
  );
}

function renderFiles() {
  const box = $("fileList");
  box.textContent = "";
  const list = activeFiles();

  if (state.peek) {
    const back = el("button", "quiet");
    back.append(icon("back", 13), el("span", null, "Leave archive"));
    back.style.margin = "4px 0 6px";
    back.onclick = leaveArchive;
    box.appendChild(back);
  }

  if (!list.length) {
    box.appendChild(emptyFileList());
    return;
  }

  list.forEach((f, i) => {
    const item = el("button", "item");
    item.setAttribute("aria-current", String(state.selected === i));
    item.onclick = () => selectFile(i);
    item.append(el("span", "gutter"));

    const body = el("div", "body");
    const { dir, name } = splitPath(f.display);
    const nameEl = el("div", "name");
    highlight(nameEl, name, state.filter);
    body.append(nameEl);
    if (dir) body.append(el("div", "dir", dir));
    item.append(body);

    const count = el("span", "count", f.versions + "v");
    if (!state.peek && !state.showArchived && !f.exists) {
      const gone = el("span", "gone", " · gone");
      gone.title = "The original file is no longer on disk. Its backups are still here.";
      count.append(gone);
    }
    item.append(count);

    box.appendChild(item);
  });
}

function emptyFileList() {
  const empty = el("div", "placeholder top");

  if (state.filter) {
    empty.append(el("div", "big", "Nothing matches that filter"));
    empty.append(el("p", null, "Try a shorter fragment of the path."));
    return empty;
  }
  if (state.peek) {
    empty.append(el("div", "big", "This archive has no tracked files"));
    return empty;
  }
  if (state.showArchived) {
    empty.append(el("div", "big", "Nothing archived yet"));
    empty.append(el("p", null,
      "Archive a tracked file to park its history here. It stops being scanned but keeps every version, and can be brought back at any time."));
    return empty;
  }

  empty.append(el("div", "big", "No files tracked yet"));
  empty.append(el("p", null,
    "Add a folder to watch, then scan it. Anything named .env, .env.* or *.env is versioned from then on."));
  const row = el("div", "row");
  const add = el("button", "accent");
  add.append(icon("folder", 14), el("span", null, "Add a folder"));
  add.onclick = addWatchDir;
  row.append(add);
  empty.append(row);
  return empty;
}

function renderDetailHead() {
  const head = $("detailHead");
  head.textContent = "";
  const d = state.detail;
  if (!d) return;

  const wrap = el("div", "detail-head");
  const { dir, name } = splitPath(d.display);

  const crumb = el("div", "crumb");
  crumb.append(el("span", "file", name));
  if (dir) crumb.append(el("span", "in", dir));
  wrap.append(crumb);

  // Newest version first: that is the one people came to look at.
  const rail = el("div", "rail");
  d.snapshots.slice().reverse().forEach(s => {
    const chip = el("button", "chip");
    chip.setAttribute("aria-current", String(s.version === d.version));
    chip.append(el("span", "v", "v" + s.version), el("span", "when", s.ago));
    chip.title = s.timestamp + " · " + s.human_size + " · " + s.short_id;
    chip.onclick = () => loadVersion(s.version);
    rail.append(chip);
  });
  wrap.append(rail);
  head.appendChild(wrap);

  head.appendChild(renderDetailTools(d, name));
}

function renderDetailTools(d, name) {
  const tools = el("div", "detail-tools");

  const tool = (iconName, label, className, onClick) => {
    const b = el("button", className || "quiet");
    b.append(icon(iconName, 14), el("span", null, label));
    b.onclick = onClick;
    return b;
  };

  tools.append(tool(
    state.revealAll ? "eye-off" : "eye",
    state.revealAll ? "Hide values" : "Reveal values",
    state.revealAll ? "quiet on" : "quiet",
    () => { state.revealAll = !state.revealAll; state.reveal.clear(); renderDetail(); },
  ));
  tools.append(tool("file", "Raw file", state.raw ? "quiet on" : "quiet",
    () => { state.raw = !state.raw; renderDetail(); }));
  tools.append(tool("copy", "Copy file", "quiet",
    () => copy(d.content, "v" + d.version + " of " + name)));

  tools.append(el("span", "sep"));

  if (state.peek) {
    tools.append(tool("save", "Save a copy", "quiet", saveFromArchive));
  } else if (state.showArchived) {
    // A parked file's history is read-only until it comes back: no restore
    // over the live path, no forgetting — that is what unarchiving is for.
    tools.append(tool("save", "Save a copy", "quiet",
      () => restoreElsewhere(true)));
    tools.append(tool("restore", "Unarchive", "quiet", unarchiveFile));
  } else {
    tools.append(tool("restore", "Restore", "quiet", restoreInPlace));
    tools.append(tool("save", "Restore to", "quiet", () => restoreElsewhere(false)));
    tools.append(tool("archive", "Archive", "quiet", archiveFile));
    tools.append(tool("trash", "Stop tracking", "quiet danger", forgetFile));
  }

  const snap = d.snapshots[d.version - 1];
  tools.append(el("span", "meta", snap.timestamp + " · " + snap.human_size));
  return tools;
}

function renderDetailBody() {
  const scroll = $("detailScroll");
  scroll.textContent = "";
  const d = state.detail;

  if (!d) {
    const ph = el("div", "placeholder");
    ph.append(el("div", "big", state.peek ? "Pick a file from the archive" : "Pick a file"));
    ph.append(el("p", null, state.peek
      ? "You're reading a backup zip. Nothing in here can change your vault."
      : "Its versions appear here. Values stay hidden until you reveal them."));
    scroll.appendChild(ph);
    return;
  }

  if (state.raw) {
    scroll.appendChild(el("pre", "raw", d.content));
    return;
  }

  const { pairs, skipped } = parseEnv(d.content);
  if (!pairs.length) {
    const ph = el("div", "placeholder");
    ph.append(el("div", "big", "No variables in this version"));
    ph.append(el("p", null,
      "Nothing here parses as KEY=value. Open the raw file to see exactly what was saved."));
    scroll.appendChild(ph);
    return;
  }

  // The previous version is what makes the added/changed marks possible.
  const before = d.prev ? new Map(parseEnv(d.prev).pairs.map(p => [p.key, p.value])) : null;
  let added = 0, changed = 0;

  const ledger = el("div", "ledger");
  pairs.forEach(({ key, value }) => {
    const row = el("div", "kv");

    if (before) {
      if (!before.has(key)) {
        const flag = el("span", "flag added");
        flag.title = "Added in v" + d.version;
        row.append(flag);
        added++;
      } else if (before.get(key) !== value) {
        const flag = el("span", "flag changed");
        flag.title = "Changed in v" + d.version;
        row.append(flag);
        changed++;
      }
    }

    row.append(el("span", "k", key));

    const shown = state.revealAll || state.reveal.has(key);
    if (!value) {
      row.append(el("span", "val blank", "empty"));
    } else {
      row.append(el("span", "val" + (shown ? "" : " masked"), shown ? value : MASK));
    }

    const tools = el("span", "tools");
    if (value) {
      tools.append(iconButton(
        shown ? "eye-off" : "eye",
        shown ? "Hide this value" : "Reveal this value",
        () => {
          if (state.reveal.has(key)) state.reveal.delete(key);
          else state.reveal.add(key);
          renderDetail();
        },
      ));
      tools.append(iconButton("copy", "Copy this value", () => copy(value, key)));
    }
    row.append(tools);

    ledger.append(row);
  });

  scroll.appendChild(ledger);

  if (added || changed) {
    scroll.insertBefore(renderLegend(d.version, added, changed), ledger);
  }
  if (skipped) {
    scroll.appendChild(el("div", "note",
      skipped + (skipped === 1 ? " line" : " lines") +
      " didn't parse as KEY=value. Open the raw file to read them."));
  }
}

function renderLegend(version, added, changed) {
  const legend = el("div", "legend");
  const entry = (kind, count, word) => {
    const s = el("span");
    s.append(el("span", "swatch " + kind),
      document.createTextNode(count + " " + word + " since v" + (version - 1)));
    return s;
  };
  if (added) legend.append(entry("added", added, "added"));
  if (changed) legend.append(entry("changed", changed, "changed"));
  return legend;
}

function renderDetail() {
  renderDetailHead();
  renderDetailBody();
  $("main").dataset.view = state.detail ? "detail" : "list";
}

function render() {
  renderHeader();
  renderFiles();
  renderDetail();
}

// ---------- data ----------

async function refresh() {
  const s = await api("/api/state");
  state.files = s.files;
  state.archives = s.archives || [];
  state.config = s.config;
  state.daemon = s.daemon;
  state.vaultDir = s.vault_dir;

  // A file may have been forgotten out from under the selection.
  if (!state.peek && state.selected !== null && state.selected >= activeFiles().length) {
    state.selected = null;
    state.detail = null;
  }
  render();
}

function selectFile(i) {
  state.selected = i;
  state.reveal.clear();
  renderFiles();
  loadVersion();
}

function loadVersion(version) {
  const file = currentFile();
  if (!file) return;

  return run(async () => {
    let res, snapshots;
    const parked = state.showArchived ? "&archived=1" : "";
    if (state.peek) {
      res = await api("/api/peek/content", "POST",
        { path: state.peek.path, file: file.index, version: version || 0 });
      snapshots = res.snapshots;
    } else {
      const history = await api("/api/history?file=" + file.index + parked);
      res = await api("/api/content?file=" + file.index + (version ? "&version=" + version : "") + parked);
      snapshots = history.snapshots;
    }

    // Failing to read the previous version costs the diff marks, not the view.
    let prev = null;
    if (res.version > 1) {
      try {
        const p = state.peek
          ? await api("/api/peek/content", "POST",
            { path: state.peek.path, file: file.index, version: res.version - 1 })
          : await api("/api/content?file=" + file.index + "&version=" + (res.version - 1) + parked);
        prev = p.content;
      } catch (e) {
        prev = null;
      }
    }

    state.detail = {
      display: res.display, path: res.path, snapshots,
      version: res.version, content: res.content, prev,
    };
    state.reveal.clear();
    render();
  });
}

// ---------- actions ----------

function scanNow() {
  run(async () => {
    const res = await api("/api/scan", "POST", {});
    toast(res.backed
      ? "Scanned " + res.found + " files · " + res.backed + " new " + (res.backed === 1 ? "snapshot" : "snapshots")
      : "Scanned " + res.found + " files · nothing changed");
    await refresh();
  });
}

function toggleDaemon() {
  run(async () => {
    const res = await api("/api/daemon", "POST", { action: state.daemon.running ? "stop" : "start" });
    toast(res.running ? "Watching for changes · pid " + res.pid : "Stopped watching");
    await refresh();
  });
}

function addWatchDir() {
  run(async () => {
    const dir = await ask("Watch a folder", "envault scans it for .env files from now on.", "~/", "Watch folder");
    if (!dir) return;
    const res = await api("/api/watch", "POST", { dir });
    toast("Watching " + res.dir);
    await refresh();
  });
}

function restoreInPlace() {
  const d = state.detail;
  run(async () => {
    const path = await ask("Restore v" + d.version + " over the live file",
      "The file at this path is overwritten with this version.", d.path, "Restore");
    if (!path) return;
    const res = await api("/api/restore", "POST", { file: currentFile().index, version: d.version, path });
    toast("Restored v" + d.version + " to " + res.path);
    await refresh();
  });
}

function restoreElsewhere(archived) {
  const d = state.detail;
  run(async () => {
    const path = await ask("Write v" + d.version + " to a new file",
      "Absolute path, or start with ~/ for your home folder.", "", "Write file");
    if (!path) return;
    const res = await api("/api/restore", "POST",
      { file: currentFile().index, version: d.version, path, archived: !!archived });
    toast("Wrote " + res.path);
  });
}

// archiveFile parks the whole history: it stops being scanned or listed but
// keeps every version and can be brought back unchanged.
function archiveFile() {
  const d = state.detail;
  const file = currentFile();
  const { name } = splitPath(d.display);

  run(async () => {
    await api("/api/archive", "POST", { file: file.index });
    state.selected = null;
    state.detail = null;
    toast("Archived " + name + ". It is no longer scanned; find it under Archived.");
    await refresh();
  });
}

function unarchiveFile() {
  const d = state.detail;
  const file = currentFile();
  const { name } = splitPath(d.display);

  run(async () => {
    await api("/api/unarchive", "POST", { file: file.index });
    state.selected = null;
    state.detail = null;
    toast("Brought back " + name + ". Scan to pick up where it left off.");
    await refresh();
  });
}

// forgetFile deletes history that cannot be recovered, so it asks the user to
// type the filename rather than accepting a single click.
function forgetFile() {
  const d = state.detail;
  const file = currentFile();
  const { name } = splitPath(d.display);

  run(async () => {
    const typed = await ask("Stop tracking this file",
      "Deletes every stored version. The file on disk is left alone. Type " + name + " to confirm.",
      "", "Stop tracking");
    if (!typed) return;
    if (typed !== name && typed !== d.display) {
      toast("Left it alone — that didn't match " + name, true);
      return;
    }
    await api("/api/forget", "POST", { file: file.index });
    state.selected = null;
    state.detail = null;
    toast("Stopped tracking " + name + ". Reclaim space to free the disk.");
    await refresh();
  });
}

function reclaim() {
  run(async () => {
    const res = await api("/api/gc", "POST", {});
    toast(res.removed
      ? "Freed " + res.human_freed + " from " + res.removed + " unused " + (res.removed === 1 ? "blob" : "blobs")
      : "Nothing to reclaim");
    await refresh();
  });
}

// exportVault streams the zip through the browser's own download machinery, so
// the user picks where it lands.
function exportVault() {
  run(async () => {
    const res = await fetch("/api/export", { headers: { "X-Envault-Token": TOKEN } });
    if (!res.ok) throw new Error("Export failed: " + res.status + " " + res.statusText);

    const match = (res.headers.get("Content-Disposition") || "").match(/filename="?([^"]+)"?/);
    const url = URL.createObjectURL(await res.blob());
    const a = document.createElement("a");
    a.href = url;
    a.download = match ? match[1] : "envault-backup.zip";
    a.click();
    URL.revokeObjectURL(url);
    toast("Downloaded " + a.download);
  });
}

function importVault() {
  run(async () => {
    const path = await ask("Import a backup zip",
      "Replaces the vault on this machine with the archive's contents.", "", "Import");
    if (!path) return;
    const force = confirm("Overwrite the vault at " + state.vaultDir +
      "?\n\nOK replaces it. Cancel imports only if the vault is empty.");
    const res = await api("/api/import", "POST", { path, force });
    toast("Imported " + res.files + " files");
    await refresh();
  });
}

function peekArchive() {
  run(async () => {
    const path = await ask("Read a backup zip",
      "Opens the archive read-only. Your vault is never touched.", "", "Open archive");
    if (!path) return;
    const res = await api("/api/peek", "POST", { path });
    state.peek = { path, files: res.files };
    state.selected = null;
    state.detail = null;
    state.filter = "";
    $("filterInput").value = "";
    render();
    toast("Reading " + path);
  });
}

function leaveArchive() {
  state.peek = null;
  state.selected = null;
  state.detail = null;
  render();
}

function saveFromArchive() {
  const d = state.detail;
  run(async () => {
    const out = await ask("Save v" + d.version + " out of the archive",
      "Absolute path, or start with ~/ for your home folder.", "", "Save file");
    if (!out) return;
    const res = await api("/api/peek/save", "POST",
      { path: state.peek.path, file: currentFile().index, version: d.version, out });
    toast("Saved " + res.out + " · your vault is unchanged");
  });
}

// toggleArchived flips the sidebar between the live index and the archive
// shelf. The two listings share selection state, so it resets on the way in.
function toggleArchived() {
  if (state.peek) return;
  state.showArchived = !state.showArchived;
  state.selected = null;
  state.detail = null;
  state.filter = "";
  $("filterInput").value = "";
  $("filterInput").placeholder = state.showArchived ? "Filter archived" : "Filter files";
  render();
}

function openSettings() {
  $("intervalInput").value = state.config.scan_interval_secs;
  $("maxVersionsInput").value = state.config.max_versions;
  $("watchDirsInput").value = (state.config.watch_dirs || []).join("\n");
  $("settingsDialog").showModal();
}

$("settingsDialog").addEventListener("close", function () {
  if (this.returnValue !== "ok") return;
  run(async () => {
    await api("/api/config", "POST", {
      scan_interval_secs: Number($("intervalInput").value),
      max_versions: Number($("maxVersionsInput").value),
      watch_dirs: $("watchDirsInput").value.split("\n").map(s => s.trim()).filter(Boolean),
    });
    toast("Settings saved. Restart watching to pick up the new interval.");
    await refresh();
  });
});

// ---------- command bar ----------

const COMMANDS = [
  { key: "s", icon: "scan",    label: "Scan",     run: scanNow },
  { key: "w", icon: "folder",  label: "Watch",    run: addWatchDir },
  { key: "a", icon: "archive", label: "Archived", run: toggleArchived },
  { key: "e", icon: "save",    label: "Export",   run: exportVault },
  { key: "i", icon: "upload",  label: "Import",   run: importVault },
  { key: "p", icon: "archive", label: "Read zip", run: peekArchive },
  { key: "x", icon: "broom",   label: "Reclaim",  run: reclaim },
  { key: ",", icon: "gear",    label: "Settings", run: openSettings },
];

const SHORTCUTS = [
  ["j / ↓", "Next file"],
  ["k / ↑", "Previous file"],
  ["g / G", "First / last file"],
  ["Esc", "Back to the file list"],
  ["/", "Filter files"],
  ["r", "Reveal or hide values"],
  ["t", "Raw file"],
  ["c", "Copy this version"],
  ["1…9", "Jump to a version"],
  ["s w a e i p x ,", "The commands along the bottom"],
];

function renderCommands() {
  const bar = $("commandBar");
  bar.textContent = "";

  COMMANDS.forEach(c => {
    const b = el("button", "cmd");
    b.append(icon(c.icon, 14), el("span", "key", c.key), el("span", null, c.label));
    b.onclick = c.run;
    bar.append(b);
  });

  bar.append(el("span", "sep"));

  const help = el("button", "cmd");
  help.append(icon("keyboard", 14), el("span", "key", "?"), el("span", null, "Keyboard"));
  help.onclick = openKeys;
  bar.append(help);
}

function openKeys() {
  const box = $("keysList");
  box.textContent = "";
  SHORTCUTS.forEach(([k, what]) => box.append(el("kbd", null, k), el("span", "what", what)));
  $("keysDialog").showModal();
}

// ---------- keyboard ----------

document.addEventListener("keydown", e => {
  const tag = document.activeElement && document.activeElement.tagName;
  if (tag === "INPUT" || tag === "TEXTAREA") {
    if (e.key === "Escape") document.activeElement.blur();
    return;
  }
  if (document.querySelector("dialog[open]")) return;
  if (e.metaKey || e.ctrlKey || e.altKey) return;
  // The divider owns the arrow keys while it has focus.
  if (document.activeElement === resizer &&
      ["ArrowLeft", "ArrowRight", "Home", "End"].includes(e.key)) return;

  const list = activeFiles();

  switch (e.key) {
    case "/":
      e.preventDefault();
      $("filterInput").focus();
      return;
    case "j": case "ArrowDown":
      if (!list.length) return;
      e.preventDefault();
      selectFile(state.selected === null ? 0 : Math.min(state.selected + 1, list.length - 1));
      return;
    case "k": case "ArrowUp":
      if (!list.length) return;
      e.preventDefault();
      selectFile(state.selected === null ? 0 : Math.max(state.selected - 1, 0));
      return;
    case "g":
      if (list.length) selectFile(0);
      return;
    case "G":
      if (list.length) selectFile(list.length - 1);
      return;
    case "Escape":
      if (state.detail) { state.detail = null; state.selected = null; render(); }
      else if (state.peek) leaveArchive();
      return;
    case "r":
      if (state.detail) { state.revealAll = !state.revealAll; state.reveal.clear(); renderDetail(); }
      return;
    case "t":
      if (state.detail) { state.raw = !state.raw; renderDetail(); }
      return;
    case "c":
      if (state.detail) copy(state.detail.content, "v" + state.detail.version);
      return;
    case "?":
      openKeys();
      return;
  }

  if (state.detail && e.key >= "1" && e.key <= "9") {
    const v = Number(e.key);
    if (v <= state.detail.snapshots.length) loadVersion(v);
    return;
  }

  const cmd = COMMANDS.find(c => c.key === e.key);
  if (cmd) { e.preventDefault(); cmd.run(); }
});

// ---------- boot ----------

$("filterInput").addEventListener("input", e => {
  state.filter = e.target.value.trim();
  state.selected = null;
  state.detail = null;
  render();
});

$("daemonToggle").onclick = toggleDaemon;

renderCommands();
run(refresh);
