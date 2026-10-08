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
const cap = combatant({ id: 'cap', label: 'Capitão Goblin' });

describe('AttackSheet: the idempotency key follows the request', () => {
  it('a typed damage after a lost app roll is a new request with a new key', async () => {
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
    state.apply(
      encounter({ mode: EncounterMode.THEATRE, currentCombatantId: 't', combatants: [toren, cap] }),
    );
    const pending = {
      id: 'p1',
      attackerId: 't',
      targetId: 'cap',
      attackKey: longsword.key,
      status: PendingDamageStatus.AWAITING_ROLL,
      diceCount: 1,
      diceSides: 8,
      bonus: 3,
      critical: false,
    } as never;
    const data: AttackSheetData = {
      campaignId: 'c',
      encounterId: 'enc',
      attackerId: 't',
      round: 1,
      attack: longsword,
      targets: [],
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
      state,
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
});
