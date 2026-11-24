import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { extname } from 'node:path';

const root = new URL('../out/docs-site/', import.meta.url);
const mime = { '.html': 'text/html', '.css': 'text/css', '.js': 'text/javascript', '.json': 'application/json' };
const port = Number(process.env.DOCS_PORT ?? 4173);
createServer(async (request, response) => {
  try {
    const path = new URL(request.url, 'http://localhost').pathname;
    if (!/^\/(?:[a-z]+\/)?[a-z.-]*$/.test(path) || path.split('/').some(part => part === '..')) {
      response.writeHead(404).end();
      return;
    }
    const file = path.endsWith('/') ? `${path}index.html` : path;
    const content = await readFile(new URL(file.slice(1), root));
    response.writeHead(200, { 'Content-Type': `${mime[extname(file)] ?? 'text/plain'}; charset=utf-8` }).end(content);
  } catch {
    response.writeHead(404).end();
  }
}).listen(port, '127.0.0.1', () => console.log(`http://127.0.0.1:${port}/`));
