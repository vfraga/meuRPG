import { ComponentFixture, TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';

import { type SceneImage, SceneImageSchema } from '../../../../../gen/meurpg/maps/v1/maps_pb';
import { LiveSessionSource, type ShownImageVm } from '../../../../core/play/live-session.types';
import { SceneImagesShow } from './scene-images';
import { nextTurn } from '../../../../../test-settle';

function image(id: string, name: string, wholeMap = false): SceneImage {
  return create(SceneImageSchema, { id, name, showsWholeMap: wholeMap });
}

function vm(id: string, name: string): ShownImageVm {
  return { id, name, width: 10, height: 10, url: `/images/${id}` };
}

describe("SceneImagesShow (the master's open scene)", () => {
  let fixture: ComponentFixture<SceneImagesShow>;
  let el: HTMLElement;
  let source: { setShownImage: ReturnType<typeof vi.fn> };
  let changed: (ShownImageVm | null)[];
  let left: number;

  function setup(images: SceneImage[], shown: ShownImageVm | null = null, keep = false) {
    source = {
      setShownImage: vi.fn(async (_c: string, id: string | null) => (id ? vm(id, id) : null)),
    };
    changed = [];
    left = 0;
    TestBed.configureTestingModule({
      providers: [{ provide: LiveSessionSource, useValue: source }],
    });
    fixture = TestBed.createComponent(SceneImagesShow);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('images', images);
    fixture.componentRef.setInput('shown', shown);
    fixture.componentRef.setInput('keep', keep);
    fixture.componentInstance.changed.subscribe((s) => changed.push(s));
    fixture.componentInstance.leftChanged.subscribe(() => left++);
    fixture.detectChanges();
    el = fixture.nativeElement;
  }

  async function settle(): Promise<void> {
    for (let i = 0; i < 3; i++) {
      await fixture.whenStable();
      await nextTurn();
      fixture.detectChanges();
    }
  }

  const tile = (id: string) => el.querySelector(`[data-image="${id}"]`)!;
  const button = (id: string, text: string) =>
    Array.from(tile(id).querySelectorAll('button')).find((b) => b.textContent?.includes(text))!;

  it('draws nothing for a scene without images', () => {
    setup([]);
    expect(el.querySelector('section')).toBeNull();
  });

  it('lists the images, each with "Mostrar aos jogadores"', () => {
    setup([image('i1', 'A carroça'), image('i2', 'A estrada')]);
    expect(el.querySelector('h3')?.textContent).toBe('Imagens da cena');
    expect(el.querySelectorAll('.sm__tile').length).toBe(2);
    expect(button('i1', 'Mostrar aos jogadores').getAttribute('aria-label')).toBe(
      'Mostrar A carroça aos jogadores',
    );
  });

  it('shows through the gallery action and tells the page', async () => {
    setup([image('i1', 'A carroça')]);
    button('i1', 'Mostrar aos jogadores').click();
    await settle();
    expect(source.setShownImage).toHaveBeenCalledWith('c1', 'i1');
    expect(changed.map((s) => s?.id)).toEqual(['i1']);
    expect(el.querySelector('[role="status"]')?.textContent).toContain(
      'A carroça está na tela dos jogadores',
    );
  });

  it('marks the image on display and offers to stop it', async () => {
    setup([image('i1', 'A carroça'), image('i2', 'A estrada')], vm('i1', 'A carroça'));
    expect(tile('i1').textContent).toContain('À mostra agora');
    expect(tile('i1').classList).toContain('sm__tile--shown');
    expect(tile('i2').textContent).not.toContain('À mostra agora');
    button('i1', 'Parar de mostrar').click();
    await settle();
    expect(source.setShownImage).toHaveBeenCalledWith('c1', null);
    expect(changed).toEqual([null]);
  });

  it('with "Deixar com os jogadores" on, replacing the image tells the page to read the left list', async () => {
    setup([image('i2', 'A estrada')], vm('i1', 'A carroça'), true);
    button('i2', 'Mostrar aos jogadores').click();
    await settle();
    expect(left).toBe(1);
  });

  it('an image that shows the whole map asks first, in place', async () => {
    setup([image('m1', 'Mapa texturizado', true)]);
    button('m1', 'Mostrar aos jogadores').click();
    await settle();
    expect(source.setShownImage).not.toHaveBeenCalled();
    expect(tile('m1').textContent).toContain('mostra o mapa inteiro');
    button('m1', 'Voltar').click();
    await settle();
    expect(button('m1', 'Mostrar aos jogadores')).toBeTruthy();
    button('m1', 'Mostrar aos jogadores').click();
    await settle();
    button('m1', 'Mostrar mesmo assim').click();
    await settle();
    expect(source.setShownImage).toHaveBeenCalledWith('c1', 'm1');
  });

  it('a refusal is written out', async () => {
    setup([image('i1', 'A carroça')]);
    source.setShownImage.mockRejectedValueOnce(new Error('x'));
    button('i1', 'Mostrar aos jogadores').click();
    await settle();
    expect(el.querySelector('[role="alert"]')).not.toBeNull();
    expect(changed).toEqual([]);
  });
});
