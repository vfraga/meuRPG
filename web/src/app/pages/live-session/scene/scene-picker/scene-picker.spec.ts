import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';
import { create } from '@bufbuild/protobuf';

import { MapPointKind, SceneActionSchema } from '../../../../../gen/meurpg/maps/v1/maps_pb';
import { SceneBlockedReason, SceneBlockedSchema } from '../../../../../gen/meurpg/play/v1/scene_pb';
import { mapPoint } from '../../../../core/maps/maps-testing';
import { SceneClient } from '../../../../core/play/scene-client';
import { SceneState } from '../../../../core/play/scene-state';
import { FakeSceneClient } from '../../../../core/play/scene-testing';
import { ScenePicker, type ScenePickerData } from './scene-picker';

const actions = (n: number) =>
  Array.from({ length: n }, (_, i) =>
    create(SceneActionSchema, { id: `a${i}`, key: 'skill:arcana', checkName: 'Arcanismo' }),
  );

describe('ScenePicker', () => {
  let api: FakeSceneClient;
  let close: ReturnType<typeof vi.fn>;
  let state: SceneState;

  beforeEach(() => {
    Element.prototype.scrollTo = vi.fn() as never;
  });

  function setup(
    points = [
      mapPoint('p1', 'A carroça tombada', { revealed: true, sceneActions: actions(5) }),
      mapPoint('p2', 'Posto da guarda', { revealed: false, sceneActions: actions(3) }),
      mapPoint('p3', 'Vau do riacho', { revealed: true }),
      mapPoint('b1', 'Emboscada', { kind: MapPointKind.BATTLE, sceneActions: [] }),
    ],
    openPointId: string | null = null,
  ) {
    api = new FakeSceneClient();
    close = vi.fn();
    state = new SceneState(
      () => api.get(),
      () => true,
    );
    const data: ScenePickerData = {
      campaignId: 'c1',
      mapName: 'Estrada do Vale',
      points: signal(points),
      openPointId,
      state,
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: SceneClient, useValue: api },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close } },
      ],
    });
    const fixture = TestBed.createComponent(ScenePicker);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const settle = async () => {
      for (let i = 0; i < 3; i++) {
        await fixture.whenStable();
        fixture.detectChanges();
      }
    };
    const button = (name: string) =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
        b.textContent?.includes(name),
      )!;
    return { fixture, el, settle, button };
  }

  it('lists the scene points of the map, with where they are and how many actions', () => {
    const { el } = setup();
    const rows = Array.from(el.querySelectorAll('.pk__row'));
    expect(rows).toHaveLength(3); // the battle point is not a scene
    const text = (i: number) =>
      rows[i].textContent
        ?.replace(/\u00a0/g, ' ')
        .replace(/\s+/g, ' ')
        .trim();
    expect(text(0)).toContain('A carroça tombada');
    expect(text(0)).toContain('Revelado no mapa · 5 ações');
    expect(text(1)).toContain('Escondido no mapa · 3 ações');
    expect(el.textContent).toContain('Estrada do Vale');
  });

  it('offers a point with no actions like any other, with no reason to refuse it (question 63)', () => {
    const { el } = setup();
    const third = el.querySelectorAll<HTMLInputElement>('input[type="radio"]')[2];
    expect(third.disabled).toBe(false);
    expect(el.textContent).not.toContain('Sem ações');
    expect(el.textContent).not.toContain('Adicione no editor do mapa');
    // The first one is marked.
    expect(el.querySelectorAll<HTMLInputElement>('input[type="radio"]')[0].checked).toBe(true);
  });

  it('counts the clues of a scene in its line', () => {
    const { el } = setup([
      mapPoint('p9', 'Vau do riacho', {
        revealed: true,
        clues: [{ id: 'c1' } as never, { id: 'c2' } as never],
      }),
    ]);
    expect(el.querySelector('.pk__row')?.textContent?.replace(/\s+/g, ' ')).toContain(
      'Revelado no mapa · 2 pistas',
    );
  });

  it('opens a hidden point, and says it stays hidden on the map', async () => {
    const { el, fixture, settle, button } = setup();
    expect(el.textContent).toContain('Abrir uma cena escondida não revela o ponto no mapa.');
    el.querySelectorAll<HTMLInputElement>('input[type="radio"]')[1].click();
    fixture.detectChanges();
    button('Abrir cena').click();
    await settle();
    expect(api.calls).toContain('open p2');
    expect(close).toHaveBeenCalledWith(true);
    expect(state.scene()?.name).toBe('A carroça tombada');
    expect(state.focusNext()).toBe('title');
  });

  it('keeps "Abrir cena" off only when the map has no scene at all', () => {
    const { el, button } = setup([mapPoint('b2', 'Emboscada', { kind: MapPointKind.BATTLE })]);
    expect(
      button('Abrir cena').disabled ||
        button('Abrir cena').getAttribute('aria-disabled') === 'true',
    ).toBe(true);
    expect(el.textContent).toContain('Este mapa não tem cena de RP');
  });

  it('marks the scene that is open when swapping', () => {
    const { el } = setup(undefined, 'p2');
    expect(el.querySelectorAll<HTMLInputElement>('input[type="radio"]')[1].checked).toBe(true);
  });

  it('shows why the server refused, from the typed reason, and stays open', async () => {
    const { el, settle, button } = setup();
    api.failWith = new ConnectError('x', Code.FailedPrecondition, undefined, [
      {
        desc: SceneBlockedSchema,
        value: create(SceneBlockedSchema, { reason: SceneBlockedReason.NO_ACTIONS }),
      },
    ]);
    button('Abrir cena').click();
    await settle();
    expect(el.querySelector('[role="alert"]')?.textContent).toContain('não tem ações');
    expect(close).not.toHaveBeenCalled();
  });

  it('cancels without opening anything', () => {
    const { button } = setup();
    button('Cancelar').click();
    expect(close).toHaveBeenCalledWith(false);
    expect(api.calls).toEqual([]);
  });
});
