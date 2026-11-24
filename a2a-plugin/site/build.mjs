import { mkdir, readFile, writeFile, copyFile } from 'node:fs/promises';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import MarkdownIt from 'markdown-it';

const site = fileURLToPath(new URL('.', import.meta.url));
const docs = join(site, '../docs');
const output = join(site, '../out/docs-site');
const md = new MarkdownIt({ html: false });
const escape = md.utils.escapeHtml;
const pages = [
  ['index', 'README.md', 'README.md', 'Inicio', 'Overview'],
  ['community', 'community.md', 'community.md', 'Community', 'Community'],
  ['enterprise', 'enterprise.md', 'enterprise.md', 'Enterprise', 'Enterprise'],
  ['agents', 'agentes.md', 'agents.md', 'Catálogo de agentes', 'Agent catalog'],
  ['configuration', 'configuracion.md', 'configuration.md', 'Referencia de configuración', 'Configuration reference'],
  ['security', 'seguridad.md', 'security.md', 'Seguridad', 'Security'],
  ['compatibility', 'compatibilidad.md', 'compatibility.md', 'Compatibilidad', 'Compatibility'],
  ['development', 'desarrollo.md', 'development.md', 'Desarrollo', 'Development'],
  ['validation', 'validacion-supuestos.md', 'validation.md', 'Validación técnica', 'Technical validation'],
];
const groups = [
  [0, 3, 'Primeros pasos', 'Get started'],
  [3, 6, 'Configuración', 'Configuration'],
  [6, 9, 'Referencia', 'Reference'],
];
const copy = {
  es: { label: 'Documentación', nav: 'Documentación', skip: 'Ir al contenido', lang: 'Idioma',
    search: 'Buscar en la documentación', placeholder: 'Buscar en la documentación', none: 'Sin resultados',
    toc: 'En esta página', previous: 'Anterior', next: 'Siguiente', menu: 'Menú' },
  en: { label: 'Documentation', nav: 'Documentation', skip: 'Skip to content', lang: 'Language',
    search: 'Search documentation', placeholder: 'Search documentation', none: 'No results',
    toc: 'On this page', previous: 'Previous', next: 'Next', menu: 'Menu' },
};

// Names that must stay out of the docs, such as customers, cannot be listed
// here because this file is public. Keep them in private-markers.txt, which git
// ignores: one case-insensitive pattern per line, # for comments.
const localMarkers = await readFile(join(site, 'private-markers.txt'), 'utf8')
  .then(text => text.split(/\r?\n/).map(line => line.trim()).filter(line => line && !line.startsWith('#')))
  .catch(error => { if (error.code === 'ENOENT') return []; throw error; });

const privateMarkers = [
  ...localMarkers.map(pattern => new RegExp(pattern, 'i')),
  /-----BEGIN (?:[A-Z ]+ )?PRIVATE KEY-----/,
  /\b(?:AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9]{25,}|sk-(?:proj-|ant-)?[A-Za-z0-9_-]{24,})\b/,
  /\b[A-Z]:[/\\]Users[/\\][^/\\\s]+|\/(?:home|Users)\/[\w.-]+/i,
  /\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b/i,
];

