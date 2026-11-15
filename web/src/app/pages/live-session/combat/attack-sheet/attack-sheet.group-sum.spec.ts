import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CombatantKind,
  CombatantSide,
  EncounterMode,
  PendingDamageStatus,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { Attack } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { AttackSheet, type AttackSheetData } from './attack-sheet';

const shortsword = {
  key: 'attack:shortsword',
  namePt: 'Espada curta',
  attackBonus: 5,
  saveDc: 0,
  rangeFt: 5,
  longRangeFt: 0,
  damage: '1d6 + 3',
  damageTypePt: 'perfurante',
  kind: 0,
} as unknown as Attack;

const part = (over: Record<string, unknown>) => ({
  key: 'weapon',
  labelPt: 'Espada curta',
  diceCount: 1,
  diceSides: 6,
  flat: 3,
  damageTypeKey: 'damage-type:piercing',
  damageTypePt: 'perfurante',
  choosable: false,
  selected: false,
  available: false,
  auto: false,
  ...over,
});
const waiting = {
  id: 'p1',
  attackerId: 'jab',
  targetId: 'zum',
  attackKey: 'attack:shortsword',
  status: PendingDamageStatus.AWAITING_GROUP_SUM,
  awaitedDamageTypeKey: 'damage-type:piercing',
  diceCount: 1,
  diceSides: 6,
  bonus: 3,
  hp: 22,
  parts: [
    part({}),
    part({
      key: 'sneak-attack',
      labelPt: 'Ataque Furtivo',
      diceCount: 3,
      flat: 0,
      choosable: true,
    }),
    part({
      key: 'frost',
      labelPt: 'Geada no Punho',
      diceCount: 2,
      diceSides: 8,
      flat: 0,
      auto: true,
      damageTypeKey: 'damage-type:cold',
      damageTypePt: 'frio',
    }),
  ],
  partRolls: [
    {
      partKey: 'frost',
      labelPt: 'Geada no Punho',
      diceCount: 2,
      diceSides: 8,
      faces: [3, 3],
      flat: 0,
      sum: 6,
      counted: true,
      damageTypeKey: 'damage-type:cold',
      rerolled: [],
      hitEffect: true,
    },
  ],
} as never;

describe('AttackSheet: the sum asked again after the master took a part out of a group (#835)', () => {
  const rollDamage = vi.fn();

  function open() {
    const state = new CombatState();
    state.apply(
      encounter({
        mode: EncounterMode.THEATRE,
        currentCombatantId: 'jab',
        combatants: [
          combatant({
            id: 'jab',
            label: 'Jabuti',
            kind: CombatantKind.PLAYER,
            side: CombatantSide.PARTY,
            mine: true,
          } as never),
          combatant({
            id: 'zum',
            label: 'Zumbi 1',
            hitPointsCurrent: 22,
            hitPointsMax: 22,
          } as never),
        ],
      }),
    );
    state.noteRemoval({
      attackerId: 'jab',
      targetId: 'zum',
      key: 'sneak-attack',
      name: 'Ataque Furtivo',
      reason: 'Sem aliado perto do alvo',
    });
    const data: AttackSheetData = {
      campaignId: 'c',
      encounterId: 'enc',
      attackerId: 'jab',
      round: 1,
      attack: shortsword,
      diceMode: DiceMode.APP,
      preference: DicePreference.APP,
      state,
      targets: [],
      resume: { pending: waiting, targetLabel: 'Zumbi 1' },
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: () => undefined } },
        { provide: CombatClient, useValue: fakeCombatClient({ rollDamage }) },
      ],
    });
    const fixture = TestBed.createComponent(AttackSheet);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    return { fixture, el, text: () => el.textContent!.replace(/\s+/g, ' ') };
  }

  it('says only what the master took out and why, the parts, the sums kept, and asks the one field', () => {
    const { el, text } = open();
    expect(text()).toContain('Digitar o dano de novo');
    expect(text()).toContain('Jabuti · Espada curta no Zumbi 1');
    expect(text()).toContain(
      'O mestre tirou o Ataque Furtivo: “Sem aliado perto do alvo”. Digite de novo só a soma de perfurante.',
    );
    expect(text()).toContain('Geada no Punho');
    expect(text()).toContain('já digitado, fica');
    expect(text()).not.toContain('22 PV');
    expect(text()).not.toContain('Acertou: role o dano.');
    expect(el.querySelectorAll('app-damage-parts')).toHaveLength(0);
  });

  it('writes the live total with the sum kept and sends the type alone', async () => {
    rollDamage.mockResolvedValue({
      encounter: encounter({}),
      pending: {
        ...(waiting as object),
        status: PendingDamageStatus.APPLIED,
        partRolls: [],
        amount: 13,
      },
      cast: [],
    });
    const { fixture, el, text } = open();
    const field = el.querySelector<HTMLInputElement>('input.type__field')!;
    expect(text()).toContain('Espada curta, perfurante: 1d6');
    expect(text()).toContain('De 1 a 6.');
    field.value = '4';
    field.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(text()).toContain('Dano: 4 + 3 + 6 de frio = 13');
    const apply = Array.from(el.querySelectorAll('button')).find((b) =>
      b.textContent!.includes('Aplicar 13 de dano'),
    )!;
    apply.click();
    await fixture.whenStable();
    expect(rollDamage).toHaveBeenCalledWith('c', 'enc', 'p1', { parts: true }, expect.any(String), {
      extras: [],
      typedParts: [{ damageType: 'damage-type:piercing', sum: 4 }],
    });
  });
});
