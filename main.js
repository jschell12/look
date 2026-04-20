const { app, BrowserWindow, ipcMain, shell, dialog } = require('electron');
const http = require('http');
const path = require('path');
const fs = require('fs');
const os = require('os');

const DESKTOP = path.join(os.homedir(), 'Desktop');
const XMUGGLE_DIR = path.join(os.homedir(), '.xmuggle');
const PROJECTS_FILE = path.join(XMUGGLE_DIR, 'projects.json');
const TASKS_FILE = path.join(XMUGGLE_DIR, 'tasks.json');
const INBOX_DIR = path.join(XMUGGLE_DIR, 'inbox');
const SERVER_PORT = 24816;
const IMAGE_EXTS = new Set(['.png', '.jpg', '.jpeg', '.webp', '.gif']);

// ── Projects ──

function loadProjects() {
  try { return JSON.parse(fs.readFileSync(PROJECTS_FILE, 'utf8')); } catch { return []; }
}

function saveProjects(list) {
  fs.mkdirSync(XMUGGLE_DIR, { recursive: true });
  fs.writeFileSync(PROJECTS_FILE, JSON.stringify(list, null, 2) + '\n');
}

function addProject(dirPath) {
  const abs = path.resolve(dirPath);
  if (!fs.existsSync(path.join(abs, '.git'))) return { error: 'Not a git repo' };
  const projects = loadProjects();
  if (!projects.includes(abs)) {
    projects.push(abs);
    saveProjects(projects);
  }
  return { path: abs, name: path.basename(abs) };
}

function removeProject(dirPath) {
  const projects = loadProjects().filter(p => p !== dirPath);
  saveProjects(projects);
}

function listProjects() {
  return loadProjects().map(p => ({ path: p, name: path.basename(p) }));
}

// ── Tasks ──

function loadTasks() {
  try { return JSON.parse(fs.readFileSync(TASKS_FILE, 'utf8')); } catch { return {}; }
}

function saveTasks(tasks) {
  fs.mkdirSync(XMUGGLE_DIR, { recursive: true });
  fs.writeFileSync(TASKS_FILE, JSON.stringify(tasks, null, 2) + '\n');
}

function updateTaskStatus(imagePath, projectPath, taskId, status, prUrl, conversation, apiMessages) {
  const tasks = loadTasks();
  const existing = tasks[imagePath] || {};
  tasks[imagePath] = {
    projectPath,
    taskId: taskId || existing.taskId,
    status,
    prUrl: prUrl || existing.prUrl || '',
    conversation: conversation || existing.conversation || [],
    apiMessages: apiMessages || existing.apiMessages || [],
  };
  saveTasks(tasks);
}

// Check project results dirs to update task statuses
function syncTaskStatuses() {
  const tasks = loadTasks();
  let changed = false;
  for (const [imgPath, task] of Object.entries(tasks)) {
    if (task.status === 'done' || task.status === 'error') continue;
    if (!task.projectPath || !task.taskId) continue;
    const resultFile = path.join(task.projectPath, '.xmuggle', 'results', task.taskId, 'result.json');
    try {
      const result = JSON.parse(fs.readFileSync(resultFile, 'utf8'));
      task.status = result.status === 'success' ? 'done' : 'error';
      task.prUrl = result.pr_url || '';
      changed = true;
    } catch {}
  }
  if (changed) saveTasks(tasks);
  return tasks;
}

// ── Images ──

function scanDir(dir, tasks) {
  const images = [];
  try {
    const entries = fs.readdirSync(dir);
    for (const name of entries) {
      if (name.startsWith('.')) continue;
      const ext = path.extname(name).toLowerCase();
      if (!IMAGE_EXTS.has(ext)) continue;
      const full = path.join(dir, name);
      try {
        const stat = fs.statSync(full);
        const task = tasks[full];
        const status = task ? task.status : 'new';
        const projectPath = task ? task.projectPath : '';
        const conversation = task ? (task.conversation || []) : [];
        images.push({ path: full, name, mtime: stat.mtimeMs, status, projectPath, conversation });
      } catch {}
    }
  } catch {}
  return images;
}

function getDesktopImages() {
  const tasks = syncTaskStatuses();
  const images = [...scanDir(DESKTOP, tasks), ...scanDir(INBOX_DIR, tasks)];
  images.sort((a, b) => b.mtime - a.mtime);
  return images;
}

// ── Window ──

