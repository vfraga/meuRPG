import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  AttackOutcome,
  CombatantKind,
  CombatantSide,
  CoverDegree,
  CoverSource,
  PendingDamageStatus,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { RollMode, RollModeRequestStatus } from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import type { Attack } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { textOf } from '../../../../core/combat/contest-testing';
import { AttackSheet, type AttackSheetData } from './attack-sheet';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

const plain = (t: string | null | undefined) => (t ?? '').replace(/\s+/g, ' ').trim();
const shortsword = {
  key: 'attack:shortsword',
  name: 'Shortsword',
  namePt: 'Espada curta',
  attackBonus: 5,
  saveDc: 0,
  rangeFt: 5,
  longRangeFt: 0,
  damage: '1d6 + 3',
  damageTypePt: 'perfurante',
  kind: 0,
} as unknown as Attack;
const toren = combatant({
  id: 't',
  label: 'Toren',
  kind: CombatantKind.PLAYER,
  side: CombatantSide.PARTY,
  mine: true,
});
const cap = combatant({ id: 'cap', label: 'Capitão Goblin' });

function target(extra: Record<string, unknown> = {}) {
  return {
    combatantId: 'cap',
    label: 'Capitão Goblin',
    state: 1,
    cover: CoverDegree.NONE,
    coverSource: CoverSource.UNSPECIFIED,
    tooFar: false,
    untargetable: false,
    rollMode: RollMode.NORMAL,
    sources: [],
    criticalOnHit: false,
    ...extra,
  };
}

const request = (status: RollModeRequestStatus, extra: Record<string, unknown> = {}) =>
  ({
    id: 'req1',
    combatantId: 't',
    targetId: 'cap',
    attackKey: shortsword.key,
    attackNamePt: 'Espada curta',
    suggestedMode: RollMode.NORMAL,
    requestedMode: RollMode.ADVANTAGE,
    decidedMode: RollMode.UNSPECIFIED,
    reason: 'ele não me vê',
    status,
    ...extra,
  }) as never;

function setup(opts: { targets: unknown[]; diceMode?: DiceMode; master?: boolean; api?: object }) {
  const state = new CombatState();
  const enc = (requests: unknown[] = []) =>
    encounter({
      currentCombatantId: 't',
      combatants: [toren, cap],
      rollModeRequests: requests,
    } as never);
  state.apply(enc());
  const api = {
    rollAttack: vi.fn().mockResolvedValue({
      encounter: enc(),
      roll: {
        outcome: AttackOutcome.MISS,
        mode: RollMode.NORMAL,
        suggestedMode: RollMode.NORMAL,
        modeReason: '',
        sources: [],
        criticalOnHit: false,
        d20: { diceCount: 1, diceSides: 20, faces: [3], modifier: 5, total: 8, countedIndex: 0 },
      },
      pending: undefined,
    }),
    requestRollMode: vi.fn(),
    cancelRollModeRequest: vi.fn(),
    rollDamage: vi.fn(),
    ...opts.api,
  };
  const data: AttackSheetData = {
    campaignId: 'c',
    encounterId: 'enc',
    attackerId: 't',
    round: 1,
    attack: shortsword,
    targets: opts.targets as never,
    diceMode: opts.diceMode ?? DiceMode.PLAYERS_CHOOSE,
    preference: DicePreference.APP,
    state,
    master: opts.master,
  };
  TestBed.configureTestingModule({
    providers: [
      { provide: MAT_DIALOG_DATA, useValue: data },
      { provide: MatDialogRef, useValue: { close: () => undefined } },
      { provide: CombatClient, useValue: fakeCombatClient(api) },
    ],
  });
  const fixture = TestBed.createComponent(AttackSheet);
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const button = (name: string) =>
    Array.from(el.querySelectorAll('button')).find((b) => plain(b.textContent).includes(name));
  const chooseTarget = () => {
    el.querySelector<HTMLInputElement>('.target input')!.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Enter' }),
    );
    fixture.detectChanges();
  };
  const radio = (word: string) =>
    Array.from(el.querySelectorAll<HTMLLabelElement>('.radio')).find((l) =>
      l.textContent!.includes(word),
    )!;
  const pick = (word: string) => {
    radio(word).querySelector('input')!.click();
    fixture.detectChanges();
  };
  const write = (value: string) => {
    const input = el.querySelector<HTMLInputElement>('.reason__field')!;
    input.value = value;
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
  };
  return { fixture, el, api, state, enc, button, chooseTarget, radio, pick, write };
}

