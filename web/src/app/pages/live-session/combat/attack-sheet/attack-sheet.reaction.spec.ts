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
import { nextTurn } from '../../../../../test-settle';

const plain = (t: string | null | undefined) => (t ?? '').replace(/\s+/g, ' ').trim();
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

describe('AttackSheet: a hit that waits for the reaction of its target (Escudo)', () => {
  it('says the damage waits for the reaction, not that the attack missed', async () => {
    const enc = encounter({
      mode: EncounterMode.THEATRE,
      currentCombatantId: 't',
      combatants: [
        combatant({
          id: 't',
          label: 'Toren',
          kind: CombatantKind.PLAYER,
          side: CombatantSide.PARTY,
          mine: true,
        }),
        combatant({
          id: 'b',
          label: 'Brisa',
          kind: CombatantKind.PLAYER,
          side: CombatantSide.PARTY,
        }),
      ],
    });
    const state = new CombatState();
    state.apply(enc);
    const api = {
      rollAttack: async () => ({
        encounter: enc,
        roll: {
          outcome: AttackOutcome.HIT,
          d20: {
            total: 17,
            modifier: 6,
            diceCount: 1,
            diceSides: 20,
            faces: [11],
            physical: false,
          },
        },
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
    expect(text.toLowerCase()).toContain('esperando o mestre');
  });
});

describe("AttackSheet: a roll a reaction holds, and the monk's throw back (PM-04)", () => {
  function mount(extra: Partial<AttackSheetData>, roll: object) {
    const toren = combatant({
      id: 't',
      label: 'Toren',
      kind: CombatantKind.PLAYER,
      side: CombatantSide.PARTY,
      mine: true,
    });
    const enc = encounter({
      mode: EncounterMode.THEATRE,
      currentCombatantId: 't',
      combatants: [toren, combatant({ id: 'g', label: 'Goblin 2' })],
      reactionWait: {
        titlePt: 'Esperando o mestre',
        detailPt: 'O resultado do seu ataque sai quando ele responder.',
      },
    } as never);
    const state = new CombatState();
    state.apply(enc);
    const rollAttack = vi.fn().mockResolvedValue({ encounter: enc, roll, pending: undefined });
    const data: AttackSheetData = {
      campaignId: 'c',
      encounterId: 'enc',
      attackerId: 't',
      round: 1,
      attack: sword,
      targets: [{ combatantId: 'g', label: 'Goblin 2', tooFar: false }] as never,
      diceMode: DiceMode.APP,
      preference: DicePreference.APP,
      state,
      ...extra,
    };
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: () => undefined } },
        { provide: CombatClient, useValue: { rollAttack } },
      ],
    });
    const fixture = TestBed.createComponent(AttackSheet);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement, rollAttack };
  }

  async function rollIt(m: ReturnType<typeof mount>) {
    (m.el.querySelector('input[type=radio]') as HTMLInputElement).dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Enter' }),
    );
    m.fixture.detectChanges();
    [...m.el.querySelectorAll('button')]
      .find((b) => plain(b.textContent).includes('Rolar no app'))!
      .click();
    await m.fixture.whenStable();
    m.fixture.detectChanges();
  }

  const d20 = { total: 17, modifier: 6, diceCount: 1, diceSides: 20, faces: [11], physical: false };

  it('shows the wait the server wrote instead of an outcome', async () => {
    const m = mount({}, { outcome: AttackOutcome.UNSPECIFIED, heldForReaction: true, d20 });
    await rollIt(m);
    const text = plain(m.el.textContent);
    expect(text).toContain(
      'Esperando o mestre. O resultado do seu ataque sai quando ele responder.',
    );
    expect(text).not.toContain('Acertou');
    expect(text).not.toContain('Errou');
    expect(text).not.toContain('Sem dano');
    expect(m.el.querySelector('.pill')).toBeNull();
  });

  it('still shows the outcome of a roll nothing holds', async () => {
    const m = mount({}, { outcome: AttackOutcome.HIT, heldForReaction: false, d20 });
    await rollIt(m);
    expect(plain(m.el.textContent)).toContain('Acertou');
  });

  it('names the window that caught the missile, as part of the same reaction', async () => {
    const m = mount({ asReaction: true, catchWindowId: 'w2' }, { outcome: AttackOutcome.HIT, d20 });
    await rollIt(m);
    const args = m.rollAttack.mock.calls[0];
    expect(args[7]).toBe(true);
    expect(args[10]).toBe('w2');
  });
});

