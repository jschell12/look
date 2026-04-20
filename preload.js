const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('xmuggle', {
  getImages: () => ipcRenderer.invoke('get-images'),
  deleteImage: (imgPath) => ipcRenderer.invoke('delete-image', imgPath),
  listProjects: () => ipcRenderer.invoke('list-projects'),
  addProject: () => ipcRenderer.invoke('add-project'),
  removeProject: (path) => ipcRenderer.invoke('remove-project', path),
  hasApiKey: () => ipcRenderer.invoke('has-api-key'),
  setApiKey: (key) => ipcRenderer.invoke('set-api-key', key),
  resetApiKey: () => ipcRenderer.invoke('reset-api-key'),
  hasGhToken: () => ipcRenderer.invoke('has-gh-token'),
  setGhToken: (token) => ipcRenderer.invoke('set-gh-token', token),
  resetGhToken: () => ipcRenderer.invoke('reset-gh-token'),
  getModel: () => ipcRenderer.invoke('get-model'),
  setModel: (modelId) => ipcRenderer.invoke('set-model', modelId),
  listModels: () => ipcRenderer.invoke('list-models'),
  sendToApi: (imagePaths, projectPath, message) => ipcRenderer.invoke('send-to-api', imagePaths, projectPath, message),
  sendFollowup: (imgPath, message) => ipcRenderer.invoke('send-followup', imgPath, message),
  getConversation: (imgPath) => ipcRenderer.invoke('get-conversation', imgPath),
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
