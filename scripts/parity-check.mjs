// Differential parity harness: replays an identical request corpus against the
// Node storage server (server/index.mjs) and the Go port (server-go), each on
// a fresh DATA_DIR, and diffs every response after normalizing UUIDs and ISO
// timestamps and canonicalizing JSON key order.
//
// Ports: node's server defaults to 18790, but on this machine 18790 is held by
// the live systemd `mermaid-storage.service`, so the harness runs its scratch
// servers on 28790/28791 instead.
import { execFileSync, spawn } from 'node:child_process';
import { mkdirSync, rmSync } from 'node:fs';

const NODE_PORT = 28790;
const GO_PORT = 28791;
const ROOT = new URL('..', import.meta.url).pathname;
const UUID_RE = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/g;
const ISO_RE = /\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z/g;
const normalize = (text) => text.replaceAll(UUID_RE, '<uuid>').replaceAll(ISO_RE, '<ts>');

const startServer = (name, command, args, env, port) =>
  new Promise((resolve, reject) => {
    const child = spawn(command, args, { cwd: ROOT, env: { ...process.env, ...env } });
    const listener = (buffer) => {
      if (String(buffer).includes(`:${port}`)) {
        child.stdout.off('data', listener);
        child.stderr.off('data', listener);
        resolve(child);
      }
    };
    child.stdout.on('data', listener);
    child.stderr.on('data', listener);
    child.on('exit', (code) => reject(new Error(`${name} exited early: ${code}`)));
  });

// An unresolved {{ref}} must never silently expand to '' — that 404s on BOTH
// servers and hides the branch behind a passing diff. Fail loudly instead.
const expand = (text, refs) =>
  text.replaceAll(/\{\{([^}]+)\}\}/g, (_, expr) => {
    const [key, index, field] = expr.split('.');
    const value = refs[key]?.[Number(index)]?.[field];
    if (value === undefined || value === null) {
      throw new Error(
        `unresolved template {{${expr}}} (refs: ${JSON.stringify(Object.keys(refs))})`
      );
    }
    return String(value);
  });

// Structural comparison: Node and Go marshal JSON with different key orders
// (object insertion order vs struct order) and different positions key order
// (insertion vs sorted), so raw-text diffing would false-positive. Parse both
// bodies, canonicalize (recursively sort object keys), THEN compare strings.
const canonicalize = (value) => {
  if (Array.isArray(value)) {
    return value.map(canonicalize);
  }
  if (value !== null && typeof value === 'object') {
    return Object.fromEntries(
      Object.keys(value)
        .sort()
        .map((key) => [key, canonicalize(value[key])])
    );
  }
  return value;
};

// `body` is the normalized comparable; `raw` keeps the real UUIDs/timestamps
// so refs captured from list responses expand to usable ids.
const request = async (port, method, path, body, refs) => {
  const response = await fetch(`http://127.0.0.1:${port}${expand(path, refs)}`, {
    body: body === undefined ? undefined : expand(JSON.stringify(body), refs),
    headers: { 'Content-Type': 'application/json' },
    method
  });
  const text = await response.text();
  let comparable;
  try {
    comparable = normalize(JSON.stringify(canonicalize(JSON.parse(text))));
  } catch {
    comparable = normalize(text); // non-JSON bodies: raw text, normalized
  }
  return { body: comparable, raw: text, status: response.status };
};

