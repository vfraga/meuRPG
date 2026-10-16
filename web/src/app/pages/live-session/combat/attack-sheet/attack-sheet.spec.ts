import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CombatantKind,
  CombatantSide,
  CoverDegree,
  CoverSource,
  CriticalDamageRule,
  EncounterMode,
  PendingDamageStatus,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { Attack } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { AttackSheet, type AttackSheetData } from './attack-sheet';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

const plain = (t: string | null | undefined) => (t ?? '').replace(/\s+/g, ' ').trim();
const longsword = {
  key: 'attack:longsword',
  name: 'Longsword',
  namePt: 'Espada longa',
  attackBonus: 6,
  saveDc: 0,
  rangeFt: 5,
  longRangeFt: 0,
  damage: '1d8 + 3',
  damageTypePt: 'cortante',
  kind: 0,
} as unknown as Attack;
const toren = combatant({
  id: 't',
  label: 'Toren',
  kind: CombatantKind.PLAYER,
  side: CombatantSide.PARTY,
  mine: true,
});
const brisa = combatant({
  id: 'b',
  label: 'Brisa',
  kind: CombatantKind.PLAYER,
  side: CombatantSide.PARTY,
});
const cap = combatant({ id: 'cap', label: 'Capitão Goblin' });

function setup(opts: {
  diceMode: DiceMode;
  pending?: Record<string, unknown>;
  targets?: unknown[];
  mode?: EncounterMode;
  api?: unknown;
}) {
  const state = new CombatState();
  state.apply(
    encounter({
      mode: opts.mode ?? EncounterMode.THEATRE,
      currentCombatantId: 't',
      combatants: [toren, brisa, cap],
    }),
  );
  const pending = opts.pending
    ? ({
        id: 'p1',
        attackerId: 't',
        targetId: 'cap',
        attackKey: longsword.key,
        status: PendingDamageStatus.AWAITING_ROLL,
        diceCount: 1,
        diceSides: 8,
        bonus: 3,
        critical: true,
        ...opts.pending,
      } as never)
    : undefined;
  const data: AttackSheetData = {
    campaignId: 'c',
    encounterId: 'enc',
    attackerId: 't',
    round: 1,
    attack: longsword,
    targets: (opts.targets ?? []) as never,
    diceMode: opts.diceMode,
    preference: DicePreference.APP,
    state,
    resume: pending ? { pending, targetLabel: 'Capitão Goblin' } : undefined,
  };
  TestBed.configureTestingModule({
    providers: [
      { provide: MAT_DIALOG_DATA, useValue: data },
      { provide: MatDialogRef, useValue: { close: () => undefined } },
      { provide: CombatClient, useValue: fakeCombatClient(opts.api ?? {}) },
    ],
  });
  const fixture = TestBed.createComponent(AttackSheet);
  fixture.detectChanges();
  return { fixture, el: fixture.nativeElement as HTMLElement };
}

describe('AttackSheet: the critical total while a physical roll is typed (RN-24)', () => {
  const critical = { criticalRule: CriticalDamageRule.MAX_PLUS_ROLL, criticalMax: 8 };

  it('says one instruction, names the fixed parts and shows the total the server will record (5 + 8 + 3 = 16)', () => {
    const { fixture, el } = setup({ diceMode: DiceMode.PHYSICAL, pending: critical });
    const text = plain(el.textContent);
    expect(text).toContain(
      'Acerto crítico: o máximo mais uma rolagem. O máximo dos dados (8) já vale sem rolar; role 1d8 uma vez.',
    );
    expect(text).toContain('Role 1d8 e digite só o que saiu, de 1 a 8. O app soma o resto.');
    expect(text).toContain('+ 8 do crítico + 3 de modificador');
    const field = el.querySelector('input') as HTMLInputElement;
    field.value = '5';
    field.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(plain(el.textContent)).toContain('5 + 11 = 16');
  });

  it('says the doubled dice under the SRD rule', () => {
    const { el } = setup({
      diceMode: DiceMode.PHYSICAL,
      pending: { diceCount: 2, criticalRule: CriticalDamageRule.DOUBLED_DICE, criticalMax: 0 },
    });
    expect(plain(el.textContent)).toContain(
      'Acerto crítico: role os dados duas vezes (2d8 no total).',
    );
    expect(plain(el.textContent)).not.toContain('do crítico');
  });

  it("says nothing of dice with the app's dice: the server rolls them, until the player chooses to type", () => {
    const { fixture, el } = setup({ diceMode: DiceMode.PLAYERS_CHOOSE, pending: critical });
    expect(plain(el.textContent)).not.toContain('o máximo mais uma rolagem');
    expect(plain(el.textContent)).toContain('Acerto crítico: o dano segue a regra da mesa.');
    [...el.querySelectorAll('button')]
      .find((b) => plain(b.textContent).includes('Digitar o resultado'))!
      .click();
    fixture.detectChanges();
    expect(plain(el.textContent)).toContain('o máximo mais uma rolagem');
  });
});

