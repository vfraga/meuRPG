import { ComponentFixture, TestBed } from '@angular/core/testing';

import type { GalleryImage } from '../../../gen/meurpg/maps/v1/gallery_pb';
import { GalleryClient } from '../../core/images/gallery-client';
import {
  FakeGalleryClient,
  FakeImageUploader,
  galleryImage,
  galleryUsage,
  mirathelImages,
} from '../../core/images/gallery-testing';
import { ImageUploader } from '../../core/images/image-uploader';
import { GalleryPicker } from './gallery-picker';

const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

describe('GalleryPicker', () => {
  let gallery: FakeGalleryClient;
  let uploader: FakeImageUploader;
  let fixture: ComponentFixture<GalleryPicker>;
  let el: HTMLElement;
  let picked: GalleryImage[];

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [GalleryPicker],
      providers: [
        { provide: GalleryClient, useClass: FakeGalleryClient },
        { provide: ImageUploader, useClass: FakeImageUploader },
      ],
    });
    gallery = TestBed.inject(GalleryClient) as unknown as FakeGalleryClient;
    uploader = TestBed.inject(ImageUploader) as unknown as FakeImageUploader;
    const images = mirathelImages();
    gallery.listResult = Promise.resolve({ images, usage: galleryUsage(images) });
  });

  async function render(selectedId: string | null = null): Promise<void> {
    fixture = TestBed.createComponent(GalleryPicker);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('selectedId', selectedId);
    fixture.componentRef.setInput('label', 'Imagem do mapa');
    picked = [];
    fixture.componentInstance.picked.subscribe((i) => picked.push(i));
    fixture.detectChanges();
    await settle();
    el = fixture.nativeElement as HTMLElement;
  }

  async function settle(): Promise<void> {
    await flush();
    await fixture.whenStable();
    fixture.detectChanges();
  }

  const radios = () => Array.from(el.querySelectorAll<HTMLButtonElement>('[role="radio"]'));
  const checked = () => radios().filter((r) => r.getAttribute('aria-checked') === 'true');
  const tabStops = () => radios().filter((r) => r.tabIndex === 0);

  it('is a named radio group of the gallery’s images, one Tab stop, nothing checked yet', async () => {
    await render();
    expect(gallery.calls).toEqual([['list', 'camp-1']]);
    expect(el.querySelector('[role="radiogroup"]')?.getAttribute('aria-label')).toBe(
      'Imagem do mapa',
    );
    expect(radios().map((r) => r.textContent?.replace(/\s+/g, ' ').trim())).toEqual([
      'Covil dos goblins, 2000 × 1400 px',
      'Taverna do Javali, 1920 × 1080 px',
      'Capitão Goblin, 800 × 1000 px',
      'Planta da torre, 1600 × 1600 px',
      'Mapa de Mirathel, 2400 × 1600 px',
    ]);
    expect(checked()).toEqual([]);
    expect(tabStops()).toEqual([radios()[0]]);
    expect(el.textContent).toContain(
      'Use imagens do jogo. Não envie fotos de pessoas sem a autorização delas.',
    );
  });

  it('checks the image the form already has, and makes it the Tab stop', async () => {
    await render('img-capitao');
    expect(checked().map((r) => r.textContent)).toEqual([
      expect.stringContaining('Capitão Goblin'),
    ]);
    expect(tabStops()).toEqual(checked());
  });

  it('checks a tile on click and emits the image, two-way', async () => {
    await render();
    radios()[1].click();
    fixture.detectChanges();
    expect(picked.map((i) => i.id)).toEqual(['img-taverna']);
    expect(fixture.componentInstance.selectedId()).toBe('img-taverna');
    expect(checked()).toEqual([radios()[1]]);
  });

  it('moves and checks with the arrow keys, wrapping, and with Home/End', async () => {
    await render('img-taverna');
    const press = (k: string) => {
      document.activeElement?.dispatchEvent(
        new KeyboardEvent('keydown', { key: k, bubbles: true }),
      );
      fixture.detectChanges();
    };
    radios()[1].focus();
    press('ArrowRight');
    expect(document.activeElement).toBe(radios()[2]);
    press('ArrowUp');
    press('ArrowLeft');
    expect(document.activeElement).toBe(radios()[0]);
    press('ArrowLeft');
    expect(document.activeElement).toBe(radios()[4]);
    press('Home');
    press('End');
    expect(picked.map((i) => i.id)).toEqual([
      'img-capitao',
      'img-taverna',
      'img-covil',
      'img-mapa',
      'img-covil',
      'img-mapa',
    ]);
    expect(checked()).toEqual([radios()[4]]);
  });

  it('uploads through "Enviar imagem" and checks the new image when it is ready', async () => {
    await render('img-covil');
    const input = el.querySelector('input[type="file"]') as HTMLInputElement;
    Object.defineProperty(input, 'files', {
      value: [new File(['x'], 'ruinas.jpg', { type: 'image/jpeg' })],
      configurable: true,
    });
    input.dispatchEvent(new Event('change'));
    fixture.detectChanges();
    expect(el.querySelector('[role="progressbar"]')).not.toBeNull();

    uploader.pending[0].resolve(galleryImage('img-ruinas', 'ruinas'));
    await settle();
    expect(radios()[0].textContent).toContain('ruinas');
    expect(checked()).toEqual([radios()[0]]);
    expect(picked.map((i) => i.id)).toEqual(['img-ruinas']);
    expect(fixture.componentInstance.selectedId()).toBe('img-ruinas');
  });

  it('refuses a GIF with the gallery’s own words', async () => {
    await render();
    const input = el.querySelector('input[type="file"]') as HTMLInputElement;
    Object.defineProperty(input, 'files', {
      value: [new File(['GIF89a'], 'mapa-antigo.gif', { type: 'image/gif' })],
      configurable: true,
    });
    input.dispatchEvent(new Event('change'));
    fixture.detectChanges();
    expect(el.querySelector('[role="alert"]')?.textContent?.replace(/\s+/g, ' ')).toContain(
      'Não deu para enviar mapa-antigo.gif. Esse arquivo não é uma imagem JPEG, PNG ou WebP.',
    );
    expect(uploader.pending).toEqual([]);
  });

  it('with an empty gallery, offers only the upload (no empty radio group)', async () => {
    gallery.listResult = Promise.resolve({ images: [], usage: galleryUsage() });
    await render();
    expect(el.querySelector('[role="radiogroup"]')).toBeNull();
    expect(el.textContent).toContain('A galeria ainda não tem imagens. Envie a primeira.');
    expect(el.textContent).toContain('Enviar imagem');
  });
  it('leaves out the images it is told to (the portraits of NPCs the players do not see)', async () => {
    fixture = TestBed.createComponent(GalleryPicker);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('excluded', new Set(['img-capitao']));
    fixture.detectChanges();
    await settle();
    el = fixture.nativeElement as HTMLElement;
    const names = radios().map((r) => r.textContent?.replace(/\s+/g, ' ').trim());
    expect(names.some((n) => n?.includes('Capitão Goblin'))).toBe(false);
    expect(names.some((n) => n?.includes('Covil dos goblins'))).toBe(true);
  });

  it('draws the images of the campaign it shows when an answer for the one before comes last', async () => {
    const answers = new Map<string, (images: GalleryImage[]) => void>();
    gallery.list = (campaignId: string) =>
      new Promise((resolve) => {
        answers.set(campaignId, (images) => resolve({ images, usage: galleryUsage(images) }));
      });
    fixture = TestBed.createComponent(GalleryPicker);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.detectChanges();
    fixture.componentRef.setInput('campaignId', 'camp-2');
    fixture.detectChanges();
    answers.get('camp-2')!([galleryImage('img-2', 'Da segunda')]);
    await settle();
    answers.get('camp-1')!([galleryImage('img-1', 'Da primeira')]);
    await settle();
    el = fixture.nativeElement as HTMLElement;
    expect(radios().map((r) => r.textContent)).toEqual([expect.stringContaining('Da segunda')]);
  });
});