function createWindow() {
  const win = new BrowserWindow({
    width: 1200,
    height: 800,
    title: 'xmuggle',
    backgroundColor: '#1a1a2e',
    icon: path.join(__dirname, 'assets', 'icon.png'),
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
    },
  });

  win.loadFile(path.join(__dirname, 'renderer', 'index.html'));

  // Watch Desktop
  try {
    fs.watch(DESKTOP, () => {
      try { win.webContents.send('images-updated', getDesktopImages()); } catch {}
    });
  } catch {}

  // Watch each project's .xmuggle/ dir
  for (const p of loadProjects()) {
    const xdir = path.join(p, '.xmuggle');
    try {
      fs.watch(xdir, { recursive: true }, () => {
        try { win.webContents.send('images-updated', getDesktopImages()); } catch {}
      });
    } catch {}
  }

  return win;
}

app.whenReady().then(() => {
  if (process.platform === 'darwin' && app.dock) {
    app.dock.setIcon(path.join(__dirname, 'assets', 'icon.png'));
  }
  const api = require('./api');

  ipcMain.handle('get-images', () => getDesktopImages());
  ipcMain.handle('delete-image', (_, imgPath) => {
    try { fs.unlinkSync(imgPath); } catch {}
    // Also remove from tasks
    const tasks = loadTasks();
    delete tasks[imgPath];
    saveTasks(tasks);
    return getDesktopImages();
  });

  // Projects
  ipcMain.handle('list-projects', () => listProjects());
  ipcMain.handle('add-project', async () => {
    const result = await dialog.showOpenDialog({ properties: ['openDirectory'] });
    if (result.canceled || !result.filePaths.length) return null;
    return addProject(result.filePaths[0]);
  });
  ipcMain.handle('remove-project', (_, dirPath) => {
    removeProject(dirPath);
    return listProjects();
  });

  // API key
  ipcMain.handle('has-api-key', () => api.hasApiKey());
  ipcMain.handle('set-api-key', (_, key) => { api.setApiKey(key); return true; });
  ipcMain.handle('reset-api-key', () => { api.resetApiKey(); return true; });

  // GitHub token
  ipcMain.handle('has-gh-token', () => api.hasGhToken());
  ipcMain.handle('set-gh-token', (_, token) => { api.setGhToken(token); return true; });
  ipcMain.handle('reset-gh-token', () => { api.resetGhToken(); return true; });

  // Model
  ipcMain.handle('get-model', () => api.getModel());
  ipcMain.handle('set-model', (_, modelId) => { api.setModel(modelId); return true; });
  ipcMain.handle('list-models', () => api.listModels());
  ipcMain.handle('open-external', (_, url) => shell.openExternal(url));

  // Relay
  ipcMain.handle('get-relay-host', () => api.getRelayHost());
  ipcMain.handle('set-relay-host', (_, host) => { api.setRelayHost(host); return true; });

  // Scan local network for xmuggle relay servers
  ipcMain.handle('scan-network', async () => {
    const nets = os.networkInterfaces();
    const localIPs = [];
    for (const iface of Object.values(nets)) {
      for (const cfg of iface) {
        if (cfg.family === 'IPv4' && !cfg.internal) localIPs.push(cfg.address);
      }
    }
    if (!localIPs.length) return [];

    const myIP = localIPs[0];
    const subnet = myIP.split('.').slice(0, 3).join('.');
    const found = [];

    const probe = async (ip) => {
      if (ip === myIP) return; // skip self
      const controller = new AbortController();
      const timer = setTimeout(() => controller.abort(), 500);
      try {
        const resp = await fetch(`http://${ip}:${SERVER_PORT}/status`, { signal: controller.signal });
        if (resp.ok) {
          const data = await resp.json();
          found.push({ ip, hostname: data.hostname || ip, projects: data.projects || [] });
        }
      } catch {} finally { clearTimeout(timer); }
    };

    // Scan 1-254 in parallel batches
    const ips = [];
    for (let i = 1; i <= 254; i++) ips.push(`${subnet}.${i}`);
    const batchSize = 50;
    for (let i = 0; i < ips.length; i += batchSize) {
      await Promise.all(ips.slice(i, i + batchSize).map(probe));
    }
    return found;
  });
  ipcMain.handle('send-to-relay', async (_, imagePath, project, message) => {
    const host = api.getRelayHost();
    if (!host) throw new Error('No relay host configured');
    const imageData = fs.readFileSync(imagePath).toString('base64');
    const filename = path.basename(imagePath);
    const body = JSON.stringify({ image: imageData, filename, project, message });
    const resp = await fetch(`http://${host}:${SERVER_PORT}/submit`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body,
    });
    if (!resp.ok) {
      const err = await resp.text();
      throw new Error(`Relay error ${resp.status}: ${err}`);
    }
    return await resp.json();
  });

  // Send
  ipcMain.handle('send-to-api', async (event, imagePaths, projectPath, message) => {
    const imgPath = imagePaths[0];
    const taskId = Date.now().toString(36) + Math.random().toString(36).slice(2, 6);
    const win = BrowserWindow.fromWebContents(event.sender);

    // Mark as processing
    updateTaskStatus(imgPath, projectPath, taskId, 'processing');

    const onProgress = (msg) => {
      try { win.webContents.send('task-progress', imgPath, msg); } catch {}
    };

    try {
      const result = await api.analyzeAndFix({ imagePaths, projectPath, message, onProgress });
      const finalStatus = result.status === 'success' ? 'done' : (result.status === 'no_changes' ? 'done' : 'error');
      updateTaskStatus(imgPath, projectPath, taskId, finalStatus, result.prUrl, result.conversation, result.messages);
      return result;
    } catch (err) {
      updateTaskStatus(imgPath, projectPath, taskId, 'error');
      throw err;
    }
  });

  // Follow-up message on existing conversation
  ipcMain.handle('send-followup', async (event, imgPath, message) => {
    const tasks = loadTasks();
    const task = tasks[imgPath];
    if (!task) throw new Error('No task found for this image');

    const win = BrowserWindow.fromWebContents(event.sender);
    const onProgress = (msg) => {
      try { win.webContents.send('task-progress', imgPath, msg); } catch {}
    };

    updateTaskStatus(imgPath, task.projectPath, task.taskId, 'processing', task.prUrl, task.conversation, task.apiMessages);

    try {
      const result = await api.analyzeAndFix({
        imagePaths: [imgPath],
        projectPath: task.projectPath,
        message,
        onProgress,
        priorMessages: task.apiMessages,
      });
      const finalStatus = result.status === 'success' ? 'done' : (result.status === 'no_changes' ? 'done' : 'error');
      updateTaskStatus(imgPath, task.projectPath, task.taskId, finalStatus, result.prUrl || task.prUrl, result.conversation, result.messages);
      return result;
    } catch (err) {
      updateTaskStatus(imgPath, task.projectPath, task.taskId, 'error', task.prUrl, task.conversation, task.apiMessages);
      throw err;
    }
  });

  // Get conversation for an image
  ipcMain.handle('get-conversation', (_, imgPath) => {
    const tasks = loadTasks();
    const task = tasks[imgPath];
    return task ? (task.conversation || []) : [];
  });

  const win = createWindow();

  // ── Relay server: receive images from remote xmuggle instances ──
  fs.mkdirSync(INBOX_DIR, { recursive: true });

  // Watch inbox for new images
  try {
    fs.watch(INBOX_DIR, () => {
      try { win.webContents.send('images-updated', getDesktopImages()); } catch {}
    });
  } catch {}

  const server = http.createServer((req, res) => {
    // CORS
    res.setHeader('Access-Control-Allow-Origin', '*');
    res.setHeader('Access-Control-Allow-Methods', 'POST, GET, OPTIONS');
    res.setHeader('Access-Control-Allow-Headers', 'Content-Type');
    if (req.method === 'OPTIONS') { res.writeHead(200); res.end(); return; }

    if (req.method === 'GET' && req.url === '/status') {
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ status: 'ok', hostname: os.hostname(), projects: listProjects() }));
      return;
    }

    if (req.method === 'POST' && req.url === '/submit') {
      let body = [];
      req.on('data', chunk => body.push(chunk));
      req.on('end', () => {
        try {
          const data = JSON.parse(Buffer.concat(body).toString());
          const { image, filename, project, message } = data;
          if (!image || !filename) {
            res.writeHead(400, { 'Content-Type': 'application/json' });
            res.end(JSON.stringify({ error: 'image and filename required' }));
            return;
          }

          // Save image to inbox
          const imgPath = path.join(INBOX_DIR, filename);
          fs.writeFileSync(imgPath, Buffer.from(image, 'base64'));

          // Pre-assign project and message if provided
          if (project) {
            const taskId = Date.now().toString(36) + Math.random().toString(36).slice(2, 6);
            updateTaskStatus(imgPath, project, taskId, 'new', '', [
              { role: 'user', text: message || '' }
            ]);
          }

          // Save the message as a sidecar so the UI can show it
          if (message) {
            fs.writeFileSync(imgPath + '.msg', message);
          }

          try { win.webContents.send('images-updated', getDesktopImages()); } catch {}

          res.writeHead(200, { 'Content-Type': 'application/json' });
          res.end(JSON.stringify({ status: 'received', path: imgPath }));
        } catch (e) {
          res.writeHead(400, { 'Content-Type': 'application/json' });
          res.end(JSON.stringify({ error: e.message }));
        }
      });
      return;
    }

    res.writeHead(404);
    res.end('Not found');
  });

  server.listen(SERVER_PORT, '0.0.0.0', () => {
    console.log(`xmuggle relay server listening on port ${SERVER_PORT}`);
  });
});

app.on('window-all-closed', () => app.quit());
