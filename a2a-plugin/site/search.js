const search = document.querySelector('.search');
const input = search.querySelector('input');
const results = search.querySelector('ul');
const normalize = text => text.normalize('NFD').replace(/\p{Diacritic}/gu, '').toLowerCase();
let index;

search.addEventListener('toggle', () => {
  if (search.open) input.focus();
});
search.addEventListener('keydown', event => {
  if (event.key === 'Escape') {
    search.open = false;
    search.querySelector('summary').focus();
  }
});

input.addEventListener('input', async () => {
  const query = input.value.trim();
  results.replaceChildren();
  if (!query) return;
  index ??= fetch('search.json').then(response => response.json());
  const entries = await index.catch(() => {
    index = undefined;
    return [];
  });
  if (input.value.trim() !== query) return;
  const words = normalize(query).split(/\s+/);
  const matches = entries.map(([title, url, body]) => {
    const heading = normalize(title);
    const text = normalize(body);
    if (!words.every(word => heading.includes(word) || text.includes(word))) return null;
    return { title, url, score: words.filter(word => heading.includes(word)).length };
  }).filter(Boolean).sort((a, b) => b.score - a.score).slice(0, 8);
  for (const { title, url } of matches) {
    const li = document.createElement('li');
    const link = document.createElement('a');
    link.href = url;
    link.textContent = title;
    li.append(link);
    results.append(li);
  }
  if (!matches.length) {
    const empty = document.createElement('li');
    empty.textContent = results.dataset.empty;
    results.append(empty);
  }
});

const menu = document.querySelector('.navigation details');
const compact = matchMedia('(max-width: 760px)');
function setMenu() {
  menu.open = !compact.matches;
}
setMenu();
compact.addEventListener('change', setMenu);