describe('AttackSheet: the attack of a hidden attacker (W7-X, board W7-Xc 8b)', () => {
  const unseen = {
    kind: 19,
    effect: RollMode.ADVANTAGE,
    textPt: 'Atacante que o alvo não vê: vantagem',
  };

  it('says the server chose advantage because the target does not see the attacker, and asks for the pair', () => {
    const s = setup({ targets: [target({ rollMode: RollMode.ADVANTAGE, sources: [unseen] })] });
    s.chooseTarget();
    expect(plain(s.el.textContent)).toContain('Atacante que o alvo não vê: vantagem');
    expect(s.radio('Vantagem').querySelector('input')!.checked).toBe(true);
    expect(s.button('Rolar 2d20 no app')).toBeTruthy();
  });

  it('shows the pair of the roll with the one that counts, and the source behind it, once rolled', async () => {
    const api = {
      rollAttack: vi.fn().mockResolvedValue({
        encounter: encounter({ currentCombatantId: 't', combatants: [toren, cap] } as never),
        roll: {
          outcome: AttackOutcome.MISS,
          mode: RollMode.ADVANTAGE,
          suggestedMode: RollMode.ADVANTAGE,
          modeReason: '',
          sources: [unseen],
          criticalOnHit: false,
          d20: {
            diceCount: 2,
            diceSides: 20,
            faces: [4, 17],
            modifier: 5,
            total: 22,
            countedIndex: 1,
          },
        },
        pending: undefined,
      }),
    };
    const s = setup({
      targets: [target({ rollMode: RollMode.ADVANTAGE, sources: [unseen] })],
      api,
    });
    s.chooseTarget();
    s.button('Rolar 2d20 no app')!.click();
    await s.fixture.whenStable();
    s.fixture.detectChanges();
    const text = textOf(s.el.querySelector('app-attack-result')!);
    expect(text).toContain('Vantagem');
    expect(text).toContain('17 vale');
    expect(text).toContain('4 descartado');
    expect(text).toContain('Atacante que o alvo não vê: vantagem');
  });
});

