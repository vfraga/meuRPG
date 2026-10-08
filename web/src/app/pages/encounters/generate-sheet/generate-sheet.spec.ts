import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';

import { EncounterBuildBlockedReason } from '../../../../gen/meurpg/play/v1/encounters_pb';
import { flat, isOff } from '../../../core/creatures/creatures-testing';
import { EncountersClient } from '../../../core/encounters/encounters-client';
import {
  BANDIT,
  BUGBEAR,
  FakeEncountersClient,
  GOBLIN,
  HOBGOBLIN,
  MIRATHEL,
  OGRE,
  buildBlocked,
  evaluation,
  line,
} from '../../../core/encounters/encounters-testing';
import { GenerateSheet, type GenerateData } from './generate-sheet';

describe('GenerateSheet: "Gerar encontro" (MR-043, E10-09 states 4 and 5)', () => {
  let api: FakeEncountersClient;
  let close: ReturnType<typeof vi.fn>;

  async function setup(prep: (api: FakeEncountersClient) => void = () => undefined) {
    api = new FakeEncountersClient();
    // The artboard's encounter (state 4): 450 + 2 × 100 + 4 × 100 + 10 × 25 = 1.300 XP.
    const CAPTAIN = {
      ...OGRE,
      key: 'monster:bandit-captain',
      namePt: 'Capitão bandido',
      name: 'Bandit captain',
      challengeRating: '2',
      xp: 450,
    };
    const SCOUT = { ...HOBGOBLIN, key: 'monster:scout', namePt: 'Batedor', name: 'Scout' };
    const THUG = { ...HOBGOBLIN, key: 'monster:thug', namePt: 'Capanga', name: 'Thug' };
    api.table.set(CAPTAIN.key, CAPTAIN as never);
    api.table.set(SCOUT.key, SCOUT as never);
    api.table.set(THUG.key, THUG as never);
    api.table.set('monster:orc', {
      ...GOBLIN,
      key: 'monster:orc',
      namePt: 'Orc',
      challengeRating: '1/2',
      xp: 100,
    } as never);
    api.generated = {
      lines: [
        [CAPTAIN as never, 1],
        [SCOUT as never, 2],
        [THUG as never, 4],
        [BANDIT, 10],
      ],
      seed: 7731,
    };
    api.swapList = [
      BANDIT,
      {
        ...GOBLIN,
        key: 'monster:orc',
        namePt: 'Orc',
        name: 'Orc',
        challengeRating: '1/2',
        xp: 100,
      } as never,
      {
        ...GOBLIN,
        key: 'monster:gnoll',
        namePt: 'Gnoll',
        name: 'Gnoll',
        challengeRating: '1/2',
        xp: 100,
      } as never,
    ];
    prep(api);
    close = vi.fn();
    const data: GenerateData = {
      campaignId: 'camp-1',
      campaignName: 'Mirathel',
      party: [],
      evaluation: evaluation([line(OGRE, 1)], { party: MIRATHEL }),
    };
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: EncountersClient, useValue: api },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close } },
      ],
    });
    const fixture = TestBed.createComponent(GenerateSheet);
    const settle = async () => {
      for (let i = 0; i < 4; i++) {
        fixture.detectChanges();
        await fixture.whenStable();
      }
    };
    await settle();
    const el = fixture.nativeElement as HTMLElement;
    const button = (name: string) =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
        (flat(b) ?? '').includes(name),
      )!;
    const radio = (label: string) =>
      Array.from(el.querySelectorAll<HTMLLabelElement>('.seg__item'))
        .find((l) => flat(l)?.includes(label))!
        .querySelector('input')!;
    return { el, settle, button, radio };
  }

  it('opens already generated: Moderada, any type, the lead with the party by level, and the encounter with its seed', async () => {
    const { el } = await setup();
    expect(flat(el.querySelector('.frame__title'))).toBe('Gerar encontro');
    expect(flat(el.querySelector('.frame__sub'))).toBe(
      'Para o grupo de Mirathel: três de nível 4 e um de nível 5',
    );
    expect(flat(el.querySelector('.seg__item--on'))).toBe('Moderada');
    expect(flat(el.querySelector('.hint'))).toBe('Moderada: até 1.875 XP para o seu grupo.');
    expect(el.querySelector<HTMLSelectElement>('select[name=type]')!.value).toBe('');
    expect(api.generateCalls).toEqual([{ band: 'moderate', type: '', seed: 0, party: [] }]);
    expect(flat(el.querySelector('.res__seed'))).toBe('Semente 7731');
    expect(Array.from(el.querySelectorAll('.line__pt')).map((l) => flat(l))).toEqual([
      '1 × Capitão bandido',
      '2 × Batedor',
      '4 × Capanga',
      '10 × Bandido',
    ]);
    expect(flat(el.querySelector('.head'))).toBe('Moderada · 1.300 de 1.875 XP');
    expect(flat(el.querySelector('.cap'))).toContain('A criatura mais forte pode ter ND 7');
    expect(flat(el.querySelector('.guide'))).toBe(
      'Guia de dificuldade do SRD 5.2.1 (regras de 2024) · Créditos',
    );
    expect(flat(el.querySelector('.guide--c'))).toBe(
      'Com os monstros de 2014, o encontro tende a ficar um pouco mais fácil.',
    );
  });

  it('"Gerar outro" asks for another with seed 0, and the seed shown is the new one', async () => {
    const { el, button, settle } = await setup((a) => (a.seeds = [7731, 2209]));
    button('Gerar outro').click();
    await settle();
    expect(api.generateCalls.map((c) => c.seed)).toEqual([0, 0]);
    expect(flat(el.querySelector('.res__seed'))).toBe('Semente 2209');
  });

  it('a new difficulty or type asks again with them', async () => {
    const { el, radio, settle } = await setup();
    radio('Alta').click();
    await settle();
    const select = el.querySelector<HTMLSelectElement>('select[name=type]')!;
    select.value = 'humanoid';
    select.dispatchEvent(new Event('change'));
    await settle();
    expect(api.generateCalls.map((c) => [c.band, c.type])).toEqual([
      ['moderate', ''],
      ['high', ''],
      ['high', 'humanoid'],
    ]);
  });

  it('"Trocar criatura" lists the same XP, leaves out what the encounter already has, and the swap keeps the count', async () => {
    const { el, settle } = await setup();
    // The Thugs (4 × 100 XP): the list is the server's, the encounter's own creatures are hidden.
    Array.from(el.querySelectorAll<HTMLButtonElement>('.line__swap'))[2].click();
    await settle();
    expect(api.swapCalls).toEqual([{ key: 'monster:thug', type: '' }]);
    expect(flat(el.querySelector('.swap__t'))).toBe('Trocar 4 × Capanga');
    expect(Array.from(el.querySelectorAll('.swap__n')).map((n) => flat(n))).toEqual([
      'Orc',
      'Gnoll',
    ]);
    expect(flat(el.querySelector('.swap__s'))).toBe(
      'Só aparecem criaturas com o mesmo XP (100 cada), então o total não muda. A quantidade fica.',
    );
    expect(isOff(el.querySelector<HTMLButtonElement>('.swap .go')!)).toBe(true);
    el.querySelector<HTMLInputElement>('.swap__opt input')!.click();
    await settle();
    expect(flat(el.querySelector('.swap .go'))).toBe('Trocar por Orc');
    el.querySelector<HTMLButtonElement>('.swap .go')!.click();
    await settle();
    // The server measures the swapped encounter: 4 of the Orc, not 1.
    expect(api.evaluateCalls.at(-1)?.entries.map((e) => [e.creatureKey, e.count])).toEqual([
      ['monster:bandit-captain', 1],
      ['monster:scout', 2],
      ['monster:orc', 4],
      ['monster:bandit', 10],
    ]);
    expect(el.querySelector('.swap')).toBeNull();
    expect(flat(el.querySelector('.res__seed'))).toBe('Trocado à mão');
  });

  it('"Cancelar" leaves the encounter as it was', async () => {
    const { el, settle } = await setup();
    Array.from(el.querySelectorAll<HTMLButtonElement>('.line__swap'))[0].click();
    await settle();
    el.querySelector<HTMLButtonElement>('.swap .no')!.click();
    await settle();
    expect(el.querySelector('.swap')).toBeNull();
    expect(api.evaluate).not.toHaveBeenCalled();
    expect(flat(el.querySelector('.res__seed'))).toBe('Semente 7731');
  });

  it('"Usar este encontro" closes with the lines and the seed', async () => {
    const { button, settle } = await setup();
    button('Usar este encontro').click();
    await settle();
    expect(close).toHaveBeenCalledTimes(1);
    const result = close.mock.calls[0][0];
    expect(result.seed).toBe(7731);
    expect(
      result.entries.map((e: { creature: { key: string }; count: number }) => [
        e.creature.key,
        e.count,
      ]),
    ).toEqual([
      ['monster:bandit-captain', 1],
      ['monster:scout', 2],
      ['monster:thug', 4],
      ['monster:bandit', 10],
    ]);
  });

  it('says in words when nothing fits or the party is empty, and leaves nothing to use', async () => {
    const nothing = await setup(
      (a) => (a.generateFail = buildBlocked(EncounterBuildBlockedReason.NOTHING_FITS)),
    );
    expect(nothing.el.querySelector('[role=alert]')?.textContent).toContain(
      'Nenhuma criatura desse tipo cabe nessa dificuldade',
    );
    expect(nothing.el.querySelector('.res')).toBeNull();
    expect(isOff(nothing.button('Usar este encontro'))).toBe(true);
    TestBed.resetTestingModule();
    const empty = await setup(
      (a) => (a.generateFail = buildBlocked(EncounterBuildBlockedReason.NO_PARTY)),
    );
    expect(empty.el.querySelector('[role=alert]')?.textContent).toContain('O grupo está vazio');
  });

  it('never says "mortal" and has Bugbear\'s band words only from the guide', async () => {
    const { el } = await setup();
    expect(el.textContent?.toLowerCase()).not.toContain('mortal');
    expect(BUGBEAR.xp).toBe(200);
  });

  it('keeps the encounter of a new draw when a swap of the old one answers last', async () => {
    const { el, settle, radio } = await setup();
    let release!: () => void;
    api.gates = [new Promise<void>((r) => (release = r))];
    Array.from(el.querySelectorAll<HTMLButtonElement>('.line__swap'))[2].click();
    await settle();
    el.querySelector<HTMLInputElement>('.swap__opt input')!.click();
    await settle();
    el.querySelector<HTMLButtonElement>('.swap .go')!.click();
    await settle();
    expect(api.evaluateCalls.length).toBe(1);

    // Meanwhile the difficulty changes: a new encounter arrives.
    api.generated = { lines: [[OGRE, 6]], seed: 222 };
    radio('Alta').click();
    await settle();
    expect(flat(el.querySelector('.res__seed'))).toBe('Semente 222');

    release();
    await settle();
    expect(flat(el.querySelector('.res__seed'))).toBe('Semente 222');
    expect(Array.from(el.querySelectorAll('.line__pt')).map((l) => flat(l))).toEqual(['6 × Ogro']);
    expect(flat(el.querySelector('.seg__item--on'))).toBe('Alta');
  });
});
