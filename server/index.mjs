import { createServer } from 'node:http';
import {
  createDiagram,
  createFolder,
  deleteDiagram,
  deleteFolder,
  getDiagram,
  getFolder,
  importAll,
  isSelfOrAncestor,
  listAllDiagrams,
  listDiagrams,
  listFolders,
  updateDiagram,
  updateFolder
} from './db.mjs';

const HOST = process.env.HOST ?? '127.0.0.1';
const PORT = Number(process.env.PORT ?? 18790);

const diagramJson = (row) =>
  row && {
    createdAt: row.created_at,
    favorite: Boolean(row.favorite),
    folderId: row.folder_id,
    id: row.id,
    name: row.name,
    state: row.state,
    updatedAt: row.updated_at
  };

const folderJson = (row) =>
  row && {
    createdAt: row.created_at,
    id: row.id,
    name: row.name,
    parentId: row.parent_id,
    updatedAt: row.updated_at
  };

const json = (res, status, body) => {
  const data = JSON.stringify(body);
  res.writeHead(status, {
    'Content-Length': Buffer.byteLength(data),
    'Content-Type': 'application/json; charset=utf-8'
  });
  res.end(data);
};

const bad = (res, status, error) => json(res, status, { error });

const readBody = (req) =>
  new Promise((resolve, reject) => {
    let raw = '';
    req.on('data', (chunk) => {
      raw += chunk;
      if (raw.length > 5_000_000) {
        reject(new Error('Payload too large'));
        req.destroy();
      }
    });
    req.on('end', () => {
      if (!raw) {
        resolve({});
        return;
      }
      try {
        resolve(JSON.parse(raw));
      } catch {
        reject(new Error('Invalid JSON body'));
      }
    });
    req.on('error', reject);
  });

const isName = (value) => typeof value === 'string' && value.trim().length > 0;
const isState = (value) => typeof value === 'string' && value.trim().length > 0;

const routes = [
  {
    handler: (req, res) => json(res, 200, listFolders().map(folderJson)),
    method: 'GET',
    pattern: /^\/api\/folders$/
  },
  {
    handler: async (req, res) => {
      const body = await readBody(req);
      if (!isName(body.name)) {
        return bad(res, 400, 'name is required');
      }
      if (body.parentId != null && !getFolder(body.parentId)) {
        return bad(res, 404, 'parent folder not found');
      }
      return json(
        res,
        201,
        folderJson(createFolder({ name: body.name.trim(), parentId: body.parentId ?? null }))
      );
    },
    method: 'POST',
    pattern: /^\/api\/folders$/
  },
  {
    handler: async (req, res, url, match) => {
      const id = match[1];
      if (!getFolder(id)) {
        return bad(res, 404, 'folder not found');
      }
      const body = await readBody(req);
      if (body.name != null && !isName(body.name)) {
        return bad(res, 400, 'name must be a non-empty string');
      }
      if (body.parentId != null) {
        if (body.parentId === id || isSelfOrAncestor(id, body.parentId)) {
          return bad(res, 400, 'cannot move a folder into itself or its descendant');
        }
        if (!getFolder(body.parentId)) {
          return bad(res, 404, 'parent folder not found');
        }
      }
      return json(
        res,
        200,
        folderJson(
          updateFolder(id, {
            name: body.name != null ? body.name.trim() : undefined,
            parentId: body.parentId
          })
        )
      );
    },
    method: 'PATCH',
    pattern: /^\/api\/folders\/([^/]+)$/
  },
  {
    handler: (req, res, url, match) =>
      deleteFolder(match[1]) ? json(res, 200, { ok: true }) : bad(res, 404, 'folder not found'),
    method: 'DELETE',
    pattern: /^\/api\/folders\/([^/]+)$/
  },
  {
    handler: (req, res, url) => {
      const query = url.searchParams;
      const diagrams = listDiagrams({
        favorite: query.get('favorite') === '1',
        folderId: query.get('folderId') ?? undefined,
        search: query.get('search') ?? undefined
      });
      return json(res, 200, diagrams.map(diagramJson));
    },
    method: 'GET',
    pattern: /^\/api\/diagrams$/
  },
  {
    handler: async (req, res) => {
      const body = await readBody(req);
      if (!isName(body.name)) {
        return bad(res, 400, 'name is required');
      }
      if (!isState(body.state)) {
        return bad(res, 400, 'state is required');
      }
      if (body.folderId != null && !getFolder(body.folderId)) {
        return bad(res, 404, 'folder not found');
      }
      return json(
        res,
        201,
        diagramJson(
          createDiagram({
            folderId: body.folderId ?? null,
            name: body.name.trim(),
            state: body.state
          })
        )
      );
    },
    method: 'POST',
    pattern: /^\/api\/diagrams$/
  },
  {
    handler: (req, res, url, match) => {
      const diagram = getDiagram(match[1]);
      return diagram ? json(res, 200, diagramJson(diagram)) : bad(res, 404, 'diagram not found');
    },
    method: 'GET',
    pattern: /^\/api\/diagrams\/([^/]+)$/
  },
  {
    handler: async (req, res, url, match) => {
      const id = match[1];
      if (!getDiagram(id)) {
        return bad(res, 404, 'diagram not found');
      }
      const body = await readBody(req);
      if (body.name != null && !isName(body.name)) {
        return bad(res, 400, 'name must be a non-empty string');
      }
      if (body.state != null && !isState(body.state)) {
        return bad(res, 400, 'state must be a non-empty string');
      }
      if (body.folderId != null && !getFolder(body.folderId)) {
        return bad(res, 404, 'folder not found');
      }
      return json(
        res,
        200,
        diagramJson(
          updateDiagram(id, {
            favorite: typeof body.favorite === 'boolean' ? Number(body.favorite) : undefined,
            folderId: body.folderId,
            name: body.name != null ? body.name.trim() : undefined,
            state: body.state
          })
        )
      );
    },
    method: 'PATCH',
    pattern: /^\/api\/diagrams\/([^/]+)$/
  },
  {
    handler: (req, res, url, match) =>
      deleteDiagram(match[1]) ? json(res, 200, { ok: true }) : bad(res, 404, 'diagram not found'),
    method: 'DELETE',
    pattern: /^\/api\/diagrams\/([^/]+)$/
  },
  {
    handler: (req, res) =>
      json(res, 200, {
        diagrams: listAllDiagrams().map(diagramJson),
        exportedAt: new Date().toISOString(),
        folders: listFolders().map(folderJson)
      }),
    method: 'GET',
    pattern: /^\/api\/export$/
  },
  {
    handler: async (req, res) => {
      const body = await readBody(req);
      if (!Array.isArray(body.folders) || !Array.isArray(body.diagrams)) {
        return bad(res, 400, 'folders and diagrams arrays are required');
      }
      return json(res, 200, importAll(body));
    },
    method: 'POST',
    pattern: /^\/api\/import$/
  }
];

const server = createServer(async (req, res) => {
  try {
    const url = new URL(req.url ?? '/', `http://${req.headers.host ?? 'localhost'}`);
    for (const route of routes) {
      if (req.method !== route.method) {
        continue;
      }
      const match = url.pathname.match(route.pattern);
      if (!match) {
        continue;
      }
      await route.handler(req, res, url, match);
      return;
    }
    bad(res, 404, 'Not found');
  } catch (error) {
    bad(res, 400, error instanceof Error ? error.message : 'Bad request');
  }
});

server.listen(PORT, HOST, () => {
  console.log(`storage API listening on http://${HOST}:${PORT}`);
});
