const grid = document.getElementById('grid');
const count = document.getElementById('count');
const relaySelect = document.getElementById('relay-select');
const relayStatusEl = document.getElementById('relay-status');
const toast = document.getElementById('toast');
const projectTabs = document.getElementById('project-tabs');
const addProjectBtn = document.getElementById('add-project');
const addNoteBtn = document.getElementById('add-note');

const BADGE_LABELS = {
  new: 'New',
  pending: 'Pending',
  queued: 'Queued',
  processing: 'Processing',
  done: 'Done',
  error: 'Error',
};

let projects = [];
let activeProject = null; // path of selected project, or null for "all"
const processingSet = new Set();
const progressLogs = {}; // imgPath -> [messages]

// ── Toast ──

function showToast(msg, isError) {
  toast.innerHTML = '';
  const text = document.createElement('span');
  text.textContent = msg;
  const closeBtn = document.createElement('button');
  closeBtn.className = 'toast-close';
  closeBtn.textContent = '\u00d7';
  closeBtn.addEventListener('click', () => toast.className = 'toast hidden');
  toast.appendChild(text);
  toast.appendChild(closeBtn);
  toast.className = `toast ${isError ? 'toast-error' : 'toast-success'}`;
}

// ── URL Detection ──

function makeLinksClickable(text) {
  const urlRegex = /(https?:\/\/[^\s]+)/g;
  const parts = text.split(urlRegex);

  const container = document.createElement('span');

  parts.forEach(part => {
    if (urlRegex.test(part)) {
      const link = document.createElement('a');
      link.href = part;
      link.textContent = part;
      link.target = '_blank';
      link.rel = 'noopener noreferrer';
      link.style.color = '#74b9ff';
      link.style.textDecoration = 'underline';
      link.addEventListener('click', (e) => {
        e.preventDefault();
        e.stopPropagation();
        window.xmuggle.openExternal(part);
      });
      container.appendChild(link);
    } else {
      container.appendChild(document.createTextNode(part));
    }
  });

  return container;
}

// ── Projects ──

async function loadProjects() {
  projects = await window.xmuggle.listProjects();
  renderProjectTabs();
}

function renderProjectTabs() {
  projectTabs.innerHTML = '';

  // "All" tab
  const allTab = document.createElement('div');
  allTab.className = 'project-tab' + (activeProject === null ? ' project-tab-active' : '');
  allTab.textContent = 'All';
  allTab.addEventListener('click', () => {
    activeProject = null;
    renderProjectTabs();
    refresh();
  });
  projectTabs.appendChild(allTab);

  for (const p of projects) {
    const tab = document.createElement('div');
    tab.className = 'project-tab' + (activeProject === p.path ? ' project-tab-active' : '');
    tab.title = p.path;

    const nameSpan = document.createElement('span');
    nameSpan.textContent = p.name;
    tab.appendChild(nameSpan);

    tab.addEventListener('click', () => {
      activeProject = activeProject === p.path ? null : p.path;
      renderProjectTabs();
      refresh();
    });

    const removeBtn = document.createElement('button');
    removeBtn.className = 'project-remove';
    removeBtn.textContent = '\u00d7';
    removeBtn.addEventListener('click', async (e) => {
      e.stopPropagation();
      await window.xmuggle.removeProject(p.path);
      if (activeProject === p.path) activeProject = null;
      await loadProjects();
      refresh();
    });
    tab.appendChild(removeBtn);
    projectTabs.appendChild(tab);
  }
}

addProjectBtn.addEventListener('click', async () => {
  const result = await window.xmuggle.addProject();
  if (result && !result.error) {
    await loadProjects();
    showToast(`Added project: ${result.name}`, false);
  } else if (result && result.error) {
    showToast(result.error, true);
  }
});

// ── Paste text note ──

addNoteBtn.addEventListener('click', () => {
  const existing = document.getElementById('note-modal');
  if (existing) existing.remove();

  const modal = document.createElement('div');
  modal.id = 'note-modal';
  modal.className = 'modal-overlay';
  modal.innerHTML = `
    <div class="modal">
      <div class="modal-title">Paste text</div>
      <div class="modal-subtitle">Saved as a text note you can send like a screenshot</div>
      <textarea id="note-text-input" placeholder="Paste error message, stack trace, log, etc\u2026" rows="10"></textarea>
      <div class="modal-actions">
        <button id="note-cancel" class="link-btn">Cancel</button>
        <button id="note-save" class="modal-send-btn">Save</button>
      </div>
    </div>
  `;
  document.body.appendChild(modal);

  const textInput = document.getElementById('note-text-input');
  textInput.focus();
  modal.addEventListener('click', (e) => { if (e.target === modal) modal.remove(); });
  document.getElementById('note-cancel').addEventListener('click', () => modal.remove());

  const save = async () => {
    const text = textInput.value.trim();
    if (!text) return;
    modal.remove();
    try {
      const note = await window.xmuggle.createNote(text);
      showToast(`Saved ${note.name}`, false);
      await refresh();
    } catch (err) {
      showToast(`Error: ${err.message}`, true);
    }
  };
  document.getElementById('note-save').addEventListener('click', save);
  textInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) save();
  });
});

