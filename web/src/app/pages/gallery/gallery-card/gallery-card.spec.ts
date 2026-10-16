import { ComponentFixture, TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';

import type { GalleryImage } from '../../../../gen/meurpg/maps/v1/gallery_pb';
import { GalleryClient } from '../../../core/images/gallery-client';
import { FakeGalleryClient, galleryImage, plain } from '../../../core/images/gallery-testing';
import { GalleryCard } from './gallery-card';
import { nextTurn } from '../../../../test-settle';

describe('GalleryCard', () => {
  let gallery: FakeGalleryClient;
  let fixture: ComponentFixture<GalleryCard>;
  let el: HTMLElement;
  let renamed: GalleryImage[];
  let deleted: GalleryImage[];

  beforeEach(async () => {
    TestBed.configureTestingModule({
      imports: [GalleryCard],
      providers: [{ provide: GalleryClient, useClass: FakeGalleryClient }],
    });
    gallery = TestBed.inject(GalleryClient) as unknown as FakeGalleryClient;
    fixture = TestBed.createComponent(GalleryCard);
    fixture.componentRef.setInput(
      'image',
      galleryImage('img-taverna', 'Taverna do Javali', { byteSize: 1258291 }),
    );
    fixture.componentRef.setInput('campaignId', 'camp-1');
    renamed = [];
    deleted = [];
    fixture.componentInstance.renamed.subscribe((i) => renamed.push(i));
    fixture.componentInstance.deleted.subscribe((i) => deleted.push(i));
    fixture.detectChanges();
    el = fixture.nativeElement as HTMLElement;
  });

  async function settle(): Promise<void> {
    await nextTurn();
    await fixture.whenStable();
    fixture.detectChanges();
  }

  function button(name: string): HTMLButtonElement {
    const found = Array.from(el.querySelectorAll('button')).find(
      (b) => (b.getAttribute('aria-label') ?? b.textContent?.trim()) === name,
    );
    if (!found) {
      throw new Error(`no button "${name}"`);
    }
    return found;
  }

  function nameField(): HTMLInputElement {
    return el.querySelector('input') as HTMLInputElement;
  }

  it('shows the name, the size and the three actions, each naming the image', () => {
    expect(el.querySelector('.card__name')?.textContent?.trim()).toBe('Taverna do Javali');
    expect(plain(el.querySelector('.card__meta')?.textContent).trim()).toBe(
      '1920 × 1080 px, 1,2 MB',
    );
    for (const name of [
      'Ver Taverna do Javali',
      'Renomear Taverna do Javali',
      'Apagar Taverna do Javali',
    ]) {
      expect(button(name)).toBeTruthy();
    }
  });

  it('renames in place with "Salvar nome"', async () => {
    button('Renomear Taverna do Javali').click();
    await settle();
    expect(nameField().value).toBe('Taverna do Javali');

    nameField().value = '  Taverna do Javali, andar de cima ';
    nameField().dispatchEvent(new Event('input'));
    button('Salvar nome').click();
    await settle();

    expect(gallery.calls).toContainEqual([
      'rename',
      'camp-1',
      'img-taverna',
      'Taverna do Javali, andar de cima',
    ]);
    expect(renamed.map((i) => i.name)).toEqual(['Taverna do Javali, andar de cima']);
    expect(el.querySelector('input')).toBeNull();
  });

  it('refuses an empty name on the client, with the field’s error', async () => {
    button('Renomear Taverna do Javali').click();
    await settle();
    nameField().value = '   ';
    nameField().dispatchEvent(new Event('input'));
    button('Salvar nome').click();
    await settle();

    expect(el.textContent).toContain('Dê um nome à imagem.');
    expect(gallery.calls.some((c) => c[0] === 'rename')).toBe(false);
  });

  it('cancels a rename with Esc', async () => {
    button('Renomear Taverna do Javali').click();
    await settle();
    nameField().dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    await settle();
    expect(el.querySelector('input')).toBeNull();
    expect(el.querySelector('.card__name')?.textContent?.trim()).toBe('Taverna do Javali');
  });

  it('asks before deleting, and "Cancelar" keeps the image', async () => {
    button('Apagar Taverna do Javali').click();
    await settle();
    const group = el.querySelector('[role="group"]');
    expect(group?.textContent?.replace(/\s+/g, ' ')).toContain(
      'Apagar Taverna do Javali? Não dá para desfazer.',
    );
    expect(document.activeElement?.textContent?.trim()).toBe('Apagar imagem');

    button('Cancelar').click();
    await settle();
    expect(el.querySelector('[role="group"]')).toBeNull();
    expect(gallery.calls).toEqual([]);
  });

  it('says a map uses the image when the server refuses the delete', async () => {
    gallery.deleteResult = () =>
      Promise.reject(new ConnectError('in use', Code.FailedPrecondition));
    button('Apagar Taverna do Javali').click();
    await settle();
    button('Apagar imagem').click();
    await settle();

    expect(el.querySelector('[role="alert"]')?.textContent).toContain(
      'Essa imagem está num mapa. Troque a imagem do mapa antes de apagá-la.',
    );
    expect(deleted).toEqual([]);
  });

  it('treats an image already deleted elsewhere as deleted', async () => {
    gallery.deleteResult = () => Promise.reject(new ConnectError('gone', Code.NotFound));
    button('Apagar Taverna do Javali').click();
    await settle();
    button('Apagar imagem').click();
    await settle();
    expect(deleted.map((i) => i.id)).toEqual(['img-taverna']);
  });
  it('tags an image the app generated "Gerada por IA", and an adjustment says which image it came from; a plain image has neither', async () => {
    expect(plain(el.textContent)).not.toContain('Gerada por IA');
    fixture.componentRef.setInput(
      'image',
      galleryImage('img-2', 'Imagem 2', { generated: true, parentImageId: 'img-1' }),
    );
    fixture.componentRef.setInput('parentName', 'Imagem 1');
    await settle();
    expect(plain(el.textContent)).toContain('Gerada por IA');
    expect(plain(el.textContent)).toContain('Editada de Imagem 1');
    fixture.componentRef.setInput('image', galleryImage('img-1', 'Imagem 1', { generated: true }));
    fixture.componentRef.setInput('parentName', '');
    await settle();
    expect(plain(el.textContent)).toContain('Gerada por IA');
    expect(plain(el.textContent)).not.toContain('Editada de');
  });
});