describe('AttackSheet: advantage and disadvantage', () => {
  it('shows the suggested mode with its sources and rolls the pair in the app with no change to report', async () => {
    const s = setup({
      targets: [
        target({
          rollMode: RollMode.ADVANTAGE,
          sources: [{ effect: RollMode.ADVANTAGE, textPt: 'Alvo Derrubado a 1,5 m: vantagem' }],
        }),
      ],
    });
    s.chooseTarget();
    expect(plain(s.el.textContent)).toContain('Alvo Derrubado a 1,5 m: vantagem');
    expect(s.radio('Vantagem').querySelector('input')!.checked).toBe(true);
    expect(s.button('Rolar 2d20 no app')).toBeTruthy();
    s.button('Rolar 2d20 no app')!.click();
    await s.fixture.whenStable();
    const call = s.api.rollAttack.mock.calls[0];
    expect(call[5]).toEqual({ inApp: true });
    expect(call[9]).toBeUndefined();
  });

  it('lets the player pick disadvantage with a reason, and sends the mode and the reason', async () => {
    const s = setup({ targets: [target()] });
    s.chooseTarget();
    s.pick('Desvantagem');
    expect(s.button('Rolar')).toBeUndefined();
    s.write('estou cego pela poeira');
    s.button('Rolar 2d20 no app')!.click();
    await s.fixture.whenStable();
    expect(s.api.rollAttack.mock.calls[0][9]).toEqual({
      mode: RollMode.DISADVANTAGE,
      reason: 'estou cego pela poeira',
      requestId: undefined,
    });
  });

  it('asks for the two physical d20 in the order rolled and sends both', async () => {
    const s = setup({
      targets: [target({ rollMode: RollMode.DISADVANTAGE })],
      diceMode: DiceMode.PHYSICAL,
    });
    s.chooseTarget();
    const fields = s.el.querySelectorAll<HTMLInputElement>('app-multi-roll input');
    expect(fields).toHaveLength(2);
    fields[0].value = '14';
    fields[0].dispatchEvent(new Event('input'));
    fields[1].value = '7';
    fields[1].dispatchEvent(new Event('input'));
    s.fixture.detectChanges();
    s.el.querySelector('form')!.dispatchEvent(new Event('submit'));
    await s.fixture.whenStable();
    expect(s.api.rollAttack.mock.calls[0][5]).toEqual({ faces: [14, 7] });
  });

  it('sends a mode better than the suggestion to the master instead of rolling it', async () => {
    const asked = request(RollModeRequestStatus.PENDING);
    const s = setup({
      targets: [target()],
      api: {
        requestRollMode: vi.fn().mockResolvedValue({
          encounter: encounter({ rollModeRequests: [asked] } as never),
          request: asked,
        }),
      },
    });
    s.chooseTarget();
    s.pick('Vantagem');
    expect(s.button('Rolar')).toBeUndefined();
    expect(s.button('Pedir ao mestre')!.getAttribute('aria-disabled')).toBe('true');
    s.write('ele não me vê');
    s.button('Pedir ao mestre')!.click();
    await s.fixture.whenStable();
    s.fixture.detectChanges();
    expect(s.api.requestRollMode).toHaveBeenCalledWith(
      'c',
      'enc',
      't',
      shortsword.key,
      'cap',
      RollMode.ADVANTAGE,
      'ele não me vê',
      expect.any(String),
    );
    expect(s.el.querySelector('[aria-live="polite"]')!.textContent).toContain(
      'Aguardando o mestre…',
    );
    expect(s.button('Cancelar pedido')).toBeTruthy();
    expect(s.button('Rolar')).toBeUndefined();
  });

  it("rolls with the master's decision and the request id once the request is answered", async () => {
    const asked = request(RollModeRequestStatus.PENDING);
    const answered = request(RollModeRequestStatus.ANSWERED, { decidedMode: RollMode.ADVANTAGE });
    const s = setup({
      targets: [target()],
      api: {
        requestRollMode: vi.fn().mockResolvedValue({
          encounter: encounter({ rollModeRequests: [asked] } as never),
          request: asked,
        }),
      },
    });
    s.chooseTarget();
    s.pick('Vantagem');
    s.write('ele não me vê');
    s.button('Pedir ao mestre')!.click();
    await s.fixture.whenStable();
    s.state.apply(encounter({ rollModeRequests: [answered] } as never));
    s.fixture.detectChanges();
    expect(plain(s.el.textContent)).toContain('O mestre decidiu: Vantagem.');
    s.button('Rolar 2d20 no app')!.click();
    await s.fixture.whenStable();
    expect(s.api.rollAttack.mock.calls[0][9]).toEqual({
      mode: RollMode.ADVANTAGE,
      reason: 'ele não me vê',
      requestId: 'req1',
    });
  });

  it('takes the request back with "Cancelar pedido"', async () => {
    const asked = request(RollModeRequestStatus.PENDING);
    const s = setup({
      targets: [target()],
      api: {
        requestRollMode: vi.fn().mockResolvedValue({
          encounter: encounter({ rollModeRequests: [asked] } as never),
          request: asked,
        }),
        cancelRollModeRequest: vi.fn().mockResolvedValue(encounter({} as never)),
      },
    });
    s.chooseTarget();
    s.pick('Vantagem');
    s.write('ele não me vê');
    s.button('Pedir ao mestre')!.click();
    await s.fixture.whenStable();
    s.fixture.detectChanges();
    s.button('Cancelar pedido')!.click();
    await s.fixture.whenStable();
    s.fixture.detectChanges();
    expect(s.api.cancelRollModeRequest).toHaveBeenCalledWith(
      'c',
      'enc',
      'req1',
      expect.any(String),
    );
    expect(s.radio('Normal').querySelector('input')!.checked).toBe(true);
  });

  it('lets the master roll any mode with a reason and never asks', async () => {
    const s = setup({ targets: [target()], master: true });
    s.chooseTarget();
    s.pick('Vantagem');
    s.write('o goblin está distraído');
    expect(s.button('Pedir ao mestre')).toBeUndefined();
    s.button('Rolar 2d20 no app')!.click();
    await s.fixture.whenStable();
    expect(s.api.rollAttack.mock.calls[0][9]).toMatchObject({
      mode: RollMode.ADVANTAGE,
      reason: 'o goblin está distraído',
    });
  });

  it('notes the automatic critical hit', () => {
    const s = setup({ targets: [target({ criticalOnHit: true })] });
    s.chooseTarget();
    expect(plain(s.el.textContent)).toContain(
      'Acerto crítico automático (a 1,5 m de alvo paralisado ou inconsciente)',
    );
  });

  it('shows both d20 of the result, the counted one first marked in words', async () => {
    const roll = {
      outcome: AttackOutcome.HIT,
      mode: RollMode.ADVANTAGE,
      suggestedMode: RollMode.ADVANTAGE,
      modeReason: '',
      sources: [],
      criticalOnHit: false,
      d20: { diceCount: 2, diceSides: 20, faces: [7, 16], modifier: 5, total: 21, countedIndex: 1 },
    };
    const s = setup({
      targets: [target({ rollMode: RollMode.ADVANTAGE })],
      api: {
        rollAttack: vi
          .fn()
          .mockResolvedValue({ encounter: encounter({} as never), roll, pending: undefined }),
      },
    });
    s.chooseTarget();
    s.button('Rolar 2d20 no app')!.click();
    await s.fixture.whenStable();
    s.fixture.detectChanges();
    const text = plain(s.el.textContent);
    expect(text).toContain('16 + 5 = 21');
    expect(s.el.querySelector('.face--counted')!.textContent).toContain('16');
    expect(s.el.querySelector('.face--dropped')!.textContent).toContain('descartado');
  });
});