// ── Images ──

function render(images) {
  grid.innerHTML = '';

  // Filter by active project
  const filtered = activeProject
    ? images.filter(i => i.projectPath === activeProject || (!i.projectPath && i.status === 'new'))
    : images;

  const total = filtered.length;
  const pending = filtered.filter(i => i.status === 'new').length;
  const inProgress = filtered.filter(i => i.status === 'processing' || processingSet.has(i.path)).length;
  const done = filtered.filter(i => i.status === 'done').length;
  const label = activeProject ? activeProject.split('/').pop() : 'all projects';
  count.textContent = `${total} images \u2022 ${pending} new \u2022 ${inProgress} in progress \u2022 ${done} done \u2022 ${label}`;

  for (const img of filtered) {
    const isProcessing = processingSet.has(img.path);
    const isText = img.type === 'text';
    const card = document.createElement('div');
    card.className = 'card'
      + (isText ? ' card-text' : '')
      + (isProcessing ? ' card-processing' : '');

    if (isText) {
      const textEl = document.createElement('div');
      textEl.className = 'text-preview';
      textEl.textContent = img.preview || '';
      card.appendChild(textEl);
    } else {
      const imgEl = document.createElement('img');
      imgEl.src = `file://${img.path}`;
      imgEl.loading = 'lazy';
      card.appendChild(imgEl);
    }

    const status = isProcessing ? 'processing' : img.status;
    const badge = document.createElement('span');
    badge.className = `badge ${status}`;
    badge.textContent = BADGE_LABELS[status] || status;
    card.appendChild(badge);

    // Project label if assigned
    if (img.projectPath) {
      const projLabel = document.createElement('div');
      projLabel.className = 'project-label';
      projLabel.textContent = img.projectPath.split('/').pop();
      card.appendChild(projLabel);
    }

    // Progress log (during processing)
    if (isProcessing && progressLogs[img.path] && progressLogs[img.path].length > 0) {
      const logEl = document.createElement('div');
      logEl.className = 'progress-log';
      logEl.id = `log-${CSS.escape(img.path)}`;
      for (const msg of progressLogs[img.path]) {
        const line = document.createElement('div');
        line.className = 'progress-line';
        line.textContent = msg;
        logEl.appendChild(line);
      }
      card.appendChild(logEl);
      requestAnimationFrame(() => { logEl.scrollTop = logEl.scrollHeight; });
    }

    // Send button
    if (!isProcessing && status !== 'done') {
      const sendBtn = document.createElement('button');
      sendBtn.className = 'send-btn';
      sendBtn.textContent = '\u25B6';
      sendBtn.title = 'Send';
      sendBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        promptAndSend(img);
      });
      card.appendChild(sendBtn);
    }

    // Delete button
    const deleteBtn = document.createElement('button');
    deleteBtn.className = 'delete-btn';
    deleteBtn.textContent = '\u00d7';
    deleteBtn.title = 'Delete screenshot';
    deleteBtn.addEventListener('click', async (e) => {
      e.stopPropagation();
      const images = await window.xmuggle.deleteImage(img.path);
      render(images);
    });
    card.appendChild(deleteBtn);

    const name = document.createElement('div');
    name.className = 'name';
    name.textContent = img.name;
    name.title = img.name;
    card.appendChild(name);

    grid.appendChild(card);
  }
}

// ── Send Modal ──

function promptAndSend(img) {
  const existing = document.getElementById('context-modal');
  if (existing) existing.remove();

  let projectOptions = '';
  for (const p of projects) {
    const selected = (activeProject === p.path) ? ' selected' : '';
    projectOptions += `<option value="${p.path}"${selected}>${p.name}</option>`;
  }
  if (projects.length === 0) {
    projectOptions = '<option value="">No projects \u2014 add one first</option>';
  }

  const relayHost = relaySelect.value;
  const sendLabel = (relayHost && relayHost !== '_scan') ? 'Send' : 'Save';

  const modal = document.createElement('div');
  modal.id = 'context-modal';
  modal.className = 'modal-overlay';
  modal.innerHTML = `
    <div class="modal">
      <div class="modal-title">Send</div>
      <div class="modal-subtitle">${img.name}</div>
      <label class="modal-label">Project</label>
      <select id="project-select">${projectOptions}</select>
      <label class="modal-label">Context</label>
      <textarea id="context-input" placeholder="What's wrong? What should be fixed?" rows="3"></textarea>
      <div class="modal-actions">
        <button id="modal-cancel" class="link-btn">Cancel</button>
        <button id="modal-send" class="modal-send-btn" ${projects.length === 0 ? 'disabled' : ''}>${sendLabel}</button>
      </div>
    </div>
  `;
  document.body.appendChild(modal);

  const contextInput = document.getElementById('context-input');
  const projectSelect = document.getElementById('project-select');
  projectSelect.focus();

  document.getElementById('modal-cancel').addEventListener('click', () => modal.remove());
  modal.addEventListener('click', (e) => { if (e.target === modal) modal.remove(); });

  const doSend = () => {
    const projectPath = projectSelect.value;
    if (!projectPath) return;
    const message = contextInput.value.trim();
    modal.remove();

    if (relayHost && relayHost !== '_scan' && relayHost !== '') {
      relayImage(img, projectPath, message);
    } else {
      saveItem(img, projectPath, message);
    }
  };

  document.getElementById('modal-send').addEventListener('click', doSend);
  contextInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) doSend();
  });
}

