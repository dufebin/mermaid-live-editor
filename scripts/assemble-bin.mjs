import { cpSync, existsSync, mkdirSync, renameSync, rmSync } from 'node:fs';

const ROOT = new URL('..', import.meta.url).pathname;
const bin = `${ROOT}bin`;

mkdirSync(bin, { recursive: true });
rmSync(`${bin}/static.prev`, { recursive: true, force: true });
if (existsSync(`${bin}/static`)) {
  renameSync(`${bin}/static`, `${bin}/static.prev`);
}
cpSync(`${ROOT}docs`, `${bin}/static`, { recursive: true });
console.log('bin/ assembled: mermaid-storage + static/ (previous generation at static.prev)');
