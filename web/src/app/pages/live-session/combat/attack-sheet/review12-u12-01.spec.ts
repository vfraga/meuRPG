// Review 12, finding U12-01: a hit on a player character who can cast Shield
// comes back AWAITING_REACTION; the sheet must say the damage waits for the reaction.
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  AttackOutcome,
  CombatantKind,
  CombatantSide,
  EncounterMode,
  PendingDamageStatus,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { Attack } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { AttackSheet, type AttackSheetData } from './attack-sheet';

const plain = (t: string | null | undefined) =>
  (t ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ').trim();
const sword = {
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

describe('Review12 U12-01: hit awaiting the target reaction (Shield)', () => {
  it('does not say "Sem dano: o ataque errou." and says the damage waits for the reaction', async () => {
    const enc = encounter({
      mode: EncounterMode.THEATRE,
      currentCombatantId: 't',
      combatants: [
        combatant({ id: 't', label: 'Toren', kind: CombatantKind.PLAYER, side: CombatantSide.PARTY, mine: true }),
        combatant({ id: 'b', label: 'Brisa', kind: CombatantKind.PLAYER, side: CombatantSide.PARTY }),
      ],
    });
    const state = new CombatState();
    state.apply(enc);
    const api = {
      rollAttack: async () => ({
        encounter: enc,
        roll: { outcome: AttackOutcome.HIT, d20: { total: 17, modifier: 6, diceCount: 1, diceSides: 20, faces: [11], physical: false } },
        pending: {
          id: 'p1',
          attackerId: 't',
          targetId: 'b',
          attackKey: sword.key,
          status: PendingDamageStatus.AWAITING_REACTION,
        },
      }),
    };
    const data: AttackSheetData = {
      campaignId: 'c',
      encounterId: 'enc',
      attackerId: 't',
      round: 1,
      attack: sword,
      targets: [{ combatantId: 'b', label: 'Brisa', tooFar: false }] as never,
      diceMode: DiceMode.APP,
      preference: DicePreference.APP,
      state,
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: () => undefined } },
        { provide: CombatClient, useValue: api },
      ],
    });
    const fixture = TestBed.createComponent(AttackSheet);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    (el.querySelector('input[type=radio]') as HTMLInputElement).dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Enter' }),
    );
    fixture.detectChanges();
    const btn = [...el.querySelectorAll('button')].find((b) =>
      plain(b.textContent).includes('Rolar no app'),
    )!;
    btn.click();
    await fixture.whenStable();
    fixture.detectChanges();
    const text = plain(el.textContent);
    expect(text).toContain('Acertou');
    expect(text).not.toContain('Sem dano: o ataque errou.');
    expect(text.toLowerCase()).toContain('reação');
  });
});
