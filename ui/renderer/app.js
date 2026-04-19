const grid = document.getElementById('grid');
const count = document.getElementById('count');

const BADGE_LABELS = {
  new: 'New',
  pending: 'Pending',
  queued: 'Queued',
  processing: 'Processing',
  done: 'Done',
  error: 'Error',
};

function render(images) {
  grid.innerHTML = '';

  const total = images.length;
  const pending = images.filter(i => i.status === 'new' || i.status === 'pending').length;
  const processing = images.filter(i => i.status === 'processing' || i.status === 'queued').length;
  const done = images.filter(i => i.status === 'done').length;
  count.textContent = `${total} images \u2022 ${pending} pending \u2022 ${processing} in progress \u2022 ${done} done`;

  for (const img of images) {
    const card = document.createElement('div');
    card.className = 'card';

    const imgEl = document.createElement('img');
    imgEl.src = `file://${img.path}`;
    imgEl.loading = 'lazy';
    card.appendChild(imgEl);

    const badge = document.createElement('span');
    badge.className = `badge ${img.status}`;
    badge.textContent = BADGE_LABELS[img.status] || img.status;
    card.appendChild(badge);

    const name = document.createElement('div');
    name.className = 'name';
    name.textContent = img.name;
    name.title = img.name;
    card.appendChild(name);

    grid.appendChild(card);
  }
}

async function refresh() {
  const images = await window.xmuggle.getImages();
  render(images);
}

window.xmuggle.onImagesUpdated((images) => render(images));
refresh();