async function saveItem(img, projectPath, message) {
  try {
    await window.xmuggle.saveItem(img.path, projectPath, message || '');
    showToast('Saved', false);
  } catch (err) {
    showToast(`Error: ${err.message}`, true);
  }
  const updated = await window.xmuggle.getImages();
  render(updated);
}

async function relayImage(img, projectPath, message) {
  processingSet.add(img.path);
  const images = await window.xmuggle.getImages();
  render(images);

  try {
    const relayHost = relaySelect.value;
    let result;
    if (relayHost === '_git') {
      result = await window.xmuggle.gitSyncPush(img.path, projectPath, message || '');
      showToast('Synced via git: ' + (result.id || 'sent'), false);
    } else {
      result = await window.xmuggle.sendToRelay(img.path, projectPath, message || '');
      showToast('Sent to relay: ' + (result.path || 'received'), false);
    }
    processingSet.delete(img.path);
  } catch (err) {
    processingSet.delete(img.path);
    showToast('Relay error: ' + err.message, true);
  }

  const updated = await window.xmuggle.getImages();
  render(updated);
}

// ── Relay Network ──

let discoveredHosts = [];

async function initRelay() {
  const saved = await window.xmuggle.getRelayHost();
  const syncRepo = await window.xmuggle.getSyncRepo();

  // Add default options
  relaySelect.innerHTML = '<option value="">Local (no relay)</option><option value="_scan">Scanning network...</option>';
  if (saved) relaySelect.value = saved;

  // Scan network in background
  try {
    const hosts = await window.xmuggle.scanNetwork();
    discoveredHosts = hosts;
    relaySelect.innerHTML = '<option value="">Local (no relay)</option>';
    for (const h of hosts) {
      const opt = document.createElement('option');
      opt.value = h.ip;
      opt.textContent = `${h.hostname} (${h.ip})`;
      if (h.ip === saved) opt.selected = true;
      relaySelect.appendChild(opt);
    }

    // Git sync option
    const gitOpt = document.createElement('option');
    gitOpt.value = '_git';
    gitOpt.textContent = syncRepo ? 'Git sync' : 'Git sync (configure...)';
    relaySelect.appendChild(gitOpt);

    if (hosts.length === 0) {
      // Default to git sync when no local peers found
      if (!saved || saved === '_scan') {
        relaySelect.value = '_git';
        await window.xmuggle.setRelayHost('_git');
      }
      relayStatusEl.textContent = 'No peers — using git sync';
      relayStatusEl.style.color = '#f0a500';
    } else {
      relayStatusEl.textContent = hosts.length + ' peer(s)';
      relayStatusEl.style.color = '#00b894';
    }

    // Add rescan option
    const rescan = document.createElement('option');
    rescan.value = '_scan';
    rescan.textContent = 'Rescan...';
    relaySelect.appendChild(rescan);
  } catch {
    relaySelect.innerHTML = '<option value="">Local (no relay)</option>';
    relayStatusEl.textContent = 'Scan failed';
    relayStatusEl.style.color = '#d63031';
  }
}

relaySelect.addEventListener('change', async () => {
  const val = relaySelect.value;
  if (val === '_scan') {
    relayStatusEl.textContent = 'Scanning...';
    relayStatusEl.style.color = '#f0a500';
    await initRelay();
    return;
  }
  if (val === '_git') {
    const syncRepo = await window.xmuggle.getSyncRepo();
    if (!syncRepo) {
      const repo = prompt('Enter git repo URL for sync (e.g. git@github.com:user/xmuggle-sync.git):');
      if (repo) {
        await window.xmuggle.setSyncRepo(repo);
        showToast('Git sync repo set: ' + repo, false);
      } else {
        relaySelect.value = '';
        return;
      }
    }
    await window.xmuggle.setRelayHost('_git');
    showToast('Using git sync', false);
    return;
  }
  await window.xmuggle.setRelayHost(val);
  if (val) {
    showToast('Relay: ' + val, false);
  } else {
    showToast('Relay disabled (local mode)', false);
  }
});

// ── Init ──

async function refresh() {
  const images = await window.xmuggle.getImages();
  render(images);
}

window.xmuggle.onImagesUpdated((images) => render(images));
window.xmuggle.onTaskProgress((imgPath, msg) => {
  if (!progressLogs[imgPath]) progressLogs[imgPath] = [];
  progressLogs[imgPath].push(msg);
  const logEl = document.getElementById(`log-${CSS.escape(imgPath)}`);
  if (logEl) {
    const line = document.createElement('div');
    line.className = 'progress-line';
    line.textContent = msg;
    logEl.appendChild(line);
    logEl.scrollTop = logEl.scrollHeight;
  } else {
    refresh();
  }
});
initRelay();
loadProjects();
refresh();
