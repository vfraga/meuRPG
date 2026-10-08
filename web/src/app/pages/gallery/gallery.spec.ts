import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';
import { BehaviorSubject, of } from 'rxjs';

import { GalleryClient, type GalleryListing } from '../../core/images/gallery-client';
import {
  FakeGalleryClient,
  FakeImageUploader,
  galleryImage,
  galleryUsage,
  mirathelImages,
  plain,
} from '../../core/images/gallery-testing';
import { ImageUploader } from '../../core/images/image-uploader';
import { UploadFailed } from '../../core/images/upload-errors';
import { GalleryPage } from './gallery';

const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

describe('GalleryPage', () => {
  let gallery: FakeGalleryClient;
  let uploader: FakeImageUploader;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [GalleryPage],
      providers: [
        provideRouter([]),
        {
          provide: ActivatedRoute,
          useValue: { paramMap: of(convertToParamMap({ id: 'camp-1' })) },
        },
        { provide: GalleryClient, useClass: FakeGalleryClient },
        { provide: ImageUploader, useClass: FakeImageUploader },
      ],
    });
    gallery = TestBed.inject(GalleryClient) as unknown as FakeGalleryClient;
    uploader = TestBed.inject(ImageUploader) as unknown as FakeImageUploader;
  });

  async function render(): Promise<{ el: HTMLElement; fixture: ComponentFixture<GalleryPage> }> {
    const fixture = TestBed.createComponent(GalleryPage);
    fixture.detectChanges();
    await settle(fixture);
    return { el: fixture.nativeElement as HTMLElement, fixture };
  }

  async function settle(fixture: ComponentFixture<GalleryPage>): Promise<void> {
    await flush();
    await fixture.whenStable();
    fixture.detectChanges();
  }

  function buttons(el: HTMLElement, name: string): HTMLButtonElement[] {
    return Array.from(el.querySelectorAll('button')).filter(
      (b) => (b.getAttribute('aria-label') ?? b.textContent?.trim()) === name,
    );
  }

  function pick(el: HTMLElement, files: File[]): void {
    const input = el.querySelector('input[type="file"]') as HTMLInputElement;
    Object.defineProperty(input, 'files', { value: files, configurable: true });
    input.dispatchEvent(new Event('change'));
  }

  it('lists the images newest first, with the quota line (E5-20)', async () => {
    const images = mirathelImages();
    gallery.listResult = Promise.resolve({ images, usage: galleryUsage(images) });
    const { el } = await render();

    expect(gallery.calls).toEqual([['list', 'camp-1']]);
    expect(el.querySelector('h1')?.textContent).toContain('Galeria');
    const names = Array.from(el.querySelectorAll('.card__name')).map((n) => n.textContent?.trim());
    expect(names).toEqual([
      'Covil dos goblins',
      'Taverna do Javali',
      'Capitão Goblin',
      'Planta da torre',
      'Mapa de Mirathel',
    ]);
    expect(plain(el.textContent)).toContain('5 imagens · 6 MB de 500 MB');
    expect(el.textContent).toContain(
      'Use imagens do jogo. Não envie fotos de pessoas sem a autorização delas.',
    );
    const thumb = el.querySelector('img') as HTMLImageElement;
    expect(thumb.getAttribute('src')).toBe('/images/img-covil/thumb');
    expect(thumb.getAttribute('alt')).toBe('Covil dos goblins');
    expect(thumb.getAttribute('width')).toBe('2000');
  });

  it('invites the first upload when the gallery is empty (E5-22)', async () => {
    const { el } = await render();
    expect(el.textContent).toContain('Nenhuma imagem ainda');
    expect(el.querySelector('.grid')).toBeNull();
    expect(el.querySelectorAll('button.mat-mdc-unelevated-button').length).toBe(1);
  });

  it('tells a player, calmly, that only the master sees the gallery', async () => {
    gallery.listResult = Promise.reject(new ConnectError('player', Code.PermissionDenied));
    const { el } = await render();
    expect(el.textContent).toContain('Só o mestre vê a galeria da campanha.');
    expect(el.querySelector('input[type="file"]')).toBeNull();
  });

  it('says "campanha não encontrada" to a non-member, as for a campaign that does not exist', async () => {
    gallery.listResult = Promise.reject(new ConnectError('nope', Code.NotFound));
    const { el } = await render();
    expect(el.querySelector('h1')?.textContent).toContain('Campanha não encontrada');
  });

  it('uploads the picked files one by one; each new image joins the start of the grid', async () => {
    const images = mirathelImages();
    gallery.listResult = Promise.resolve({ images, usage: galleryUsage(images) });
    const { el, fixture } = await render();

    pick(el, [new File(['x'], 'ruinas.jpg', { type: 'image/jpeg' })]);
    fixture.detectChanges();
    expect(el.querySelector('[role="progressbar"]')?.getAttribute('aria-label')).toBe(
      'Enviando ruinas.jpg',
    );
    expect(uploader.pending.map((p) => [p.campaignId, p.file.name])).toEqual([
      ['camp-1', 'ruinas.jpg'],
    ]);

    uploader.pending[0].resolve(galleryImage('img-ruinas', 'ruinas', { byteSize: 1024 * 1024 }));
    await settle(fixture);
    expect(el.querySelector('[role="progressbar"]')).toBeNull();
    expect(el.querySelector('.card__name')?.textContent?.trim()).toBe('ruinas');
    expect(plain(el.textContent)).toContain('6 imagens · 7 MB de 500 MB');
  });

  it('shows one danger notice per refused file, naming it, and leaves the grid alone', async () => {
    const images = mirathelImages();
    gallery.listResult = Promise.resolve({ images, usage: galleryUsage(images) });
    const { el, fixture } = await render();

    pick(el, [
      new File(['GIF89a'], 'mapa-antigo.gif', { type: 'image/gif' }),
      new File(['text'], 'nota.png', { type: 'image/png' }),
    ]);
    fixture.detectChanges();
    uploader.pending[0].reject(new UploadFailed('UNSUPPORTED_TYPE'));
    await settle(fixture);

    const notices = Array.from(el.querySelectorAll('.mr-notice--danger p')).map((n) =>
      n.textContent?.replace(/\s+/g, ' ').trim(),
    );
    expect(notices).toEqual([
      'Não deu para enviar mapa-antigo.gif. Esse arquivo não é uma imagem JPEG, PNG ou WebP.',
      'Não deu para enviar nota.png. Esse arquivo não é uma imagem JPEG, PNG ou WebP.',
    ]);
    expect(el.querySelectorAll('.card__name').length).toBe(5);
  });

  it('deletes after the in-place confirmation, and updates the quota', async () => {
    const images = mirathelImages();
    gallery.listResult = Promise.resolve({ images, usage: galleryUsage(images) });
    const { el, fixture } = await render();

    buttons(el, 'Apagar Capitão Goblin')[0].click();
    await settle(fixture);
    expect(el.textContent).toContain('Apagar Capitão Goblin? Não dá para desfazer.');
    expect(gallery.calls.some((c) => c[0] === 'delete')).toBe(false);

    buttons(el, 'Apagar imagem')[0].click();
    await settle(fixture);
    expect(gallery.calls).toContainEqual(['delete', 'camp-1', 'img-capitao']);
    const names = Array.from(el.querySelectorAll('.card__name')).map((n) => n.textContent?.trim());
    expect(names).not.toContain('Capitão Goblin');
    expect(el.textContent).toContain('4 imagens');
  });

  it('opens the lightbox on "Ver" with "2 de 5", and turns the page with the arrows', async () => {
    const images = mirathelImages();
    gallery.listResult = Promise.resolve({ images, usage: galleryUsage(images) });
    const { el, fixture } = await render();

    buttons(el, 'Ver Taverna do Javali')[0].click();
    await settle(fixture);
    const dialog = el.querySelector('dialog') as HTMLDialogElement;
    expect(dialog.hasAttribute('open')).toBe(true);
    expect(dialog.querySelector('h2')?.textContent).toBe('Taverna do Javali');
    expect(dialog.querySelector('.lightbox__full')?.getAttribute('src')).toBe(
      '/images/img-taverna',
    );
    expect(dialog.textContent).toContain('2 de 5');

    dialog.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight' }));
    await settle(fixture);
    expect(dialog.querySelector('h2')?.textContent).toBe('Capitão Goblin');
    expect(dialog.textContent).toContain('3 de 5');
  });

  it('turns the quota line into a warning from 90% of a limit', async () => {
    const images = mirathelImages();
    gallery.listResult = Promise.resolve({
      images,
      usage: galleryUsage(images, { imageCount: 280 }),
    });
    const { el } = await render();
    const warning = el.querySelector('.mr-notice--warning');
    expect(warning?.textContent?.replace(/\s+/g, ' ')).toContain(
      'A galeria está quase cheia: 280 imagens · 6 MB de 500 MB.',
    );
    expect(el.querySelector('.quota')).toBeNull();
  });

  it('takes files dropped anywhere on the page, lighting up the zone while they are over it', async () => {
    const images = mirathelImages();
    gallery.listResult = Promise.resolve({ images, usage: galleryUsage(images) });
    const { el, fixture } = await render();
    const files = [new File(['x'], 'solto.png', { type: 'image/png' })];
    const drag = (type: string) => {
      const event = new Event(type, { bubbles: true, cancelable: true });
      Object.defineProperty(event, 'dataTransfer', {
        value: { types: ['Files'], files, dropEffect: 'none' },
      });
      document.body.dispatchEvent(event);
      return event;
    };

    expect(drag('dragover').defaultPrevented).toBe(true);
    fixture.detectChanges();
    expect(el.querySelector('.zone.dragging')).not.toBeNull();

    expect(drag('drop').defaultPrevented).toBe(true);
    fixture.detectChanges();
    expect(el.querySelector('.zone.dragging')).toBeNull();
    expect(uploader.pending.map((p) => p.file.name)).toEqual(['solto.png']);
  });

  it('hands "Renomear" from the lightbox to the card, with the field focused', async () => {
    const images = mirathelImages();
    gallery.listResult = Promise.resolve({ images, usage: galleryUsage(images) });
    const { el, fixture } = await render();

    buttons(el, 'Ver Planta da torre')[0].click();
    await settle(fixture);
    const dialog = el.querySelector('dialog') as HTMLDialogElement;
    buttons(dialog, 'Renomear Planta da torre')[0].click();
    await settle(fixture);

    expect(dialog.hasAttribute('open')).toBe(false);
    const field = el.querySelector('app-gallery-card input') as HTMLInputElement;
    expect(field.value).toBe('Planta da torre');
    expect(document.activeElement).toBe(field);
  });

  it('offers "Tentar de novo" when the server does not answer', async () => {
    gallery.listResult = Promise.reject(new ConnectError('down', Code.Unavailable));
    const { el, fixture } = await render();
    expect(el.querySelector('h1')?.textContent).toContain('Não foi possível abrir a galeria');

    const images = mirathelImages();
    gallery.listResult = Promise.resolve({ images, usage: galleryUsage(images) });
    buttons(el, 'Tentar de novo')[0].click();
    await settle(fixture);
    expect(el.querySelectorAll('.card__name').length).toBe(5);
  });
});

