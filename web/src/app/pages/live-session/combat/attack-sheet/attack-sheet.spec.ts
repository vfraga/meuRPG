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
      { provide: CombatClient, useValue: {} },
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
