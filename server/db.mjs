import { randomUUID } from 'node:crypto';
import { mkdirSync } from 'node:fs';
import { DatabaseSync } from 'node:sqlite';
import path from 'node:path';

const DATA_DIR = process.env.DATA_DIR ?? 'server-data';
const now = () => new Date().toISOString();

mkdirSync(DATA_DIR, { recursive: true });

export const db = new DatabaseSync(path.join(DATA_DIR, 'diagrams.db'));

db.exec(`
  PRAGMA journal_mode = WAL;
  PRAGMA foreign_keys = ON;
  CREATE TABLE IF NOT EXISTS folders (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    parent_id TEXT REFERENCES folders(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
  );
  CREATE TABLE IF NOT EXISTS diagrams (
    id TEXT PRIMARY KEY,
    folder_id TEXT REFERENCES folders(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    state TEXT NOT NULL,
    favorite INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
  );
  CREATE INDEX IF NOT EXISTS idx_diagrams_folder ON diagrams(folder_id);
  CREATE INDEX IF NOT EXISTS idx_diagrams_favorite ON diagrams(favorite);
`);

export const listFolders = () => db.prepare('SELECT * FROM folders ORDER BY name').all();

export const listAllDiagrams = () => db.prepare('SELECT * FROM diagrams').all();

export const getFolder = (id) => db.prepare('SELECT * FROM folders WHERE id = ?').get(id) ?? null;

// True when ancestorId is id itself or one of id's ancestors (cycle guard for moves).
export const isSelfOrAncestor = (id, ancestorId) => {
  let current = getFolder(ancestorId);
  while (current) {
    if (current.id === id) {
      return true;
    }
    current = current.parent_id ? getFolder(current.parent_id) : null;
  }
  return false;
};

export const createFolder = ({ name, parentId = null }) => {
  const id = randomUUID();
  const ts = now();
  db.prepare(
    'INSERT INTO folders (id, name, parent_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)'
  ).run(id, name, parentId, ts, ts);
  return getFolder(id);
};

export const updateFolder = (id, { name, parentId }) => {
  const folder = getFolder(id);
  if (!folder) {
    return null;
  }
  db.prepare('UPDATE folders SET name = ?, parent_id = ?, updated_at = ? WHERE id = ?').run(
    name ?? folder.name,
    parentId === undefined ? folder.parent_id : parentId,
    now(),
    id
  );
  return getFolder(id);
};

// Child folders cascade via FK; diagrams in any deleted folder fall back to
// uncategorized via ON DELETE SET NULL.
export const deleteFolder = (id) =>
  db.prepare('DELETE FROM folders WHERE id = ?').run(id).changes > 0;

export const listDiagrams = ({ favorite, folderId, search } = {}) => {
  const where = [];
  const params = [];
  if (folderId === 'root') {
    where.push('folder_id IS NULL');
  } else if (folderId) {
    where.push('folder_id = ?');
    params.push(folderId);
  }
  if (search) {
    where.push('name LIKE ?');
    params.push(`%${search}%`);
  }
  if (favorite) {
    where.push('favorite = 1');
  }
  const sql = `SELECT * FROM diagrams ${
    where.length > 0 ? `WHERE ${where.join(' AND ')}` : ''
  } ORDER BY updated_at DESC`;
  return db.prepare(sql).all(...params);
};

export const getDiagram = (id) => db.prepare('SELECT * FROM diagrams WHERE id = ?').get(id) ?? null;

export const createDiagram = ({ folderId = null, name, state }) => {
  const id = randomUUID();
  const ts = now();
  db.prepare(
    'INSERT INTO diagrams (id, folder_id, name, state, favorite, created_at, updated_at) VALUES (?, ?, ?, ?, 0, ?, ?)'
  ).run(id, folderId, name, state, ts, ts);
  return getDiagram(id);
};

export const updateDiagram = (id, { favorite, folderId, name, state }) => {
  const diagram = getDiagram(id);
  if (!diagram) {
    return null;
  }
  db.prepare(
    'UPDATE diagrams SET name = ?, folder_id = ?, state = ?, favorite = ?, updated_at = ? WHERE id = ?'
  ).run(
    name ?? diagram.name,
    folderId === undefined ? diagram.folder_id : folderId,
    state ?? diagram.state,
    favorite === undefined ? diagram.favorite : favorite,
    now(),
    id
  );
  return getDiagram(id);
};

export const deleteDiagram = (id) =>
  db.prepare('DELETE FROM diagrams WHERE id = ?').run(id).changes > 0;

export const importAll = (payload) => {
  let imported = 0;
  let skipped = 0;
  const insertFolder = db.prepare(
    'INSERT OR IGNORE INTO folders (id, name, parent_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)'
  );
  for (const folder of payload.folders ?? []) {
    const result = insertFolder.run(
      folder.id,
      folder.name,
      folder.parentId ?? null,
      folder.createdAt ?? now(),
      folder.updatedAt ?? now()
    );
    if (result.changes > 0) {
      imported++;
    } else {
      skipped++;
    }
  }
  const insertDiagram = db.prepare(
    'INSERT OR IGNORE INTO diagrams (id, folder_id, name, state, favorite, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)'
  );
  for (const diagram of payload.diagrams ?? []) {
    const result = insertDiagram.run(
      diagram.id,
      diagram.folderId ?? null,
      diagram.name,
      diagram.state,
      diagram.favorite ? 1 : 0,
      diagram.createdAt ?? now(),
      diagram.updatedAt ?? now()
    );
    if (result.changes > 0) {
      imported++;
    } else {
      skipped++;
    }
  }
  return { imported, skipped };
};