describe('GalleryPage when the campaign in the route changes', () => {
  let params: BehaviorSubject<ReturnType<typeof convertToParamMap>>;
  let gallery: FakeGalleryClient;
  let uploader: FakeImageUploader;

  beforeEach(() => {
    params = new BehaviorSubject(convertToParamMap({ id: 'camp-A' }));
    TestBed.configureTestingModule({
      imports: [GalleryPage],
      providers: [
        provideRouter([]),
        { provide: ActivatedRoute, useValue: { paramMap: params } },
        { provide: GalleryClient, useClass: FakeGalleryClient },
        { provide: ImageUploader, useClass: FakeImageUploader },
      ],
    });
    gallery = TestBed.inject(GalleryClient) as unknown as FakeGalleryClient;
    uploader = TestBed.inject(ImageUploader) as unknown as FakeImageUploader;
  });

  function deferred() {
    let resolve!: (v: GalleryListing) => void;
    const promise = new Promise<GalleryListing>((r) => (resolve = r));
    return { promise, resolve };
  }

  it('shows the images of the new campaign when the old one answers last', async () => {
    const imgA = galleryImage('img-a', 'Imagem de A', { campaignId: 'camp-A' });
    const imgB = galleryImage('img-b', 'Imagem de B', { campaignId: 'camp-B' });
    const a = deferred();
    const b = deferred();

    gallery.listResult = a.promise;
    const fixture = TestBed.createComponent(GalleryPage);
    fixture.detectChanges();
    gallery.listResult = b.promise;
    params.next(convertToParamMap({ id: 'camp-B' }));
    expect(gallery.calls).toEqual([
      ['list', 'camp-A'],
      ['list', 'camp-B'],
    ]);

    // B answers first, then the slow answer of A arrives last.
    b.resolve({ images: [imgB], usage: galleryUsage([imgB]) });
    await flush();
    a.resolve({ images: [imgA], usage: galleryUsage([imgA]) });
    await flush();
    fixture.detectChanges();

    const names = Array.from(
      (fixture.nativeElement as HTMLElement).querySelectorAll('.card__name'),
    ).map((n) => n.textContent?.trim());
    expect(names).toEqual(['Imagem de B']);
  });

  it('does not upload a file queued on the old campaign into the new one', async () => {
    const a = deferred();
    gallery.listResult = a.promise;
    const fixture = TestBed.createComponent(GalleryPage);
    fixture.detectChanges();
    a.resolve({ images: [], usage: galleryUsage() });
    await flush();
    fixture.detectChanges();

    const el = fixture.nativeElement as HTMLElement;
    const input = el.querySelector('input[type="file"]') as HTMLInputElement;
    const files = [
      new File(['1'], 'um.jpg', { type: 'image/jpeg' }),
      new File(['2'], 'dois.jpg', { type: 'image/jpeg' }),
    ];
    Object.defineProperty(input, 'files', { value: files, configurable: true });
    input.dispatchEvent(new Event('change'));
    expect(uploader.pending.map((p) => p.campaignId)).toEqual(['camp-A']);

    // Same component instance moves to campaign B while the queue still holds "dois.jpg".
    params.next(convertToParamMap({ id: 'camp-B' }));
    uploader.pending[0].resolve(galleryImage('up-1', 'um', { campaignId: 'camp-A' }));
    await flush();

    // Whatever was dropped on A must never be sent to B.
    expect(uploader.pending.map((p) => p.campaignId)).not.toContain('camp-B');
  });
});
