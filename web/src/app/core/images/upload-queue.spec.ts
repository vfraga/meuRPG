import type { GalleryImage, GalleryUsage } from '../../../gen/meurpg/maps/v1/gallery_pb';
import { FakeImageUploader, galleryImage, galleryUsage, plain } from './gallery-testing';
import { UploadFailed } from './upload-errors';
import { UploadQueue } from './upload-queue';
import { nextTurn } from '../../../test-settle';

const jpeg = (name: string, size = 1000) =>
  new File([new Uint8Array(size)], name, { type: 'image/jpeg' });

describe('UploadQueue', () => {
  let uploader: FakeImageUploader;
  let usage: GalleryUsage | null;
  let uploaded: GalleryImage[];
  let queue: UploadQueue;

  beforeEach(() => {
    uploader = new FakeImageUploader();
    usage = galleryUsage();
    uploaded = [];
    queue = new UploadQueue({
      uploader,
      campaignId: () => 'camp-1',
      usage: () => usage,
      uploaded: (image) => uploaded.push(image),
    });
  });

  it('sends files one after the other, each with its own card', async () => {
    queue.add([jpeg('a.jpg'), jpeg('b.jpg')]);
    expect(queue.items().map((i) => [i.fileName, i.status])).toEqual([
      ['a.jpg', 'sending'],
      ['b.jpg', 'queued'],
    ]);
    expect(uploader.pending.map((p) => p.campaignId)).toEqual(['camp-1']);

    uploader.pending[0].resolve(galleryImage('img-a', 'a'));
    await nextTurn();
    expect(uploaded.map((i) => i.id)).toEqual(['img-a']);
    expect(queue.items().map((i) => [i.fileName, i.status])).toEqual([['b.jpg', 'sending']]);

    uploader.pending[1].resolve(galleryImage('img-b', 'b'));
    await nextTurn();
    expect(uploaded.map((i) => i.id)).toEqual(['img-a', 'img-b']);
    expect(queue.items()).toEqual([]);
  });

  it('shows progress, then "processing" once every byte went out', () => {
    queue.add([jpeg('a.jpg')]);
    uploader.pending[0].options.onProgress?.(0.6);
    expect(queue.items()[0]).toMatchObject({ status: 'sending', percent: 60 });
    uploader.pending[0].options.onProgress?.(1);
    expect(queue.items()[0]).toMatchObject({ status: 'processing', percent: 100 });
  });

  it('refuses a GIF and a 12 MB file on the client, with one notice each, and sends the rest', () => {
    const gif = new File(['GIF89a'], 'mapa-antigo.gif', { type: 'image/gif' });
    const big = jpeg('grande.jpg', 12 * 1024 * 1024);
    queue.add([gif, big, jpeg('ok.jpg')]);

    expect(queue.failures().map((f) => [f.fileName, f.kind, plain(f.message)])).toEqual([
      ['mapa-antigo.gif', 'UNSUPPORTED_TYPE', 'Esse arquivo não é uma imagem JPEG, PNG ou WebP.'],
      ['grande.jpg', 'TOO_LARGE', 'A imagem passa de 10 MB.'],
    ]);
    expect(uploader.pending.map((p) => p.file.name)).toEqual(['ok.jpg']);
  });

  it('turns a server refusal into that file’s notice, and goes on with the next file', async () => {
    queue.add([jpeg('texto.png'), jpeg('b.jpg')]);
    uploader.pending[0].reject(new UploadFailed('UNSUPPORTED_TYPE'));
    await nextTurn();
    expect(queue.failures().map((f) => f.fileName)).toEqual(['texto.png']);
    expect(queue.items().map((i) => i.fileName)).toEqual(['b.jpg']);
    expect(uploader.pending.length).toBe(2);
  });

  it('counts the files already in line against the image limit', () => {
    usage = galleryUsage([], { imageCount: 299 });
    queue.add([jpeg('a.jpg'), jpeg('b.jpg')]);
    expect(queue.items().map((i) => i.fileName)).toEqual(['a.jpg']);
    expect(queue.failures().map((f) => [f.fileName, f.kind])).toEqual([['b.jpg', 'QUOTA']]);
  });

  it('cancels the file on its way without a notice, and takes a waiting one out of line', async () => {
    queue.add([jpeg('a.jpg'), jpeg('b.jpg'), jpeg('c.jpg')]);
    const [a, b] = queue.items();
    queue.cancel(b.key);
    expect(queue.items().map((i) => i.fileName)).toEqual(['a.jpg', 'c.jpg']);

    queue.cancel(a.key);
    await nextTurn();
    expect(uploader.pending[0].options.signal?.aborted).toBe(true);
    expect(queue.failures()).toEqual([]);
    expect(uploader.pending.map((p) => p.file.name)).toEqual(['a.jpg', 'c.jpg']);
  });

  it('clears the last batch’s notices when a new batch starts', () => {
    queue.add([new File(['x'], 'a.gif', { type: 'image/gif' })]);
    expect(queue.failures().length).toBe(1);
    queue.add([jpeg('b.jpg')]);
    expect(queue.failures()).toEqual([]);
  });

  it('aborts the upload in flight on destroy, and reports nothing after', async () => {
    queue.add([jpeg('a.jpg'), jpeg('b.jpg')]);
    queue.destroy();
    expect(uploader.pending[0].options.signal?.aborted).toBe(true);
    await nextTurn();
    expect(queue.items()).toEqual([]);
    expect(uploader.pending.length).toBe(1);
    expect(uploaded).toEqual([]);
  });
});