// Corpus: every route + every validation branch. {{folders.0.id}} etc. come
// from each server's own previous list responses.
const corpus = [
  ['POST', '/api/folders', { name: '  Work  ' }],
  ['POST', '/api/folders', { name: 'Sub', parentId: '{{folders.0.id}}' }],
  ['GET', '/api/folders'],
  ['POST', '/api/folders', { name: '' }],
  ['POST', '/api/folders', { name: 'X', parentId: 'nope' }],
  ['PATCH', '/api/folders/{{folders.0.id}}', { name: 'Renamed' }],
  ['PATCH', '/api/folders/{{folders.1.id}}', { parentId: '{{folders.1.id}}' }],
  ['PATCH', '/api/folders/{{folders.1.id}}', { parentId: null }],
  ['PATCH', '/api/folders/missing', { name: 'Y' }],
  [
    'POST',
    '/api/diagrams',
    { name: ' alpha', state: 'code-1', positions: { A: { dx: 1, dy: 2 } } }
  ],
  ['POST', '/api/diagrams', { name: 'beta', state: 'code-2', folderId: '{{folders.0.id}}' }],
  ['POST', '/api/diagrams', { name: 'gamma', state: 'code-3' }],
  ['GET', '/api/diagrams'],
  ['POST', '/api/diagrams', { state: 'x' }],
  ['POST', '/api/diagrams', { name: 'x' }],
  ['POST', '/api/diagrams', { name: 'x', state: 'y', positions: [1] }],
  ['GET', '/api/diagrams?folderId=root'],
  ['GET', '/api/diagrams?folderId={{folders.0.id}}'],
  ['GET', '/api/diagrams?favorite=1'],
  ['GET', '/api/diagrams?search=alp'],
  ['GET', '/api/diagrams/{{diagrams.0.id}}'],
  ['GET', '/api/diagrams/missing'],
  ['PATCH', '/api/diagrams/{{diagrams.0.id}}', { favorite: true, positions: null }],
  ['PATCH', '/api/diagrams/{{diagrams.0.id}}', { name: '' }],
  ['PATCH', '/api/diagrams/{{diagrams.0.id}}', { state: '' }],
  ['PATCH', '/api/diagrams/missing', { name: 'z' }],
  ['DELETE', '/api/diagrams/{{diagrams.2.id}}'],
  ['DELETE', '/api/diagrams/missing'],
  ['DELETE', '/api/folders/{{folders.0.id}}'],
  ['GET', '/api/export'],
  ['POST', '/api/import', { folders: {}, diagrams: [] }],
  [
    'POST',
    '/api/import',
    { folders: [{ id: 'if1', name: 'Imp' }], diagrams: [{ id: 'id1', name: 'D', state: 's' }] }
  ],
  ['POST', '/api/import', { folders: [{ id: 'if1', name: 'dup' }], diagrams: [] }],
  ['GET', '/api/nope'],
  ['PUT', '/api/folders', {}]
];

const run = async (port) => {
  const refs = { diagrams: [], folders: [] };
  const out = [];
  for (const [method, path, body] of corpus) {
    const response = await request(port, method, path, body, refs);
    out.push(`${method} ${path} -> ${response.status} ${response.body}`);
    let parsed;
    try {
      parsed = JSON.parse(response.raw);
    } catch {
      continue;
    }
    if (Array.isArray(parsed) && method === 'GET' && path === '/api/folders') {
      refs.folders = parsed;
    }
    if (Array.isArray(parsed) && method === 'GET' && path === '/api/diagrams') {
      refs.diagrams = parsed;
    }
    // Corpus[1] templates folders.0.id BEFORE the first GET list (the brief's
    // "lists precede every templated use" miscounts), so single-object
    // create/update responses must seed refs too. Appends never disturb the
    // indices the corpus templates (lists replace, objects append).
    if (!Array.isArray(parsed) && typeof parsed?.id === 'string') {
      if ('state' in parsed) {
        refs.diagrams.push(parsed);
      } else {
        refs.folders.push(parsed);
      }
    }
  }
  return out;
};

const scratchDirs = ['server-data/parity-node', 'server-data/parity-go'];
const cleanup = (children) => {
  for (const child of children) {
    child.kill();
  }
  for (const dir of scratchDirs) {
    rmSync(`${ROOT}${dir}`, { recursive: true, force: true });
  }
};

const main = async () => {
  execFileSync('go', ['build', '-o', 'bin/mermaid-storage', '.'], { cwd: `${ROOT}server-go` });
  for (const dir of scratchDirs) {
    rmSync(`${ROOT}${dir}`, { recursive: true, force: true });
    mkdirSync(`${ROOT}${dir}`, { recursive: true });
  }
  const children = [];
  try {
    children.push(
      await startServer(
        'node',
        'node',
        ['server/index.mjs'],
        { DATA_DIR: 'server-data/parity-node', PORT: String(NODE_PORT) },
        NODE_PORT
      )
    );
    children.push(
      await startServer(
        'go',
        'server-go/bin/mermaid-storage',
        [],
        { DATA_DIR: 'server-data/parity-go', PORT: String(GO_PORT) },
        GO_PORT
      )
    );
    const [nodeOut, goOut] = await Promise.all([run(NODE_PORT), run(GO_PORT)]);
    if (process.env.PARITY_DEBUG) {
      for (const line of nodeOut) {
        console.log(`  node| ${line}`);
      }
    }
    let diffs = 0;
    for (let i = 0; i < nodeOut.length; i++) {
      if (nodeOut[i] !== goOut[i]) {
        diffs++;
        console.log(
          `DIFF at corpus[${i}] ${corpus[i][0]} ${corpus[i][1]}:\n  node: ${nodeOut[i]}\n  go:   ${goOut[i]}`
        );
      }
    }
    console.log(
      diffs === 0 ? `parity OK: ${nodeOut.length} responses identical` : `${diffs} diffs`
    );
    process.exitCode = diffs === 0 ? 0 : 1;
  } finally {
    cleanup(children);
  }
};

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
