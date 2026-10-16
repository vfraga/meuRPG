import { TestBed } from '@angular/core/testing';

import { ImportPartUploader, importPartPath } from './import-part-uploader';

// With --coverage test-setup.ts only hooks the first file of a worker, so the stubbed globals this file leaves on would
// reach the next file (a real setTimeout never fires on a fake clock).
afterEach(() => {
  vi.unstubAllGlobals();
});

/** Just enough of XMLHttpRequest to drive `ImportPartUploader` by hand. */
class FakeXhr {
  static last: FakeXhr;
  method = '';
  url = '';
  status = 0;
  responseText = '';
  responseType = '';
  body: unknown = null;
  requestHeaders: Record<string, string> = {};
  aborted = false;
  private readonly listeners: Record<string, (() => void)[]> = {};
  readonly upload = {
    progressListeners: [] as ((e: {
      lengthComputable: boolean;
      loaded: number;
      total: number;
    }) => void)[],
    addEventListener(
      _type: string,
      fn: (e: { lengthComputable: boolean; loaded: number; total: number }) => void,
    ) {
      this.progressListeners.push(fn);
    },
  };

  constructor() {
    FakeXhr.last = this;
  }

  open(method: string, url: string): void {
    this.method = method;
    this.url = url;
  }

  setRequestHeader(name: string, value: string): void {
    this.requestHeaders[name] = value;
  }

  addEventListener(type: string, fn: () => void): void {
    (this.listeners[type] ??= []).push(fn);
  }

  send(body: unknown): void {
    this.body = body;
  }

  abort(): void {
    this.aborted = true;
    this.fire('abort');
  }

  respond(status: number, body = ''): void {
    this.status = status;
    this.responseText = body;
    this.fire('load');
  }

  fire(type: string): void {
    for (const fn of this.listeners[type] ?? []) {
      fn();
    }
  }
}

describe('ImportPartUploader', () => {
  let uploader: ImportPartUploader;
  const bytes = new Blob(['part bytes']);

  beforeEach(() => {
    vi.stubGlobal('XMLHttpRequest', FakeXhr);
    uploader = TestBed.inject(ImportPartUploader);
  });

  it('PUTs the raw bytes of the part to its own route, with no headers of its own', async () => {
    const done = uploader.upload('imp-1', 3, bytes);
    const xhr = FakeXhr.last;
    expect([xhr.method, xhr.url]).toEqual(['PUT', '/uploads/campaign-imports/imp-1/parts/3']);
    expect(xhr.body).toBe(bytes);
    expect(xhr.requestHeaders).toEqual({});

    xhr.respond(204);
    await expect(done).resolves.toBeUndefined();
  });

  it('keeps an odd id inside the path', () => {
    expect(importPartPath('a/b c', 1)).toBe('/uploads/campaign-imports/a%2Fb%20c/parts/1');
  });

  it('reports the progress of the part from 0 to 1', () => {
    const progress: number[] = [];
    void uploader.upload('imp-1', 1, bytes, { onProgress: (f) => progress.push(f) });
    for (const fn of FakeXhr.last.upload.progressListeners) {
      fn({ lengthComputable: true, loaded: 25, total: 100 });
      fn({ lengthComputable: false, loaded: 0, total: 0 });
    }
    expect(progress).toEqual([0.25]);
  });

  it('rejects with the reason the server gave, by the JSON and never the message', async () => {
    const done = uploader.upload('imp-1', 1, bytes);
    FakeXhr.last.respond(404, '{"code":"not_found","reason":"","message":"import gone"}');
    await expect(done).rejects.toMatchObject({ kind: 'NOT_FOUND' });
  });

  it('maps a plain-text refusal by its status', async () => {
    const done = uploader.upload('imp-1', 1, bytes);
    FakeXhr.last.respond(403, 'forbidden');
    await expect(done).rejects.toMatchObject({ kind: 'PERMISSION_DENIED' });
  });

  it('rejects with NETWORK when the connection drops or times out', async () => {
    const dropped = uploader.upload('imp-1', 1, bytes);
    FakeXhr.last.fire('error');
    await expect(dropped).rejects.toMatchObject({ kind: 'NETWORK' });
    const slow = uploader.upload('imp-1', 1, bytes);
    FakeXhr.last.fire('timeout');
    await expect(slow).rejects.toMatchObject({ kind: 'NETWORK' });
  });

  it('cancels the request when the signal aborts, and not even starts when it already has', async () => {
    const controller = new AbortController();
    const canceled = uploader.upload('imp-1', 1, bytes, { signal: controller.signal });
    controller.abort();
    expect(FakeXhr.last.aborted).toBe(true);
    await expect(canceled).rejects.toMatchObject({ kind: 'CANCELED' });

    const before = FakeXhr.last;
    await expect(
      uploader.upload('imp-1', 2, bytes, { signal: controller.signal }),
    ).rejects.toMatchObject({ kind: 'CANCELED' });
    expect(FakeXhr.last).toBe(before);
  });
});
