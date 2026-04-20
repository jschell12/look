const grid = document.getElementById('grid');
const count = document.getElementById('count');
const modelSelect = document.getElementById('model-select');
const relaySelect = document.getElementById('relay-select');
const relayStatusEl = document.getElementById('relay-status');
const toast = document.getElementById('toast');
const projectTabs = document.getElementById('project-tabs');
const addProjectBtn = document.getElementById('add-project');

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
let expandedCard = null; // imgPath of card with conversation open

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
  // Simple URL regex that matches http/https URLs
  const urlRegex = /(https?:\/\/[^\s]+)/g;
  return text.replace(urlRegex, '<a href="$1" class="conv-link" target="_blank" rel="noopener">$1</a>');
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

// ── Helper function to make links clickable ──

function makeLinksClickable(text) {
  // Regular expression to match URLs
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
    const isExpanded = expandedCard === img.path;
    const hasConversation = img.conversation && img.conversation.length > 0;
    const card = document.createElement('div');
    card.className = 'card' + (isProcessing ? ' card-processing' : '') + (isExpanded ? ' card-expanded' : '');

    const imgEl = document.createElement('img');
    imgEl.src = `file://${img.path}`;
    imgEl.loading = 'lazy';
    card.appendChild(imgEl);

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

    // Conversation panel (when expanded or has history)
    if (isExpanded && hasConversation) {
      const convEl = document.createElement('div');
      convEl.className = 'conversation';
      convEl.id = `conv-${CSS.escape(img.path)}`;

      for (const msg of img.conversation) {
        const msgEl = document.createElement('div');
        msgEl.className = `conv-msg conv-${msg.role}`;

        // Use makeLinksClickable for message content
        const contentEl = makeLinksClickable(msg.text);
        msgEl.appendChild(contentEl);
        
        convEl.appendChild(msgEl);
      }

      card.appendChild(convEl);
      requestAnimationFrame(() => { convEl.scrollTop = convEl.scrollHeight; });

      // Follow-up input
      if (!isProcessing) {
        const inputRow = document.createElement('div');
        inputRow.className = 'conv-input-row';
        const input = document.createElement('input');
        input.type = 'text';
        input.className = 'conv-input';
        input.placeholder = 'Follow up\u2026';
        input.addEventListener('click', (e) => e.stopPropagation());
        const sendBtn = document.createElement('button');
        sendBtn.className = 'conv-send-btn';
        sendBtn.textContent = '\u25B6';
        sendBtn.addEventListener('click', (e) => {
          e.stopPropagation();
          const text = input.value.trim();
          if (!text) return;
          input.value = '';
          sendFollowup(img, text);
        });
        input.addEventListener('keydown', (e) => {
          if (e.key === 'Enter') {
            e.stopPropagation();
            sendBtn.click();
          }
        });
        inputRow.appendChild(input);
        inputRow.appendChild(sendBtn);
        card.appendChild(inputRow);
      }
    }

    // Chat indicator / expand toggle for cards with conversation
    if (hasConversation && !isExpanded) {
      const chatBtn = document.createElement('button');
      chatBtn.className = 'chat-btn';
      chatBtn.textContent = `\u{1F4AC} ${img.conversation.length}`;
      chatBtn.title = 'View conversation';
      chatBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        expandedCard = img.path;
        refresh();
      });
      card.appendChild(chatBtn);
    }

    // Send button
    if (!isProcessing && status !== 'done') {
      const sendBtn = document.createElement('button');
      sendBtn.className = 'send-btn';
      sendBtn.textContent = '\u25B6';
      sendBtn.title = 'Send to Claude';
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

    // Click card to toggle conversation
    if (hasConversation) {
      card.addEventListener('click', () => {
        expandedCard = expandedCard === img.path ? null : img.path;
        refresh();
      });
    }

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

  const modal = document.createElement('div');
  modal.id = 'context-modal';
  modal.className = 'modal-overlay';
  modal.innerHTML = `
    <div class="modal">
      <div class="modal-title">Send to Claude</div>
      <div class="modal-subtitle">${img.name}</div>
      <label class="modal-label">Project</label>
      <select id="project-select">${projectOptions}</select>
      <label class="modal-label">Context</label>
      <textarea id="context-input" placeholder="What's wrong? What should be fixed?" rows="3"></textarea>
      <div class="modal-actions">
        <button id="modal-cancel" class="link-btn">Cancel</button>
        <button id="modal-relay" class="link-btn" style="display:none;">Relay</button>
        <button id="modal-send" class="modal-send-btn" ${projects.length === 0 ? 'disabled' : ''}>Send</button>
      </div>
    </div>
  `;
  document.body.appendChild(modal);

  const contextInput = document.getElementById('context-input');
  const projectSelect = document.getElementById('project-select');
  projectSelect.focus();

  document.getElementById('modal-cancel').addEventListener('click', () => modal.remove());
  modal.addEventListener('click', (e) => { if (e.target === modal) modal.remove(); });

  // Show relay button if relay host is configured
  const relayHost = relaySelect.value;
  const relayBtn = document.getElementById('modal-relay');
  if (relayHost && relayHost !== '_scan' && relayHost !== '') {
    relayBtn.style.display = '';
    relayBtn.textContent = 'Relay to ' + relayHost;
    relayBtn.addEventListener('click', () => {
      const projectPath = projectSelect.value;
      if (!projectPath) return;
      const message = contextInput.value.trim();
      modal.remove();
      relayImage(img, projectPath, message);
    });
  }

  const doSend = () => {
    const projectPath = projectSelect.value;
    if (!projectPath) return;
    const message = contextInput.value.trim();
    modal.remove();
    sendImage(img, projectPath, message);
  };

  document.getElementById('modal-send').addEventListener('click', doSend);
  contextInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) doSend();
  });
}

