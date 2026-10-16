import { TestBed } from '@angular/core/testing';

import { MapState } from '../../../../core/maps/map-state';
import { mapMessage, mapPoint, mapResponse } from '../../../../core/maps/maps-testing';
import { SceneClient } from '../../../../core/play/scene-client';
import { SceneState } from '../../../../core/play/scene-state';
import { FakeSceneClient, masterScene, sceneRoll } from '../../../../core/play/scene-testing';
import { LiveSessionSource } from '../../../../core/play/live-session.types';
import { SceneOpen } from './scene-open';

const flat = (e: Element | null | undefined) =>
  e?.textContent
    ?.replace(/\u00a0/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();

describe('SceneOpen', () => {
  const toren = sceneRoll('r1', 'a2', 'Toren', 7, { passed: false });
  const brisa = sceneRoll('r2', 'a4', 'Brisa', 17, {
    roll: { diceCount: 1, diceSides: 20, faces: [14], modifier: 3, total: 17, physical: true },
  });
  const pens = sceneRoll('r3', 'a1', 'Pensantus', 17, {
    passed: true,
    roll: { diceCount: 1, diceSides: 20, faces: [11], modifier: 6, total: 17 },
  });

  async function setup(scene = masterScene([toren, brisa, pens])) {
    const api = new FakeSceneClient();
    const state = new SceneState(
      () => api.get(),
      () => true,
    );
    state.apply(scene);
    const mapState = new MapState(() =>
      Promise.resolve(
        mapResponse(mapMessage('m1', 'Estrada do Vale'), [mapPoint('p1', 'A carroça tombada')]),
      ),
    );
    await mapState.open('m1');
    TestBed.configureTestingModule({
      providers: [
        { provide: SceneClient, useValue: api },
        { provide: LiveSessionSource, useValue: { setShownImage: async () => null } },
      ],
    });
    const fixture = TestBed.createComponent(SceneOpen);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('state', state);
    fixture.componentRef.setInput('mapState', mapState);
    fixture.detectChanges();
    return { fixture, api, state, el: fixture.nativeElement as HTMLElement };
  }

  const lines = (el: HTMLElement) => Array.from(el.querySelectorAll('app-scene-roll-line'));

  it('shows the scene\'s images with "Mostrar aos jogadores", and nothing when it has none', async () => {
    const none = await setup();
    expect(none.el.querySelector('app-scene-images-show section')).toBeNull();
    TestBed.resetTestingModule();
    const { el } = await setup(
      masterScene([], [], {
        images: [
          { id: 'i1', name: 'A carroça', showsWholeMap: false },
          { id: 'i2', name: 'A estrada', showsWholeMap: false },
        ],
      }),
    );
    expect(flat(el.querySelector('app-scene-images-show h3'))).toBe('Imagens da cena');
    expect(el.querySelectorAll('app-scene-images-show .sm__tile').length).toBe(2);
  });

  it('shows the title, when it opened, where it is and how much has happened', async () => {
    const { el } = await setup();
    expect(el.querySelector('h2')?.textContent).toBe('Cena: A\u00a0carroça tombada');
    expect(flat(el.querySelector('.so__since'))).toContain('Aberta para os jogadores desde 21:10');
    expect(flat(el.querySelector('.so__meta'))).toBe('Mapa Estrada do Vale · 5 ações · 3 rolagens');
  });

  it('lists the rolls newest first, each with its formula', async () => {
    const { el } = await setup();
    expect(lines(el).map((l) => flat(l.querySelector('.rl__who')))).toEqual([
      'Toren',
      'Brisa',
      'Pensantus',
    ]);
    expect(lines(el).map((l) => flat(l.querySelector('.rl__formula')))).toEqual([
      '1d20 (6) + 1 = 7',
      '14 + 3 = 17',
      '1d20 (11) + 6 = 17',
    ]);
    expect(flat(lines(el)[0].querySelector('.rl__time'))).toBe('21:14');
  });

  it('says passed or not with the DC, on its own line under the formula, and nothing without a DC', async () => {
    const { el } = await setup();
    const [failed, noDc, passed] = lines(el);
    expect(flat(failed.querySelector('.mr-tag'))).toContain('Não passou · CD 13');
    expect(failed.querySelector('.mr-tag--danger mat-icon')?.textContent).toBe('close');
    expect(flat(passed.querySelector('.mr-tag'))).toContain('Passou · CD 12');
    expect(passed.querySelector('.mr-tag--success mat-icon')?.textContent).toBe('check');
    expect(noDc.querySelector('.mr-tag')).toBeNull();
    // The pill follows the formula in the DOM, in the same column.
    const body = failed.querySelector('.rl__body')!;
    expect(Array.from(body.children).map((c) => c.className.split(' ')[0])).toEqual([
      'rl__top',
      'rl__action',
      'rl__formula',
      'rl__line',
    ]);
    // A typed die is named on the action line, not in the pill slot.
    expect(flat(noDc.querySelector('.rl__action'))).toBe('Percepção · dado físico');
  });

  it('lists the actions with their DCs, which only the master sees', async () => {
    const { el } = await setup();
    const items = Array.from(el.querySelectorAll('.so__action'));
    expect(items).toHaveLength(5);
    expect(items.map((i) => flat(i.querySelector('.mr-tag')) ?? null)).toEqual([
      'CD 12',
      'CD 13',
      null,
      null,
      'CD 10',
    ]);
    expect(Array.from(items[3].children, (c) => flat(c))).toEqual([
      'Percepção',
      'Perícia',
      '3 tentativas por jogador',
    ]);
    expect(flat(el.querySelector('.so__hint'))).toBe('Chegam aqui na hora, a mais nova em cima');
  });

  it('says there is no roll yet', async () => {
    const { el } = await setup(masterScene());
    expect(el.textContent).toContain('Ninguém rolou ainda.');
    expect(el.querySelector('.so__meta')?.textContent).toContain('nenhuma rolagem');
  });

  it('closes the scene asking nothing, and hands focus to "Abrir cena"', async () => {
    const { fixture, api, state, el } = await setup();
    const close = Array.from(el.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Fechar cena'),
    )!;
    close.click();
    for (let i = 0; i < 3; i++) {
      await fixture.whenStable();
    }
    expect(api.calls).toEqual(['close']);
    expect(state.scene()).toBeNull();
    expect(state.focusNext()).toBe('open');
  });

  it('keeps both buttons outlined (the one filled button is not here)', async () => {
    const { el } = await setup();
    const buttons = Array.from(el.querySelectorAll<HTMLElement>('.so__buttons button'));
    expect(buttons.map((b) => flat(b))).toEqual(['swap_horizTrocar cena', 'closeFechar cena']);
    expect(buttons.every((b) => b.classList.contains('mat-mdc-outlined-button'))).toBe(true);
  });

  it('reads the new roll aloud in a polite region', async () => {
    const { fixture, el, state } = await setup(masterScene([toren]));
    state.apply(masterScene([pens, toren]));
    fixture.detectChanges();
    const region = el.querySelector('.so__live');
    expect(region?.getAttribute('aria-live')).toBe('polite');
    expect(region?.textContent).toBe('Pensantus: Procurar pistas na carroça, 17, passou');
    expect(lines(el)).toHaveLength(2);
  });

  describe('the options of the scene (E8-13)', () => {
    const words = (e: Element | null | undefined) => {
      if (!e) {
        return undefined;
      }
      const copy = e.cloneNode(true) as Element;
      copy.querySelectorAll('mat-icon').forEach((i) => i.remove());
      return flat(copy);
    };

    it('says at the top of "Ações" whether the players see the DC, with an eye or a struck eye', async () => {
      const off = await setup();
      expect(words(off.el.querySelector('.so__dcnote'))).toBe('Só você vê a CD');
      expect(off.el.querySelector('.so__dcnote mat-icon')?.textContent).toBe('visibility_off');
      TestBed.resetTestingModule();
      const on = await setup(masterScene([], [], { showDc: true }));
      expect(words(on.el.querySelector('.so__dcnote'))).toBe('Os jogadores veem a CD');
      expect(on.el.querySelector('.so__dcnote mat-icon')?.textContent).toBe('visibility');
    });

    it('says each action\'s limit: "1 tentativa por jogador", "3 tentativas por jogador", "Sem limite de tentativas"', async () => {
      const { el } = await setup();
      const items = Array.from(el.querySelectorAll('.so__action'));
      expect(
        items.map((i) => flat(i.querySelector('.so__acheck:last-child, .so__dcline .so__acheck'))),
      ).toEqual([
        '1 tentativa por jogador',
        '1 tentativa por jogador',
        'Sem limite de tentativas',
        '3 tentativas por jogador',
        '1 tentativa por jogador',
      ]);
    });

    it('offers "Dar mais uma tentativa" only on the cards of players with no attempts left', async () => {
      const out = sceneRoll('r1', 'a2', 'Toren', 7, { passed: false, attemptsLeft: 0 });
      const some = sceneRoll('r2', 'a4', 'Pensantus', 9, { attemptsLeft: 2 });
      const { el } = await setup(masterScene([out, some]));
      const grants = Array.from(el.querySelectorAll('.rl__grant'));
      expect(grants.map((g) => g.getAttribute('aria-label'))).toEqual([
        'Dar mais uma tentativa a Toren em Seguir os rastros dos goblins',
      ]);
      expect(flat(lines(el)[1].querySelector('.rl__attempt'))).toBe('Tentativa 1 de 3');
    });
  });
});
