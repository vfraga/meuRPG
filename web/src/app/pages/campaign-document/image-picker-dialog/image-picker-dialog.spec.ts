import { TestBed } from '@angular/core/testing';

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
import { ImagePickerDialog } from './image-picker-dialog';
import { nextTurn } from '../../../../test-settle';

// RN-10: the players read the document, so a picture of the whole map asks before it goes in.
describe("the document's ImagePickerDialog and a picture of the whole map", () => {
  async function setup() {
    const images = [
      galleryImage('img-tx', 'Mapa com textura', { generated: true, showsWholeMap: true }),
      galleryImage('img-carta', 'Carta do rei'),
    ];
    const gallery = new FakeGalleryClient();
    gallery.listResult = Promise.resolve({ images, usage: galleryUsage(images) });
    TestBed.configureTestingModule({
      providers: [
        { provide: GalleryClient, useValue: gallery },
        { provide: ImageUploader, useClass: FakeImageUploader },
      ],
    });
    const fixture = TestBed.createComponent(ImagePickerDialog);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    const inserted: GalleryImage[] = [];
    fixture.componentInstance.picked.subscribe((i) => inserted.push(i));
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
    return { el, inserted, settle, tile, button };
  }

  it('inserts an ordinary image at once', async () => {
    const { inserted, settle, tile, button } = await setup();
    tile('Carta do rei').click();
    await settle();
    button('Inserir imagem')!.click();
    await settle();
    expect(inserted.map((i) => i.id)).toEqual(['img-carta']);
  });

  it('asks in place for a picture of the whole map: "Voltar" (focused) inserts nothing, "Inserir mesmo assim" inserts it', async () => {
    const { el, inserted, settle, tile, button } = await setup();
    tile('Mapa com textura').click();
    await settle();
    button('Inserir imagem')!.click();
    await settle();
    expect(plain(el.textContent)).toContain('Inserir o mapa inteiro?');
    expect(plain(el.textContent)).toContain(
      'Esta imagem mostra o mapa inteiro, também o que os jogadores ainda não descobriram',
    );
    expect(inserted).toEqual([]);
    expect(document.activeElement?.textContent?.trim()).toBe('Voltar');
    button('Voltar')!.click();
    await settle();
    expect(plain(el.textContent)).not.toContain('Inserir o mapa inteiro?');
    expect(inserted).toEqual([]);
    button('Inserir imagem')!.click();
    await settle();
    button('Inserir mesmo assim')!.click();
    await settle();
    expect(inserted.map((i) => i.id)).toEqual(['img-tx']);
  });

  it('choosing another image closes the question', async () => {
    const { el, settle, tile, button } = await setup();
    tile('Mapa com textura').click();
    await settle();
    button('Inserir imagem')!.click();
    await settle();
    tile('Carta do rei').click();
    await settle();
    expect(plain(el.textContent)).not.toContain('Inserir o mapa inteiro?');
  });
});
