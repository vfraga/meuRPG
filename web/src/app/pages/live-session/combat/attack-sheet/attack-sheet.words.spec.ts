import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  AttackOutcome,
  CombatantKind,
  CombatantSide,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { RollMode } from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import {
  type Attack,
  AttackKind,
  BonusAttackRule,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { AttackSheet, type AttackSheetData } from './attack-sheet';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

const plain = (t: string | null | undefined) =>
  (t ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

/** The words of the attack sheet around the roll: the line under the title, the rows of targets and what the attack spent. */
describe('AttackSheet: the words around the roll', () => {
  const bow = {
    key: 'attack:bow',
    name: 'Shortbow',
    namePt: 'Arco curto',
    attackBonus: -1,
    saveDc: 0,
    rangeFt: 80,
    longRangeFt: 320,
    damage: '1d6',
    damageTypePt: 'perfurante',
    kind: 0,
  } as unknown as Attack;
  const people = [
    combatant({
      id: 't',
      label: 'Toren',
      kind: CombatantKind.PLAYER,
      side: CombatantSide.PARTY,
      mine: true,
    }),
    combatant({
      id: 'ally',
      label: 'Brisa',
      kind: CombatantKind.PLAYER,
      side: CombatantSide.PARTY,
    }),
    combatant({
      id: 'near',
      label: 'Goblin 1',
    }),
    combatant({ id: 'far', label: 'Goblin 2' }),
  ];
  const reach = (id: string, label: string, distanceFt?: number, tooFar = false) => ({
    combatantId: id,
    label,
    state: 1,
    cover: 0,
    coverSource: 0,
    tooFar,
    untargetable: false,
    rollMode: RollMode.NORMAL,
    sources: [],
    criticalOnHit: false,
    distanceFt,
  });
  const targets = [
    reach('near', 'Goblin 1', 20),
    reach('far', 'Goblin 2', 200, true),
    reach('ally', 'Brisa', undefined),
  ];

  function open(over: Partial<AttackSheetData> = {}, outcome = AttackOutcome.MISS) {
    TestBed.resetTestingModule();
    const state = new CombatState();
    const enc = encounter({ currentCombatantId: 't', combatants: people } as never);
    state.apply(enc);
    const api = {
      rollAttack: vi.fn().mockResolvedValue({
        encounter: enc,
        roll: {
          outcome,
          d20: { diceCount: 1, diceSides: 20, faces: [3], modifier: 5, total: 8, countedIndex: 0 },
        },
        pending: undefined,
      }),
    };
    const data: AttackSheetData = {
      campaignId: 'c',
      encounterId: 'enc',
      attackerId: 't',
      round: 3,
      attack: bow,
      targets: targets as never,
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
      state,
      ...over,
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
    const finish = async () => {
      el.querySelector<HTMLInputElement>('.target input')?.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'Enter' }),
      );
      fixture.detectChanges();
      button('Rolar no app')!.click();
      await fixture.whenStable();
      fixture.detectChanges();
    };
    return { el, api, finish, text: () => plain(el.textContent) };
  }
  const subtitle = (el: HTMLElement) => plain(el.querySelector('.sheet__sub')?.textContent);
  const rows = (el: HTMLElement) =>
    Array.from(el.querySelectorAll('.target')).map((r) => plain(r.textContent));

  it('says "Ação" with the weapon and its range', () => {
    const { el } = open();
    expect(subtitle(el)).toBe('Ação · −1 para acertar · 1d6 perfurante · alcance 24 m');
  });

  it('says "Ação extra" when an effect gives the action', () => {
    expect(subtitle(open({ useExtraAction: true }).el)).toContain('Ação extra ·');
  });

  it('says "Reação" at 1,5 m for an opportunity attack, whatever range the weapon has', () => {
    const { el } = open({ asReaction: true });
    expect(subtitle(el)).toContain('Reação · ');
    expect(subtitle(el)).toContain('corpo a corpo');
  });

  it('says "Reação" with the weapon range of the monk throw back', () => {
    const { el } = open({ catchWindowId: 'w1' });
    expect(subtitle(el)).toContain('Reação · ');
    expect(subtitle(el)).toContain('alcance 24 m');
  });

  it('shows the range in the note under the targets, from the weapon, the throw or the reaction', () => {
    expect(open().text()).toContain('Um alvo fora do alcance de 24 m');
    expect(open({ catchWindowId: 'w1' }).text()).toContain('alcance de 96 m');
    expect(open({ asReaction: true }).text()).toContain('alcance de 1,5 m');
  });

  it('turns a target past the weapon range off, and tags an ally', () => {
    const list = rows(open().el);
    expect(list.find((r) => r.includes('Goblin 2'))).toContain('Longe demais');
    expect(list.find((r) => r.includes('Brisa'))).toContain('Aliada');
    expect(list.find((r) => r.includes('Goblin 1'))).not.toContain('Longe demais');
  });

  it('judges the throw back by its long range', () => {
    const list = rows(open({ catchWindowId: 'w1' }).el);
    expect(list.find((r) => r.includes('Goblin 2'))).not.toContain('Longe demais');
    const near = rows(
      open({ catchWindowId: 'w1', attack: { ...bow, longRangeFt: 100 } as Attack }).el,
    );
    expect(near.find((r) => r.includes('Goblin 2'))).toContain('Longe demais');
  });

  it('judges an opportunity attack by 5 ft, and a target with no distance is too far', () => {
    const list = rows(open({ asReaction: true }).el);
    expect(list.find((r) => r.includes('Goblin 1'))).toContain('Longe demais');
    expect(list.find((r) => r.includes('Brisa'))).toContain('Longe demais');
  });

  it('opens an opportunity attack at the roll, on the mover alone', () => {
    const { el } = open({
      opportunity: { offerId: 'o1', targetId: 'near', targetLabel: 'Goblin 1', byMaster: false },
    });
    expect(plain(el.querySelector('.chosen')?.textContent)).toContain('Goblin 1');
    expect(plain(el.querySelector('.chosen')?.textContent)).not.toContain('Trocar');
  });

  it('says what each attack spent once it is told', async () => {
    const spent = async (over: Partial<AttackSheetData>) => {
      const s = open(over);
      await s.finish();
      return plain(s.el.querySelector('.sheet__note')?.textContent);
    };
    expect(await spent({})).toContain('Sua ação foi usada.');
    expect(
      await spent({
        asReaction: true,
        opportunity: { offerId: 'o1', targetId: 'near', targetLabel: 'Goblin 1', byMaster: false },
      }),
    ).toContain('Sua reação foi usada.');
    expect(
      await spent({
        opportunity: { offerId: 'o1', targetId: 'near', targetLabel: 'Goblin 1', byMaster: true },
        asReaction: true,
      }),
    ).toContain('A reação dele foi usada.');
    expect(await spent({ attacksLeft: 3, attacksPerAction: 2 })).toContain(
      'Você ainda tem 2 ataques desta ação.',
    );
    expect(await spent({ attacksLeft: 2, attacksPerAction: 2 })).toContain(
      'Você ainda tem 1 ataque desta ação.',
    );
    expect(await spent({ attacksLeft: 1, attacksPerAction: 2 })).toContain('Sua ação foi usada.');
    expect(await spent({ bonusRule: BonusAttackRule.OFF_HAND })).toContain(
      'Sua ação bônus foi usada.',
    );
  });

  it('says how many beams of a cantrip remain, and spends no slot', async () => {
    const cantrip = {
      ...bow,
      key: 'spell:eldritch-blast',
      namePt: 'Rajada Mística',
      kind: AttackKind.SPELL,
    } as unknown as Attack;
    const s = open({ attack: cantrip, beamsLeft: 3 });
    await s.finish();
    const note = plain(s.el.querySelector('.sheet__note')?.textContent);
    expect(note).toContain('Rajada Mística: 2 raios restantes.');
  });
});