async function sendImage(img, projectPath, message) {
  processingSet.add(img.path);
  progressLogs[img.path] = [];
  expandedCard = img.path;
  const images = await window.xmuggle.getImages();
  render(images);

  try {
    const result = await window.xmuggle.sendToApi([img.path], projectPath, message || '');
    processingSet.delete(img.path);

    if (result.status === 'success') {
      const prInfo = result.prUrl ? ` PR: ${result.prUrl}` : '';
      showToast(`Fixed: ${result.summary}${prInfo}`, false);
      
      // Add the success message to the conversation
      if (result.prUrl) {
        const successMessage = `Fixed: ${result.summary} PR: ${result.prUrl}`;
        await addToConversation(img.path, 'assistant', successMessage);
      }
    } else if (result.status === 'no_changes') {
      showToast(result.summary, false);
    } else {
      showToast(`Error: ${result.summary}`, true);
    }
  } catch (err) {
    processingSet.delete(img.path);
    showToast(`Error: ${err.message}`, true);
  }

  delete progressLogs[img.path];
  const updated = await window.xmuggle.getImages();
  render(updated);
}

async function sendFollowup(img, message) {
  processingSet.add(img.path);
  progressLogs[img.path] = [];
  const images = await window.xmuggle.getImages();
  render(images);

  try {
    const result = await window.xmuggle.sendFollowup(img.path, message);
    processingSet.delete(img.path);

    if (result.status === 'success') {
      const prInfo = result.prUrl ? ` PR: ${result.prUrl}` : '';
      showToast(`Fixed: ${result.summary}${prInfo}`, false);
      
      // Add the success message to the conversation
      if (result.prUrl) {
        const successMessage = `Fixed: ${result.summary} PR: ${result.prUrl}`;
        await addToConversation(img.path, 'assistant', successMessage);
      }
    } else if (result.status === 'no_changes') {
      showToast(result.summary, false);
    } else {
      showToast(`Error: ${result.summary}`, true);
    }
  } catch (err) {
    processingSet.delete(img.path);
    showToast(`Error: ${err.message}`, true);
  }

  delete progressLogs[img.path];
  const updated = await window.xmuggle.getImages();
  render(updated);
}

// Helper function to add messages to conversation (this would need to be implemented in the backend)
async function addToConversation(imgPath, role, text) {
  // This is a placeholder - the actual implementation would need to be added
  // to store the message in the conversation history
  console.log(`Adding to conversation for ${imgPath}: [${role}] ${text}`);
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

    // If no peers found, add git sync option
    if (hosts.length === 0) {
      const gitOpt = document.createElement('option');
      gitOpt.value = '_git';
      gitOpt.textContent = syncRepo ? 'Git sync' : 'Git sync (configure...)';
      if (saved === '_git') gitOpt.selected = true;
      relaySelect.appendChild(gitOpt);
      relayStatusEl.textContent = 'No peers — git sync available';
      relayStatusEl.style.color = '#f0a500';
    } else {
      // Still add git as an option
      const gitOpt = document.createElement('option');
      gitOpt.value = '_git';
      gitOpt.textContent = 'Git sync';
      relaySelect.appendChild(gitOpt);
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

// ── Model Selector ──

async function initModelSelect() {
  const models = await window.xmuggle.listModels();
  const current = await window.xmuggle.getModel();
  modelSelect.innerHTML = '';
  for (const m of models) {
    const opt = document.createElement('option');
    opt.value = m.id;
    opt.textContent = m.label;
    if (m.id === current) opt.selected = true;
    modelSelect.appendChild(opt);
  }
}

modelSelect.addEventListener('change', async () => {
  await window.xmuggle.setModel(modelSelect.value);
  const label = modelSelect.options[modelSelect.selectedIndex].textContent;
  showToast(`Model: ${label}`, false);
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
  // Update the log element in-place if it exists, otherwise re-render
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
initModelSelect();
initRelay();
loadProjects();
refresh();