describe('AttackSheet: the critical with the extra dice of Crítico Brutal (PM-03b)', () => {
  const brutal = {
    diceCount: 2,
    diceSides: 12,
    bonus: 3,
    extraDiceCount: 1,
    extraDiceNamePt: 'Crítico Brutal',
    damageTypePt: 'cortante',
    criticalRule: CriticalDamageRule.DOUBLED_DICE,
    criticalMax: 0,
  };
  const type = (fixture: { detectChanges(): void }, el: HTMLElement, n: string) => {
    const field = el.querySelector('input') as HTMLInputElement;
    field.value = n;
    field.dispatchEvent(new Event('input'));
    fixture.detectChanges();
  };

  it("with the app's dice, says the whole sum before rolling and which part is the critical and which the feature", () => {
    const { el } = setup({ diceMode: DiceMode.PLAYERS_CHOOSE, pending: brutal });
    expect(plain(el.querySelector('.crit__cap')?.textContent)).toBe('Dano do crítico');
    expect(plain(el.querySelector('.crit__sum')?.textContent)).toBe('2d12 + 1d12 + 3');
    expect(plain(el.querySelector('.crit__text')?.textContent)).toBe(
      '2d12 do crítico (dados dobrados) e 1d12 do Crítico Brutal (nível 9), mais 3 de modificador, de cortante.',
    );
    expect(plain(el.textContent)).toContain('Rolar dano no app');
  });

  it('writes the level of the feature from its dice: 13 for two extra dice, 17 for three', () => {
    const two = setup({
      diceMode: DiceMode.PLAYERS_CHOOSE,
      pending: { ...brutal, extraDiceCount: 2 },
    });
    expect(plain(two.el.querySelector('.crit__sum')?.textContent)).toBe('2d12 + 2d12 + 3');
    expect(plain(two.el.querySelector('.crit__text')?.textContent)).toContain('(nível 13)');
  });

  it('with physical dice, says how many dice to roll, whose they are, and accepts 3 to 36', () => {
    const { fixture, el } = setup({ diceMode: DiceMode.PHYSICAL, pending: brutal });
    expect(plain(el.querySelector('label.type__label')?.textContent)).toBe(
      'Role 3d12 para a Espada longa (+3)',
    );
    expect(plain(el.querySelector('.type__hint')?.textContent)).toBe(
      '2d12 do crítico e 1d12 do Crítico Brutal. Role os três dados e digite a soma (3 a 36).',
    );
    expect(el.querySelector('.crit')).toBeNull();
    type(fixture, el, '37');
    expect(plain(el.querySelector('[role="alert"]')?.textContent)).toContain(
      'Digite um número de 3 a 36',
    );
    type(fixture, el, '22');
    expect(plain(el.querySelector('.type__formula')?.textContent)).toContain('22 (3d12) + 3 = 25');
    expect(plain(el.querySelector('.type__cap')?.textContent)).toBe('dado físico · de cortante');
    expect(plain(el.textContent)).toContain('Confirmar 25');
  });

  it('under "o máximo mais uma rolagem" the maximum enters alone and the dice to roll are the critical\'s and the feature\'s', () => {
    const { fixture, el } = setup({
      diceMode: DiceMode.PHYSICAL,
      pending: {
        ...brutal,
        diceCount: 1,
        criticalRule: CriticalDamageRule.MAX_PLUS_ROLL,
        criticalMax: 12,
      },
    });
    expect(plain(el.querySelector('label.type__label')?.textContent)).toBe(
      'Role 2d12 para a Espada longa (+3)',
    );
    expect(plain(el.querySelector('.type__hint')?.textContent)).toBe(
      'O máximo do crítico (12) já está contado. Role 1d12 do crítico e 1d12 do Crítico Brutal; digite a soma (2 a 24).',
    );
    type(fixture, el, '10');
    expect(plain(el.querySelector('.type__formula')?.textContent)).toContain(
      '12 (máximo) + 10 (2d12) + 3 = 25',
    );
    expect(plain(el.querySelector('.type__num')?.textContent)).toBe('25');
  });

  it('after the app rolls, splits the groups of the damage line and names the feature', async () => {
    const rolled = {
      id: 'p1',
      status: PendingDamageStatus.APPLIED,
      amount: 25,
      damageTypePt: 'cortante',
      targetId: 'cap',
      extraDiceCount: 1,
      extraDiceNamePt: 'Crítico Brutal',
      criticalMax: 0,
      roll: {
        diceCount: 3,
        diceSides: 12,
        faces: [7, 11, 4],
        modifier: 3,
        total: 25,
        physical: false,
      },
    };
    const api = {
      rollDamage: async () => ({
        encounter: encounter({ combatants: [toren, brisa, cap] }),
        pending: rolled,
      }),
    };
    const { fixture, el } = setup({ diceMode: DiceMode.PLAYERS_CHOOSE, pending: brutal, api });
    [...el.querySelectorAll('button')]
      .find((b) => plain(b.textContent).includes('Rolar dano no app'))!
      .click();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(plain(el.querySelector('.part__formula')?.textContent)).toBe(
      '2d12 (7, 11) + 1d12 Crítico Brutal (4) + 3 = 25 de dano cortante',
    );
  });
});

