const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('xmuggle', {
  getImages: () => ipcRenderer.invoke('get-images'),
  onImagesUpdated: (callback) => {
    ipcRenderer.on('images-updated', (_, images) => callback(images));
  },
});
