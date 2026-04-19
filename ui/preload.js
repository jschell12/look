const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('xmuggle', {
  getImages: () => ipcRenderer.invoke('get-images'),
  deleteImage: (imgPath) => ipcRenderer.invoke('delete-image', imgPath),
  onImagesUpdated: (callback) => {
    ipcRenderer.on('images-updated', (_, images) => callback(images));
  },
});
