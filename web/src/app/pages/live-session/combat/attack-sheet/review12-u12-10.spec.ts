// Finding U12-10: idempotency keys survive a change of intent (damageKey constant per sheet;
// ShieldSheet shares one key between use() and decline()).
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { ConnectError, Code } from '@connectrpc/connect';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
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
import { ShieldSheet } from '../shield-sheet/shield-sheet';

const longsword = {
  key: 'attack:longsword', name: 'Longsword', namePt: 'Espada longa', attackBonus: 6, saveDc: 0,
  rangeFt: 5, longRangeFt: 0, damage: '1d8 + 3', damageTypePt: 'cortante', kind: 0,
} as unknown as Attack;
const toren = combatant({ id: 't', label: 'Toren', kind: CombatantKind.PLAYER, side: CombatantSide.PARTY, mine: true });
const cap = combatant({ id: 'cap', label: 'Capitão Goblin' });

describe('Review12 U12-10: idempotency key kept across a change of intent', () => {
  it('AttackSheet: typed damage after a failed app roll must not reuse the app roll key', async () => {
    const keys: string[] = [];
    const api = {
      rollDamage: (_c: string, _e: string, _p: string, die: { sum?: number }, key: string) => {
        keys.push(key);
        return die.sum === undefined
          ? Promise.reject(new ConnectError('timeout', Code.Unavailable))
          : Promise.reject(new ConnectError('stop', Code.Aborted));
      },
    };
    const state = new CombatState();
    state.apply(encounter({ mode: EncounterMode.THEATRE, currentCombatantId: 't', combatants: [toren, cap] }));
    const pending = {
      id: 'p1', attackerId: 't', targetId: 'cap', attackKey: longsword.key,
      status: PendingDamageStatus.AWAITING_ROLL, diceCount: 1, diceSides: 8, bonus: 3, critical: false,
    } as never;
    const data: AttackSheetData = {
      campaignId: 'c', encounterId: 'enc', attackerId: 't', round: 1, attack: longsword, targets: [],
      diceMode: DiceMode.PLAYERS_CHOOSE, preference: DicePreference.APP, state,
      resume: { pending, targetLabel: 'Capitão Goblin' },
    } as AttackSheetData;
    TestBed.configureTestingModule({
      providers: [
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: () => undefined } },
        { provide: CombatClient, useValue: api },
      ],
    });
    const sheet = TestBed.createComponent(AttackSheet).componentInstance as unknown as {
      rollDamage(d: object): Promise<void>;
      rollDamageTyped(n: number): Promise<void>;
    };
    await sheet.rollDamage({}); // "Rolar no app": the answer is lost
    await sheet.rollDamageTyped(9); // then "Digitar o resultado"
    expect(keys.length).toBe(2);
    expect(keys[1]).not.toBe(keys[0]);
  });

  it('ShieldSheet: "Conjurar" after a failed "Não usar" must not reuse the decline key', async () => {
    const keys: Record<string, string> = {};
    const api = {
      declineReaction: (_c: string, _e: string, _p: string, key: string) => {
        keys['decline'] = key;
        return Promise.reject(new ConnectError('timeout', Code.Unavailable));
      },
      useReaction: (_c: string, _e: string, _p: string, _s: unknown, key: string) => {
        keys['use'] = key;
        return Promise.reject(new ConnectError('stop', Code.Aborted));
      },
    };
    const state = new CombatState();
    state.apply(encounter({ currentCombatantId: 't', combatants: [toren, cap] }));
    const data = {
      campaignId: 'c', encounterId: 'enc', round: 1, state, armorClass: 15, pact: null,
      prompt: { pendingDamageId: 'p1', spellNamePt: 'Escudo Arcano', slots: [{ level: 1, pact: false, free: 2 }] },
      usage: [{ level: 1, total: 2, used: 0 }],
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: () => undefined } },
        { provide: CombatClient, useValue: api },
      ],
    });
    const sheet = TestBed.createComponent(ShieldSheet).componentInstance as unknown as {
      use(): Promise<void>;
      decline(): Promise<void>;
    };
    await sheet.decline();
    await sheet.use();
    expect(keys['use']).toBeDefined();
    expect(keys['use']).not.toBe(keys['decline']);
  });
});