describe('AttackSheet: the hit that waited for Escudo follows the server (R2-07)', () => {
  async function mountWaiting(options: () => object) {
    const enc = encounter({
      mode: EncounterMode.THEATRE,
      currentCombatantId: 't',
      combatants: [
        combatant({
          id: 't',
          label: 'Bruna',
          kind: CombatantKind.PLAYER,
          side: CombatantSide.PARTY,
          mine: true,
        }),
        combatant({ id: 'g', label: 'Orc de teste' }),
      ],
    });
    const state = new CombatState();
    state.apply(enc);
    const pending = {
      id: 'p1',
      attackerId: 't',
      targetId: 'g',
      attackKey: sword.key,
      status: PendingDamageStatus.AWAITING_REACTION,
    };
    const api = {
      rollAttack: async () => ({
        encounter: enc,
        roll: {
          outcome: AttackOutcome.HIT,
          d20: {
            total: 17,
            modifier: 6,
            diceCount: 1,
            diceSides: 20,
            faces: [11],
            physical: false,
          },
        },
        pending,
      }),
      turnOptions: async () => options(),
    };
    const data: AttackSheetData = {
      campaignId: 'c',
      encounterId: 'enc',
      attackerId: 't',
      round: 1,
      attack: sword,
      targets: [{ combatantId: 'g', label: 'Orc de teste', tooFar: false }] as never,
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
    [...el.querySelectorAll('button')]
      .find((b) => plain(b.textContent).includes('Rolar no app'))!
      .click();
    await fixture.whenStable();
    fixture.detectChanges();
    return { fixture, el, state, enc };
  }

  it('turns into the miss when the target used Escudo (the pending damage is gone)', async () => {
    let answered = false;
    const { fixture, el, state, enc } = await mountWaiting(() => ({
      pendingDamages: answered ? [] : [{ id: 'p1', status: PendingDamageStatus.AWAITING_REACTION }],
    }));
    expect(plain(el.textContent)).toContain('Acertou');
    expect(plain(el.textContent)).toContain('Esperando o mestre.');
    answered = true;
    state.apply(encounter({ ...enc, revision: 2 } as never));
    fixture.detectChanges();
    await nextTurn();
    fixture.detectChanges();
    const text = plain(el.textContent);
    expect(text).toContain('Errou');
    expect(text).not.toContain('Acertou');
    expect(text).toContain('Sem dano: o ataque errou.');
  });

  it('goes on to the damage when the reaction let the hit through', async () => {
    let answered = false;
    const { fixture, el, state, enc } = await mountWaiting(() => ({
      pendingDamages: [
        {
          id: 'p1',
          attackerId: 't',
          targetId: 'g',
          attackKey: sword.key,
          status: answered
            ? PendingDamageStatus.AWAITING_ROLL
            : PendingDamageStatus.AWAITING_REACTION,
          diceCount: 1,
          diceSides: 8,
          bonus: 3,
        },
      ],
    }));
    answered = true;
    state.apply(encounter({ ...enc, revision: 2 } as never));
    fixture.detectChanges();
    await nextTurn();
    fixture.detectChanges();
    const text = plain(el.textContent);
    expect(text).toContain('Acertou');
    expect(text.toLowerCase()).not.toContain('esperando o mestre');
    expect(el.querySelector('app-roll-picker, app-multi-roll')).not.toBeNull();
  });
});
