import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import type { GalleryImage } from '../../../../gen/meurpg/maps/v1/gallery_pb';
import { GalleryClient } from '../../../core/images/gallery-client';
import {
  FakeGalleryClient,
  FakeImageUploader,
  galleryImage,
  galleryUsage,
  plain,
} from '../../../core/images/gallery-testing';
import { ImageUploader } from '../../../core/images/image-uploader';
import { type ImagePickerData, ImagePickerDialog } from './image-picker-dialog';
import { nextTurn } from '../../../../test-settle';

// RN-10: a picture of the whole map (the textured map and its edits) is tagged in the picker, and showing it asks first, in place.
describe('ImagePickerDialog and a picture of the whole map', () => {
  let submitted: string[];
  let closed: unknown[];

  async function setup(extra: Partial<ImagePickerData> = {}) {
    const images: GalleryImage[] = [
      galleryImage('img-tx', 'Mapa com textura', { generated: true, showsWholeMap: true }),
      galleryImage('img-carta', 'Carta do rei'),
    ];
    submitted = [];
    closed = [];
    const data: ImagePickerData = {
      campaignId: 'camp-1',
      title: 'Mostrar uma imagem aos jogadores',
      confirmLabel: 'Mostrar aos jogadores',
      current: null,
      currentTag: 'À mostra agora',
      currentNote: '',
      note: () => 'Os jogadores veem a imagem na hora.',
      hiddenMapImages: new Map(),
      emptyError: 'Escolha uma imagem para mostrar.',
      submit: (image) => {
        submitted.push(image.id);
        return Promise.resolve();
      },
      errorMessage: () => 'erro',
      ...extra,
    };
    const gallery = new FakeGalleryClient();
    gallery.listResult = Promise.resolve({ images, usage: galleryUsage(images) });
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: GalleryClient, useValue: gallery },
        { provide: ImageUploader, useClass: FakeImageUploader },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: (r: unknown) => closed.push(r) } },
      ],
    });
    const fixture = TestBed.createComponent(ImagePickerDialog);
    fixture.detectChanges();
    await nextTurn();
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const settle = async () => {
      await nextTurn();
      fixture.detectChanges();
    };
    const tile = (name: string) =>
      Array.from(el.querySelectorAll<HTMLElement>('[role="radio"]')).find((r) =>
        plain(r.textContent).includes(name),
      )!;
    const button = (name: string) =>
      Array.from(el.querySelectorAll('button')).find((b) => plain(b.textContent).includes(name));
    return { fixture, el, settle, tile, button };
  }

  it('tags the picture of the whole map "Mapa inteiro", with an icon, and no other', async () => {
    const { tile } = await setup();
    expect(plain(tile('Mapa com textura').textContent)).toContain('Mapa inteiro');
    expect(plain(tile('Carta do rei').textContent)).not.toContain('Mapa inteiro');
  });

  it('confirming it asks first, in place; "Voltar" has the focus and shows nothing', async () => {
    const { el, settle, tile, button } = await setup({
      wholeMapConfirmLabel: 'Mostrar mesmo assim',
    });
    tile('Mapa com textura').click();
    await settle();
    button('Mostrar aos jogadores')!.click();
    await settle();
    expect(plain(el.textContent)).toContain('Mostrar o mapa inteiro?');
    expect(plain(el.textContent)).toContain(
      'Esta imagem mostra o mapa inteiro, também o que os jogadores ainda não descobriram.',
    );
    expect(submitted).toEqual([]);
    expect(document.activeElement?.textContent?.trim()).toBe('Voltar');
    button('Voltar')!.click();
    await settle();
    expect(plain(el.textContent)).not.toContain('Mostrar o mapa inteiro?');
    expect(submitted).toEqual([]);
    expect(closed).toEqual([]);
  });

  it('"Mostrar mesmo assim" shows it, and the dialog closes done', async () => {
    const { settle, tile, button } = await setup({ wholeMapConfirmLabel: 'Mostrar mesmo assim' });
    tile('Mapa com textura').click();
    await settle();
    button('Mostrar aos jogadores')!.click();
    await settle();
    button('Mostrar mesmo assim')!.click();
    await settle();
    expect(submitted).toEqual(['img-tx']);
    expect(closed).toEqual([true]);
  });

  it("any other picture is shown at once, and a picker that did not ask for the question (a map's image) never asks", async () => {
    const withQuestion = await setup({ wholeMapConfirmLabel: 'Mostrar mesmo assim' });
    withQuestion.tile('Carta do rei').click();
    await withQuestion.settle();
    withQuestion.button('Mostrar aos jogadores')!.click();
    await withQuestion.settle();
    expect(submitted).toEqual(['img-carta']);

    const without = await setup();
    without.tile('Mapa com textura').click();
    await without.settle();
    without.button('Mostrar aos jogadores')!.click();
    await without.settle();
    expect(submitted).toEqual(['img-tx']);
  });
});
