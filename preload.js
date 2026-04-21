const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('xmuggle', {
  getImages: () => ipcRenderer.invoke('get-images'),
  deleteImage: (imgPath) => ipcRenderer.invoke('delete-image', imgPath),
  listProjects: () => ipcRenderer.invoke('list-projects'),
  addProject: () => ipcRenderer.invoke('add-project'),
  removeProject: (path) => ipcRenderer.invoke('remove-project', path),
  hasGhToken: () => ipcRenderer.invoke('has-gh-token'),
  setGhToken: (token) => ipcRenderer.invoke('set-gh-token', token),
  resetGhToken: () => ipcRenderer.invoke('reset-gh-token'),
  saveItem: (imagePath, projectPath, message) => ipcRenderer.invoke('save-item', imagePath, projectPath, message),
  createNote: (text) => ipcRenderer.invoke('create-note', text),
  getRelayHost: () => ipcRenderer.invoke('get-relay-host'),
  setRelayHost: (host) => ipcRenderer.invoke('set-relay-host', host),
  scanNetwork: () => ipcRenderer.invoke('scan-network'),
  sendToRelay: (imagePath, project, message) => ipcRenderer.invoke('send-to-relay', imagePath, project, message),
  getSyncRepo: () => ipcRenderer.invoke('get-sync-repo'),
  setSyncRepo: (repo) => ipcRenderer.invoke('set-sync-repo', repo),
  gitSyncPush: (imagePath, project, message) => ipcRenderer.invoke('git-sync-push', imagePath, project, message),
  openExternal: (url) => ipcRenderer.invoke('open-external', url),
  onImagesUpdated: (callback) => {
    ipcRenderer.on('images-updated', (_, images) => callback(images));
  },
  onTaskProgress: (callback) => {
    ipcRenderer.on('task-progress', (_, imgPath, msg) => callback(imgPath, msg));
  },
});