describe('AttackSheet: the target list without a map (RN-25)', () => {
  const targets = [
    {
      combatantId: 'b',
      label: 'Brisa',
      state: 0,
      cover: CoverDegree.NONE,
      coverSource: CoverSource.UNSPECIFIED,
      tooFar: false,
      untargetable: false,
    },
    {
      combatantId: 'cap',
      label: 'Capitão Goblin',
      state: 1,
      cover: CoverDegree.HALF,
      coverSource: CoverSource.MARK,
      tooFar: false,
      untargetable: false,
    },
    {
      combatantId: 'x',
      label: 'Goblin',
      state: 1,
      cover: CoverDegree.TOTAL,
      coverSource: CoverSource.MARK,
      tooFar: false,
      untargetable: true,
    },
  ];

  it('says the master judges the reach, shows no distance, tags the ally and reads total cover as a target that cannot be chosen', () => {
    const { el } = setup({ diceMode: DiceMode.APP, targets });
    const text = plain(el.textContent);
    expect(text).toContain('Escolha o alvo (quem você vê)');
    expect(text).toContain(
      'O mestre decide quem está ao alcance. Sem mapa, o app não mostra distância.',
    );
    expect(text).not.toContain('Longe demais:');
    expect(text).toContain('Aliada');
    expect(text).toContain('Meia cobertura (marcada pelo mestre)');
    const rows = [...el.querySelectorAll('.target')];
    const total = rows.find(
      (r) => plain(r.textContent).includes('Goblin') && !plain(r.textContent).includes('Capitão'),
    )!;
    expect(plain(total.textContent)).toContain('não pode ser alvo');
    expect((total.querySelector('input') as HTMLInputElement).disabled).toBe(true);
  });
});