describe('AttackSheet: the damage parts', () => {
  const parts = [
    {
      key: 'weapon',
      labelPt: 'Espada curta',
      diceCount: 1,
      diceSides: 6,
      flat: 3,
      damageTypePt: 'perfurante',
    },
    {
      key: 'sneak-attack',
      labelPt: 'Ataque Furtivo',
      diceCount: 2,
      diceSides: 6,
      flat: 0,
      choosable: true,
      selected: true,
      available: true,
      reasonPt: 'Você tem vantagem.',
    },
    {
      key: 'hunters-mark',
      labelPt: 'Marca do Caçador',
      diceCount: 1,
      diceSides: 6,
      flat: 0,
      choosable: true,
      available: false,
      reasonPt: 'O alvo não está marcado.',
    },
  ];

  function withDamage(diceMode: DiceMode) {
    const state = new CombatState();
    state.apply(encounter({ currentCombatantId: 't', combatants: [toren, cap] } as never));
    const api = {
      rollDamage: vi
        .fn()
        .mockResolvedValue({ encounter: encounter({} as never), pending: { amount: 9 } }),
    };
    const data: AttackSheetData = {
      campaignId: 'c',
      encounterId: 'enc',
      attackerId: 't',
      round: 1,
      attack: shortsword,
      targets: [],
      diceMode,
      preference: DicePreference.APP,
      state,
      resume: {
        pending: {
          id: 'p1',
          attackerId: 't',
          targetId: 'cap',
          attackKey: shortsword.key,
          status: PendingDamageStatus.AWAITING_ROLL,
          diceCount: 1,
          diceSides: 6,
          bonus: 3,
          parts,
        } as never,
        targetLabel: 'Capitão Goblin',
      },
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: () => undefined } },
        { provide: CombatClient, useValue: fakeCombatClient(api) },
      ],
    });
    const fixture = TestBed.createComponent(AttackSheet);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement, api };
  }

  it('lists the weapon, the marked extra and the disabled one with its reason', () => {
    const { el } = withDamage(DiceMode.APP);
    const text = plain(el.textContent);
    expect(text).toContain('O que entra no dano');
    expect(text).toContain('Espada curta 1d6 + 3 de perfurante');
    expect(text).toContain('Indisponível: O alvo não está marcado.');
    const boxes = el.querySelectorAll<HTMLInputElement>('.extra__box');
    expect([...boxes].map((b) => [b.checked, b.disabled])).toEqual([
      [true, false],
      [false, true],
    ]);
  });

  it('answers with the extras marked, even none', async () => {
    const { fixture, el, api } = withDamage(DiceMode.APP);
    el.querySelector<HTMLInputElement>('.extra__box')!.click();
    fixture.detectChanges();
    [...el.querySelectorAll('button')]
      .find((b) => plain(b.textContent).includes('Rolar o dano no app'))!
      .click();
    await fixture.whenStable();
    expect(api.rollDamage.mock.calls[0][3]).toEqual({ inApp: true });
    expect(api.rollDamage.mock.calls[0][5]).toEqual({ extras: [] });
  });

  it('asks one sum for each group of dice for physical dice, the weapon included', async () => {
    const { fixture, el, api } = withDamage(DiceMode.PHYSICAL);
    const labels = [...el.querySelectorAll('app-multi-roll label')].map((l) =>
      plain(l.textContent),
    );
    expect(labels).toEqual(['Espada curta: 1d6', 'Ataque Furtivo: 2d6']);
    const fields = el.querySelectorAll<HTMLInputElement>('app-multi-roll input');
    fields[0].value = '4';
    fields[0].dispatchEvent(new Event('input'));
    fields[1].value = '8';
    fields[1].dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(plain(el.querySelector('app-multi-roll .type__sum')!.textContent)).toContain('15');
    el.querySelector('app-multi-roll form')!.dispatchEvent(new Event('submit'));
    await fixture.whenStable();
    expect(api.rollDamage.mock.calls[0][5]).toEqual({
      extras: [{ key: 'sneak-attack', slotLevel: 0, pact: false }],
      typedParts: [
        { partKey: 'weapon', sum: 4 },
        { partKey: 'sneak-attack', sum: 8 },
      ],
    });
  });
});
