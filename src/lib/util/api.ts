export interface Folder {
  createdAt: string;
  id: string;
  name: string;
  parentId: string | null;
  updatedAt: string;
}

export interface User {
  createdAt: string;
  email: string;
  id: string;
}

export interface NodeOffset {
  dx: number;
  dy: number;
}

export interface Diagram {
  createdAt: string;
  favorite: boolean;
  folderId: string | null;
  id: string;
  name: string;
  positions: Record<string, NodeOffset>;
  state: string;
  updatedAt: string;
}

export interface ExportData {
  diagrams: Diagram[];
  exportedAt: string;
  folders: Folder[];
}

const request = async <T>(path: string, init?: RequestInit): Promise<T> => {
  const response = await fetch(`/api${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...init
  });
  if (!response.ok) {
    if (response.status === 401) {
      onUnauthorized?.();
    }
    const body = (await response.json().catch(() => undefined)) as { error?: string } | undefined;
    throw new Error(body?.error ?? `API request failed (${response.status})`);
  }
  return (await response.json()) as T;
};

let onUnauthorized: (() => void) | undefined;
export const setUnauthorizedHandler = (handler: () => void): void => {
  onUnauthorized = handler;
};

const authRequest = async <T>(path: string, init?: RequestInit): Promise<T> => {
  const response = await fetch(`/api/auth${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...init
  });
  if (!response.ok) {
    const body = (await response.json().catch(() => undefined)) as { error?: string } | undefined;
    throw new Error(body?.error ?? `API request failed (${response.status})`);
  }
  return (await response.json()) as T;
};

export const apiRegister = (email: string, password: string): Promise<User> =>
  authRequest<User>('/register', { body: JSON.stringify({ email, password }), method: 'POST' });

export const apiLogin = (email: string, password: string): Promise<User> =>
  authRequest<User>('/login', { body: JSON.stringify({ email, password }), method: 'POST' });

export const apiLogout = (): Promise<{ ok: boolean }> =>
  authRequest<{ ok: boolean }>('/logout', { method: 'POST' });

export const listFolders = (): Promise<Folder[]> => request<Folder[]>('/folders');

export const createFolder = (name: string, parentId: string | null = null): Promise<Folder> =>
  request<Folder>('/folders', { body: JSON.stringify({ name, parentId }), method: 'POST' });

export const updateFolder = (
  id: string,
  patch: { name?: string; parentId?: string | null }
): Promise<Folder> =>
  request<Folder>(`/folders/${id}`, { body: JSON.stringify(patch), method: 'PATCH' });

export const deleteFolder = (id: string): Promise<{ ok: boolean }> =>
  request<{ ok: boolean }>(`/folders/${id}`, { method: 'DELETE' });

export const listDiagrams = (
  query: { favorite?: boolean; folderId?: string; search?: string } = {}
): Promise<Diagram[]> => {
  const params = new URLSearchParams();
  if (query.folderId) {
    params.set('folderId', query.folderId);
  }
  if (query.search) {
    params.set('search', query.search);
  }
  if (query.favorite) {
    params.set('favorite', '1');
  }
  const qs = params.toString();
  return request<Diagram[]>(`/diagrams${qs ? `?${qs}` : ''}`);
};

export const createDiagram = (data: {
  folderId?: string | null;
  name: string;
  positions?: Record<string, NodeOffset>;
  state: string;
}): Promise<Diagram> =>
  request<Diagram>('/diagrams', { body: JSON.stringify(data), method: 'POST' });

export const getDiagram = (id: string): Promise<Diagram> => request<Diagram>(`/diagrams/${id}`);

export const updateDiagram = (
  id: string,
  patch: {
    favorite?: boolean;
    folderId?: string | null;
    name?: string;
    positions?: Record<string, NodeOffset>;
    state?: string;
  }
): Promise<Diagram> =>
  request<Diagram>(`/diagrams/${id}`, { body: JSON.stringify(patch), method: 'PATCH' });

export const deleteDiagram = (id: string): Promise<{ ok: boolean }> =>
  request<{ ok: boolean }>(`/diagrams/${id}`, { method: 'DELETE' });

export const exportAll = (): Promise<ExportData> => request<ExportData>('/export');

export const importAll = (data: {
  diagrams: unknown[];
  folders: unknown[];
}): Promise<{ imported: number; skipped: number }> =>
  request<{ imported: number; skipped: number }>('/import', {
    body: JSON.stringify(data),
    method: 'POST'
  });
