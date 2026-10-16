import { ComponentFixture, TestBed } from '@angular/core/testing';
import { MatDialog } from '@angular/material/dialog';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import { type SceneImage, SceneImageSchema } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { MapsClient } from '../../../core/maps/maps-client';
import { FakeMapsClient } from '../../../core/maps/maps-testing';
import { SceneImages } from './scene-images';
import { nextTurn } from '../../../../test-settle';

function image(id: string, name: string): SceneImage {
  return create(SceneImageSchema, { id, name });
}

const TWO = [image('i1', 'A carroça'), image('i2', 'A estrada')];

describe('SceneImages (the point panel)', () => {
  let api: FakeMapsClient;
  let fixture: ComponentFixture<SceneImages>;
  let el: HTMLElement;
  let emitted: (readonly SceneImage[])[];
  let pickerData: { excluded?: Set<string>; submit: (i: unknown) => Promise<void> } | null;

  function setup(images: SceneImage[]) {
    api = new FakeMapsClient();
    api.imageNames = new Map([
      ['i1', 'A carroça'],
      ['i2', 'A estrada'],
      ['i3', 'O poço'],
    ]);
    emitted = [];
    pickerData = null;
    TestBed.configureTestingModule({
      providers: [
        { provide: MapsClient, useValue: api },
        {
          provide: MatDialog,
          useValue: {
            open: (_c: unknown, config: { data: typeof pickerData }) => {
              pickerData = config.data;
              return { afterClosed: () => ({ subscribe: () => undefined }) };
            },
          },
        },
      ],
    });
    fixture = TestBed.createComponent(SceneImages);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('mapId', 'm1');
    fixture.componentRef.setInput('pointId', 'p1');
    fixture.componentRef.setInput('name', 'A carroça tombada');
    fixture.componentRef.setInput('images', images);
    fixture.componentInstance.imagesChange.subscribe((list) => {
      emitted.push(list);
      fixture.componentRef.setInput('images', list);
    });
    fixture.detectChanges();
    el = fixture.nativeElement;
  }

  async function settle(): Promise<void> {
    for (let i = 0; i < 4; i++) {
      await fixture.whenStable();
      await nextTurn();
      fixture.detectChanges();
    }
  }

  const rows = () => Array.from(el.querySelectorAll('.si__row'));
  const control = (id: string, kind: string) =>
    el.querySelector<HTMLButtonElement>(`[data-image="${id}"][data-control="${kind}"]`)!;

  it("says it is the master's list, and what is empty", () => {
    setup([]);
    expect(el.querySelector('h3')?.textContent).toBe('Imagens da cena');
    expect(el.querySelector('.si__privacy')?.textContent).toContain(
      'só veem uma imagem quando você a mostra',
    );
    expect(el.querySelector('.si__empty')?.textContent).toContain('Nenhuma imagem ainda');
    expect(el.querySelector('.si__count')?.textContent?.replace(/\u00a0/g, ' ')).toContain(
      '0 de 8',
    );
  });

  it('lists the images in order with thumbnails, and the edges are quiet', () => {
    setup(TWO);
    expect(
      rows().map((r) => r.querySelector('.si__name')?.textContent?.replace(/\s+/g, ' ').trim()),
    ).toEqual(['Imagem 1: A carroça', 'Imagem 2: A estrada']);
    expect(rows()[0].querySelector('img')?.getAttribute('src')).toBe('/images/i1/thumb');
    expect(control('i1', 'up').getAttribute('aria-disabled')).toBe('true');
    expect(control('i2', 'down').getAttribute('aria-disabled')).toBe('true');
    expect(control('i1', 'down').getAttribute('aria-disabled')).toBe('false');
  });

  it('moves an image down with one SetSceneImages call of the whole list', async () => {
    setup(TWO);
    control('i1', 'down').click();
    await settle();
    expect(api.calls).toContain('setSceneImages p1 i2,i1');
    expect(emitted.at(-1)?.map((i) => i.id)).toEqual(['i2', 'i1']);
    expect(el.querySelector('[role="status"]')?.textContent).toContain('desceu para a posição 2');
  });

  it('takes an image off the scene at once, without asking, and says it stays in the gallery', async () => {
    setup(TWO);
    control('i1', 'remove').click();
    await settle();
    expect(api.calls).toContain('setSceneImages p1 i2');
    expect(rows().length).toBe(1);
    expect(el.querySelector('[role="status"]')?.textContent).toContain('Continua na galeria');
  });

  it('the picker offers only images not on the scene, and a pick is appended', async () => {
    setup(TWO);
    el.querySelector<HTMLButtonElement>('[data-control="pick"]')!.click();
    expect(pickerData?.excluded).toEqual(new Set(['i1', 'i2']));
    await pickerData!.submit({ id: 'i3', name: 'O poço' });
    await settle();
    expect(api.calls).toContain('setSceneImages p1 i1,i2,i3');
    expect(rows().length).toBe(3);
  });

  it('a generated picture joins the list; one already there does not repeat', async () => {
    setup(TWO);
    await fixture.componentInstance['generated']({ generated: 1, lastImageId: 'i3', map: null });
    await settle();
    expect(api.calls).toContain('setSceneImages p1 i1,i2,i3');
    await fixture.componentInstance['generated']({ generated: 1, lastImageId: 'i3', map: null });
    expect(api.calls.filter((c) => c.startsWith('setSceneImages')).length).toBe(1);
  });

  it('at 8 images the gallery button is off with the limit in words, and a generated picture says it only went to the gallery', async () => {
    const eight = Array.from({ length: 8 }, (_, n) => image(`x${n}`, `Imagem ${n}`));
    setup(eight);
    expect(el.querySelector('#si-limit')?.textContent).toContain('Limite de 8 imagens');
    expect(
      el.querySelector<HTMLButtonElement>('[data-control="pick"]')?.getAttribute('aria-disabled'),
    ).toBe('true');
    await fixture.componentInstance['generated']({ generated: 1, lastImageId: 'new', map: null });
    fixture.detectChanges();
    expect(el.querySelector('[role="alert"]')?.textContent).toContain('foi para a galeria');
    expect(api.calls.some((c) => c.startsWith('setSceneImages'))).toBe(false);
  });

  it('a refused write says so and leaves the list', async () => {
    setup(TWO);
    api.failWith = new ConnectError('x', Code.PermissionDenied);
    control('i1', 'down').click();
    await settle();
    expect(el.querySelector('[role="alert"]')?.textContent).toContain('Só o mestre');
    expect(rows().length).toBe(2);
  });
});