function reviewSource(source, file) {
  if (privateMarkers.some(pattern => pattern.test(source))) {
    throw new Error(`Review potentially private data in ${file}`);
  }
  for (const match of source.matchAll(/https?:\/\/([^\s"'<>\x60(){}]+)/gi)) {
    const url = new URL(`https://${match[1]}`);
    const host = url.hostname.toLowerCase();
    const allowed = /^(?:[\w-]+\.)*example\.(?:com|org|net)$/.test(host) ||
      ['localhost', '127.0.0.1', 'billing', 'research', 'mock-idp', 'www.krakend.io', 'a2a-protocol.org'].includes(host);
    if (!allowed || url.username || url.password) {
      throw new Error(`Review a URL in ${file}. Use a reserved example hostname for deployment examples.`);
    }
  }
}

function slug(text) {
  return text.toLowerCase().replace(/[^\p{Letter}\p{Number}\s-]/gu, '').trim().replace(/\s+/g, '-');
}

function markdown(source, peers, file) {
  const env = {};
  const tokens = md.parse(source, env);
  const headings = [];
  const used = new Set();
  for (const [i, token] of tokens.entries()) {
    if (token.type === 'heading_open') {
      const heading = tokens[i + 1].children.map(child => child.content).join('');
      const base = slug(heading) || 'section';
      let id = base;
      for (let suffix = 2; used.has(id); suffix++) id = `${base}-${suffix}`;
      used.add(id);
      token.attrSet('id', id);
      headings.push({ id, heading, level: Number(token.tag.slice(1)) });
    }
    for (const child of token.children ?? []) {
      if (child.type !== 'link_open') continue;
      const href = child.attrGet('href');
      const [path, anchor] = href.split('#');
      if (!path.endsWith('.md')) continue;
      const target = peers.get(path.replace(/^\.\//, ''));
      if (!target) throw new Error(`Unknown documentation link in ${file}: ${href}`);
      child.attrSet('href', `${target}${anchor ? `#${anchor}` : ''}`);
    }
  }
  return { html: md.renderer.render(tokens, md.options, env), headings, ids: used };
}

function layout(locale, index, article) {
  const active = pages[index][0];
  const languageIndex = locale === 'es' ? 3 : 4;
  const lang = copy[locale];
  const localeHref = other => `../${other}/${active}.html`;
  const nav = groups.map(([start, end, ...titles]) => `
    <section class="nav-group"><h2>${titles[languageIndex - 3]}</h2>
      ${pages.slice(start, end).map(([id, , , es, en]) => `<a href="${id}.html"${id === active ? ' aria-current="page"' : ''}>${locale === 'es' ? es : en}</a>`).join('')}
    </section>`).join('');
  const contents = article.headings.filter(h => h.level === 2 || h.level === 3);
  const footLink = (page, label) => page ? `<a href="${page[0]}.html"><small>${label}</small>${page[languageIndex]}</a>` : '<span></span>';
  const title = article.headings[0].heading;
  return `<!doctype html>
<html lang="${locale}">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light"><title>${escape(title)} · A2A Gateway Docs</title>
<link rel="stylesheet" href="../style.css"><script type="module" src="../search.js"></script></head>
<body>
<a class="skip" href="#content">${lang.skip}</a>
<header class="top"><a class="brand" href="index.html"><span class="mark" aria-hidden="true">A2A</span><span>A2A Gateway<span class="brand-label">${lang.label}</span></span></a>
<details class="search"><summary>${lang.search}</summary><div class="search-panel">
<label class="sr-only" for="search">${lang.search}</label><input type="search" id="search" placeholder="${lang.placeholder}" autocomplete="off">
<ul id="results" aria-live="polite" data-empty="${lang.none}"></ul></div></details>
<nav class="language" aria-label="${lang.lang}"><a href="${localeHref('es')}" lang="es"${locale === 'es' ? ' aria-current="page"' : ''}>Español</a>
<a href="${localeHref('en')}" lang="en"${locale === 'en' ? ' aria-current="page"' : ''}>English</a></nav></header>
<div class="layout">
<div class="navigation"><details open><summary>${lang.menu}</summary><nav aria-label="${lang.nav}">${nav}</nav></details></div>
<main id="content" tabindex="-1"><article>${article.html}</article>
<nav class="pagination" aria-label="${lang.nav}">${footLink(pages[index - 1], lang.previous)}${footLink(pages[index + 1], lang.next)}</nav></main>
${contents.length ? `<aside><nav aria-label="${lang.toc}"><p>${lang.toc}</p>${contents.map(({id, heading, level}) => `<a class="level-${level}" href="#${id}">${escape(heading)}</a>`).join('')}</nav></aside>` : ''}
</div></body></html>\n`;
}

for (const locale of ['es', 'en']) {
  const column = locale === 'es' ? 1 : 2;
  const target = join(output, locale);
  const peers = new Map(pages.flatMap(([id, es, en]) => locale === 'es'
    ? [[es, `${id}.html`], [`en/${en}`, `../en/${id}.html`]]
    : [[en, `${id}.html`], [`../${es}`, `../es/${id}.html`]]));
  await mkdir(target, { recursive: true });
  const search = [];
  for (const [index, page] of pages.entries()) {
    const file = page[column];
    const source = await readFile(join(docs, locale === 'es' ? '' : 'en', file), 'utf8');
    reviewSource(source, file);
    const article = markdown(source, peers, file);
    await writeFile(join(target, `${page[0]}.html`), layout(locale, index, article));
    search.push([article.headings[0].heading, `${page[0]}.html`, source]);
    for (const { id, heading, level } of article.headings) {
      if (level === 2 || level === 3) search.push([heading, `${page[0]}.html#${id}`, article.headings[0].heading]);
    }
  }
  await writeFile(join(target, 'search.json'), JSON.stringify(search));
}
for (const asset of ['style.css', 'search.js']) await copyFile(join(site, asset), join(output, asset));
await writeFile(join(output, 'index.html'), '<!doctype html><html lang="es"><meta charset="utf-8"><meta http-equiv="refresh" content="0; url=es/index.html"><title>A2A Gateway Docs</title><a href="es/index.html">Documentación</a> · <a href="en/index.html" lang="en">Documentation</a></html>\n');
