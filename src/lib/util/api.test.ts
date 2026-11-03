import { afterEach, describe, expect, it, vi } from 'vitest';
import { listFolders, listDiagrams, updateDiagram } from './api';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('api client', () => {
  it('parses successful responses as JSON', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          new Response(JSON.stringify([{ id: 'f1', name: 'Work' }]), { status: 200 })
        )
    );
    const folders = await listFolders();
    expect(folders).toEqual([{ id: 'f1', name: 'Work' }]);
  });

  it('sends method, body and query parameters', async () => {
    // A fresh Response per call, like real fetch: undici bodies are single-use,
    // so a shared instance would fail on the second request.
    const fetchMock = vi
      .fn()
      .mockImplementation(() => Promise.resolve(new Response('[]', { status: 200 })));
    vi.stubGlobal('fetch', fetchMock);
    await listDiagrams({ favorite: true, folderId: 'root', search: 'flow' });
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('/api/diagrams?folderId=root&search=flow&favorite=1');
    expect(init.method).toBeUndefined();
    await updateDiagram('d1', { name: 'New' });
    const [url2, init2] = fetchMock.mock.calls[1] as [string, RequestInit];
    expect(url2).toBe('/api/diagrams/d1');
    expect(init2.method).toBe('PATCH');
    expect(init2.body).toBe(JSON.stringify({ name: 'New' }));
  });

  it('throws with server error message on non-2xx', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          new Response(JSON.stringify({ error: 'folder not found' }), { status: 404 })
        )
    );
    await expect(updateDiagram('nope', { name: 'x' })).rejects.toThrow('folder not found');
  });

  it('throws with status when error body is not JSON', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('<html>', { status: 502 })));
    await expect(updateDiagram('d1', { name: 'x' })).rejects.toThrow('API request failed (502)');
  });
});
