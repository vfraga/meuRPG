import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { Code, ConnectError } from '@connectrpc/connect';
import { of } from 'rxjs';

import { CharacterKind } from '../../../../gen/meurpg/characters/v1/characters_pb';
import { type BestiaryAccess, BestiaryAccessCheck } from '../../../core/creatures/bestiary-access';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { FakeCreaturesClient, flat, isOff } from '../../../core/creatures/creatures-testing';
import {
  BUGBEAR,
  FakeEncountersClient,
  GOBLIN,
  HOBGOBLIN,
  OGRE,
  evaluation,
  line,
} from '../../../core/encounters/encounters-testing';
import { EncountersClient } from '../../../core/encounters/encounters-client';
import { RosterClient } from '../../../core/maps/roster-client';
import { EncounterBuilder } from './encounter-builder';

describe('EncounterBuilder (MR-043, RN-29, E10-09)', () => {
  let api: FakeEncountersClient;
  let creatures: FakeCreaturesClient;
  let access: BestiaryAccess;
  /** What each sheet closes with, by the id of its title: the dialogs are the sheets' own specs. */
  let results: Record<string, unknown>;
  let opened: { id: string; data: Record<string, unknown> }[];

  const artboard = () => ({
    entries: [OGRE, BUGBEAR, HOBGOBLIN, GOBLIN].map((creature, i) => ({
      creature,
      count: [1, 2, 4, 6][i],
    })),
    seed: null,
  });

  let api0: ((a: FakeEncountersClient) => void) | null = null;

  async function open(url = '/campaigns/camp-1/encounters') {
    api = new FakeEncountersClient();
    api0?.(api);
    api0 = null;
    creatures = new FakeCreaturesClient();
    creatures.catalog = [OGRE, BUGBEAR, HOBGOBLIN, GOBLIN];
    opened = [];
    TestBed.configureTestingModule({
      providers: [
        provideRouter([{ path: 'campaigns/:id/encounters', component: EncounterBuilder }]),
        { provide: EncountersClient, useValue: api },
        { provide: CreaturesClient, useValue: creatures },
        { provide: BestiaryAccessCheck, useValue: { check: async () => access } },
        {
          provide: RosterClient,
          useValue: {
            list: async () => [
              {
                id: 'npc-1',
                name: 'Orin, o guia',
                kind: CharacterKind.STORY,
                playerUserId: '',
                classSummary: '',
                raceName: '',
                playerName: null,
              },
              {
                id: 'pc-1',
                name: 'Pensantus',
                kind: CharacterKind.PLAYER,
                playerUserId: 'u',
                classSummary: '',
                raceName: '',
                playerName: null,
              },
            ],
          },
        },
        {
          provide: MatDialog,
          useValue: {
            open: (
              _c: unknown,
              config: { ariaLabelledBy: string; data: Record<string, unknown> },
            ) => {
              opened.push({ id: config.ariaLabelledBy, data: config.data });
              return { afterClosed: () => of(results[config.ariaLabelledBy]) };
            },
          },
        },
        { provide: MatBottomSheet, useValue: {} },
      ],
    });
    const harness = await RouterTestingHarness.create();
    await harness.navigateByUrl(url, EncounterBuilder);
    const settle = async (ms = 0) => {
      for (let i = 0; i < 4; i++) {
        harness.detectChanges();
        await harness.fixture.whenStable();
        await vi.advanceTimersByTimeAsync(ms);
      }
      harness.detectChanges();
    };
    await settle(300);
    const el = harness.routeNativeElement as HTMLElement;
    const button = (name: string) =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
        (flat(b) ?? '').includes(name),
      )!;
    const rows = () => Array.from(el.querySelectorAll('.row')).map((r) => flat(r));
    /** Fills the builder with the artboard's encounter through "Gerar encontro" (its sheet is the sheet's own spec). */
    const fill = async () => {
      results['gen-t'] = artboard();
      button('Gerar encontro').click();
      await settle(300);
    };
    return { el, settle, button, rows, fill };
  }

  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
    access = { status: 'master', campaignName: 'Mirathel' };
    results = {};
  });
  afterEach(() => vi.useRealTimers());

  it('opens on the party and the budget: four chips, the budgets of the bar, "Baixa" for an empty encounter, and the label and the credits', async () => {
    const { el } = await open();
    expect(flat(el.querySelector('h1'))).toBe('Encontros');
    expect(flat(el.querySelector('.mr-page-lead'))).toBe('Mirathel');
    expect(
      Array.from(el.querySelectorAll('.chip')).map((c) => flat(c.querySelector('.chip__t'))),
    ).toEqual(['Pensantus nível 4', 'Toren nível 4', 'Brisa nível 4', 'Sálvia nível 5']);
    expect(flat(el.querySelector('.head-xp'))).toBe('Baixa · 0 de 1.250 XP');
    expect(Array.from(el.querySelectorAll('.bar__budget')).map((b) => flat(b))).toEqual([
      '1.250',
      '1.875',
      '2.600',
    ]);
    expect(flat(el.querySelector('.guide'))).toBe(
      'Guia de dificuldade do SRD 5.2.1 (regras de 2024) · Créditos',
    );
    expect(el.querySelector('.guide a')?.getAttribute('href')).toBe('/credits');
    expect(flat(el.querySelector('.guide--c'))).toBe(
      'Com os monstros de 2014, o encontro tende a ficar um pouco mais fácil.',
    );
    expect(flat(el.querySelector('.cap'))).toBe(
      'A criatura mais forte pode ter ND 7: o menor nível do grupo (4) mais 3.',
    );
    expect(flat(el.querySelector('.lines .mr-muted'))).toContain('Nenhuma criatura ainda');
    // The first thing the page does is ask the server: the browser measures nothing.
    expect(api.evaluateCalls[0]).toEqual({ entries: [], party: [] });
  });

  it('measures the artboard\'s encounter: "Moderada · 1.550 de 1.875 XP", 13 criaturas, the rows with their XP', async () => {
    const { el, fill, rows } = await open();
    await fill();
    expect(flat(el.querySelector('.head-xp'))).toBe('Moderada · 1.550 de 1.875 XP');
    expect(flat(el.querySelector('.lines__n'))).toBe('13 criaturas');
    expect(flat(el.querySelector('.total__n'))).toBe('1.550 XP');
    expect(el.querySelector('.band--on')?.textContent).toContain('Moderada');
    expect(rows()).toEqual(
      [
        'Ogro Ogre · SRD ND 2 Tirar Ogro do encontro 1 Mais um Ogro 450 XP cada 450 XP',
        'Bugbear Bugbear · SRD ND 1 Menos um Bugbear 2 Mais um Bugbear 200 XP cada 400 XP',
        'Hobgoblin Hobgoblin · SRD ND 1/2 Menos um Hobgoblin 4 Mais um Hobgoblin 100 XP cada 400 XP',
        'Goblin Goblin · SRD ND 1/4 Menos um Goblin 6 Mais um Goblin 50 XP cada 300 XP',
      ].map((t) => expect.stringContaining(t.split(' ').slice(0, 2).join(' '))),
    );
    expect(api.evaluateCalls.at(-1)?.entries).toEqual([
      { creatureKey: 'monster:ogre', count: 1 },
      { creatureKey: 'monster:bugbear', count: 2 },
      { creatureKey: 'monster:hobgoblin', count: 4 },
      { creatureKey: 'monster:goblin', count: 6 },
    ]);
  });

  it('a run of taps on "+" asks the server once, after the pause, and the answer replaces the numbers', async () => {
    const { el, fill, settle } = await open();
    await fill();
    const before = api.evaluate.mock.calls.length;
    const more = () =>
      el.querySelector<HTMLButtonElement>('app-count-stepper button[aria-label="Mais um Ogro"]')!;
    more().click();
    more().click();
    more().click();
    await settle(0);
    expect(api.evaluate.mock.calls.length).toBe(before);
    await settle(300);
    expect(api.evaluate.mock.calls.length).toBe(before + 1);
    expect(api.evaluateCalls.at(-1)?.entries[0]).toEqual({ creatureKey: 'monster:ogre', count: 4 });
    expect(flat(el.querySelector('.head-xp'))).toBe('Acima de alta: 2.900 de 2.600 XP');
  });

  it('above high is allowed and warned, with the numbers of the server, and never says "mortal"', async () => {
    const { el, fill, settle } = await open();
    await fill();
    const more = el.querySelector<HTMLButtonElement>(
      'app-count-stepper button[aria-label="Mais um Ogro"]',
    )!;
    more.click();
    more.click();
    more.click();
    await settle(300);
    expect(flat(el.querySelector('.head-xp'))).toBe('Acima de alta: 2.900 de 2.600 XP');
    expect(flat(el.querySelector('.mr-notice--warning'))).toBe(
      'Passa do orçamento de alta. 2.900 XP contra 2.600 XP: o encontro fica bem acima do que o grupo aguenta. Dá para guardar e jogar assim.',
    );
    expect(el.querySelector('.band--on')?.textContent).toContain('Acima de alta');
    expect(el.textContent?.toLowerCase()).not.toContain('mortal');
    // It can still be kept.
    expect(
      isOff(
        Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find(
          (b) => flat(b) === 'Guardar no ponto de batalha',
        )!,
      ),
    ).toBe(false);
  });

  it('"−" at 1 takes the creature out of the encounter', async () => {
    const { el, fill, settle } = await open();
    await fill();
    el.querySelector<HTMLButtonElement>(
      'app-count-stepper button[aria-label="Tirar Ogro do encontro"]',
    )!.click();
    await settle(300);
    expect(el.querySelectorAll('.row')).toHaveLength(3);
    expect(flat(el.querySelector('.head-xp'))).toBe('Baixa · 1.100 de 1.250 XP');
  });

  it('adds a creature from the search by its name: the options show the type, the ND and CA and PV, and picking one adds one of it', async () => {
    const { el, settle, rows } = await open();
    const input = el.querySelector<HTMLInputElement>('input[role=combobox]')!;
    input.value = 'ogr';
    input.dispatchEvent(new Event('input'));
    await settle(300);
    const options = Array.from(el.querySelectorAll<HTMLElement>('[role=option]'));
    expect(options.map((o) => flat(o.querySelector('.pick__pt')))).toEqual(['Ogro']);
    options[0].click();
    await settle(300);
    expect(rows()).toHaveLength(1);
    expect(flat(el.querySelector('.head-xp'))).toBe('Baixa · 450 de 1.250 XP');
    expect(input.value).toBe('');
  });

  it('puts an NPC in the party with a level: the request carries it, the chip says "NPC · nível 3", and taking it out measures again', async () => {
    results['pn-t'] = { characterId: 'npc-1', name: '', level: 3, label: 'Orin, o guia' };
    const { el, button, settle } = await open();
    button('Pôr um NPC no grupo').click();
    await settle(300);
    expect(opened.find((o) => o.id === 'pn-t')?.data).toMatchObject({ campaignId: 'camp-1' });
    // Only the campaign's NPCs are offered, never a player's character.
    expect(
      (opened.find((o) => o.id === 'pn-t')?.data['npcs'] as { name: string }[]).map((n) => n.name),
    ).toEqual(['Orin, o guia']);
    expect(api.evaluateCalls.at(-1)?.party).toEqual([{ characterId: 'npc-1', name: '', level: 3 }]);
    const chips = Array.from(el.querySelectorAll('.chip')).map((c) =>
      flat(c.querySelector('.chip__t')),
    );
    expect(chips.at(-1)).toBe('Orin, o guia NPC · nível 3');
    expect(flat(el.querySelector('.cap'))).toBe(
      'A criatura mais forte pode ter ND 6: o menor nível do grupo (3) mais 3.',
    );
    el.querySelector<HTMLButtonElement>('.chip__x')!.click();
    await settle(300);
    expect(api.evaluateCalls.at(-1)?.party).toEqual([]);
    expect(el.querySelectorAll('.chip')).toHaveLength(4);
  });

  it('keeps an encounter on a battle point: "Guardar" says where, with the band and the creatures the server measured', async () => {
    const { el, fill, button, settle } = await open();
    await fill();
    results['save-t'] = {
      mapId: 'map-1',
      pointName: 'Emboscada na ponte',
      evaluation: evaluation([
        line(OGRE, 1),
        line(BUGBEAR, 2),
        line(HOBGOBLIN, 4),
        line(GOBLIN, 6),
      ]),
    };
    button('Guardar no ponto de batalha').click();
    await settle();
    const saved = opened.find((o) => o.id === 'save-t')!.data as {
      entries: unknown[];
      mapId: string;
    };
    expect(saved.entries).toHaveLength(4);
    expect(flat(el.querySelector('.mr-notice--success'))).toBe(
      'Encontro guardado em “Emboscada na ponte”. Moderada · 1.550 de 1.875 XP · 13 criaturas. Abrir o mapa',
    );
    expect(el.querySelector('.mr-notice--success a')?.getAttribute('href')).toBe(
      '/campaigns/camp-1/maps/map-1',
    );
  });

  it('opens "Guardar" on the point a link from the editor named', async () => {
    const { button, fill } = await open('/campaigns/camp-1/encounters?map=map-1&point=pt-1');
    await fill();
    button('Guardar no ponto de batalha').click();
    expect(opened.find((o) => o.id === 'save-t')?.data).toMatchObject({
      mapId: 'map-1',
      pointId: 'pt-1',
    });
  });

  it('with ?point= it brings the point\'s saved encounter in; "Tirar o encontro do ponto" asks in place and clears it', async () => {
    api0 = (a: FakeEncountersClient) => {
      a.battle.set('pt-1', {
        encounter: {
          monsters: [
            { creatureKey: GOBLIN.key, count: 2 },
            { creatureKey: 'monster:gone', count: 1 },
          ],
          hp: 'average',
          hidden: true,
        },
        evaluation: evaluation([line(GOBLIN, 2)]),
        unknownKeys: ['monster:gone'],
      });
    };
    const { el, button, settle } = await open('/campaigns/camp-1/encounters?map=map-1&point=pt-1');
    expect(flat(el.querySelector('.lines__n'))).toBe('2 criaturas');
    expect(api.getCalls).toEqual(['pt-1']);
    expect(flat(el.querySelector('.mr-notice--warning'))).toContain(
      'Uma criatura deste ponto não está mais no SRD.',
    );
    button('Tirar o encontro do ponto').click();
    await settle();
    expect(flat(el.querySelector('.act__ask'))).toContain('Tirar o encontro do ponto?');
    expect(document.activeElement?.textContent?.trim()).toBe('Voltar');
    button('Voltar').click();
    await settle();
    expect(api.clear).not.toHaveBeenCalled();
    button('Tirar o encontro do ponto').click();
    await settle();
    Array.from(el.querySelectorAll<HTMLButtonElement>('.act__ask button'))
      .find((b) => flat(b) === 'Tirar o encontro')!
      .click();
    await settle();
    expect(api.clear).toHaveBeenCalledWith('camp-1', 'pt-1');
    expect(flat(el.querySelector('.mr-notice--success'))).toContain(
      'O ponto não guarda mais um encontro.',
    );
    // The draft stays on screen, and the button is gone: the point keeps nothing.
    expect(flat(el.querySelector('.lines__n'))).toBe('2 criaturas');
    expect(el.textContent).not.toContain('Tirar o encontro do ponto');
  });

  it('"Guardar" waits for a creature, and says so', async () => {
    const { el, button } = await open();
    expect(isOff(button('Guardar no ponto de batalha'))).toBe(true);
    expect(flat(el.querySelector('.act__why'))).toBe('Ponha pelo menos uma criatura.');
    // The button stays focusable (disabledInteractive), so a click still reaches the handler: it does nothing.
    button('Guardar no ponto de batalha').click();
    expect(opened.filter((o) => o.id === 'save-t')).toHaveLength(0);
  });

  it('a failed measure says what to do, by code, and "Tentar de novo" asks again', async () => {
    const { el, settle } = await open();
    api.evaluateFail = new ConnectError('boom', Code.Unavailable);
    el.querySelector<HTMLButtonElement>('app-creature-pick')?.focus();
    results['gen-t'] = artboard();
    Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
      .find((b) => flat(b) === 'Gerar encontro')!
      .click();
    await settle(300);
    expect(el.querySelector('[role=alert]')?.textContent).toContain(
      'Não deu para medir o encontro: o servidor não respondeu. Tente de novo.',
    );
    api.evaluateFail = null;
    Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
      .find((b) => flat(b) === 'Tentar de novo')!
      .click();
    await settle(300);
    expect(el.querySelector('[role=alert]')).toBeNull();
    expect(flat(el.querySelector('.head-xp'))).toBe('Moderada · 1.550 de 1.875 XP');
  });

  it("is the master's alone: a player reads that it is, and the server is not asked", async () => {
    access = { status: 'forbidden' };
    const { el } = await open();
    expect(flat(el.querySelector('.mr-notice'))).toContain('Só o mestre monta encontros.');
    expect(api.evaluate).not.toHaveBeenCalled();
  });

  it('a campaign that is not there says so', async () => {
    access = { status: 'not-found' };
    const { el } = await open();
    expect(flat(el.querySelector('h1'))).toBe('Campanha não encontrada');
  });
});
