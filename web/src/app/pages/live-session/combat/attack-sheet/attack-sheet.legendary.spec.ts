import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CombatantKind, CombatantSide } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { Attack } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { AttackSheet, type AttackSheetData } from './attack-sheet';

const tail = {
  key: 'm#tail',
  name: 'Tail',
  namePt: 'Cauda',
  attackBonus: 14,
  saveDc: 0,
  rangeFt: 15,
  longRangeFt: 0,
  damage: '2d8 + 8',
  damageTypePt: 'concussão',
  kind: 0,
} as unknown as Attack;
const dragon = combatant({ id: 'd', label: 'Dragão vermelho adulto' });
const brisa = combatant({
  id: 'b',
  label: 'Brisa',
  kind: CombatantKind.PLAYER,
  side: CombatantSide.PARTY,
});

describe("AttackSheet: the master's legendary attack", () => {
  const rollAttack = vi.fn();

  function open(legendary?: AttackSheetData['legendary']) {
    rollAttack.mockReset().mockRejectedValue(new Error('stop'));
    const state = new CombatState();
    state.apply(encounter({ currentCombatantId: 't', combatants: [dragon, brisa] }));
    const data: AttackSheetData = {
      campaignId: 'camp',
      encounterId: 'enc',
      attackerId: 'd',
      round: 4,
      attack: tail,
      targets: [],
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
      state,
      master: true,
      ...(legendary ? { legendary } : {}),
    } as AttackSheetData;
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: () => undefined } },
        { provide: CombatClient, useValue: fakeCombatClient({ rollAttack }) },
      ],
    });
    const fixture = TestBed.createComponent(AttackSheet);
    fixture.detectChanges();
    return fixture;
  }
  const legendary = {
    option: 'tail',
    title: 'Cauda · ação lendária',
    subtitle: 'Dragão vermelho adulto · custa 1 de 2 restantes',
    cost: 1,
    label: 'Dragão vermelho adulto',
  };

  it('wears the option\'s title and what it costs instead of "Atacar com"', () => {
    const el = open(legendary).nativeElement as HTMLElement;
    expect(el.querySelector('.sheet__title')?.textContent?.trim()).toBe('Cauda · ação lendária');
    expect(el.querySelector('.sheet__sub')?.textContent?.trim()).toBe(
      'Dragão vermelho adulto · custa 1 de 2 restantes',
    );
  });

  it('keeps the plain attack title for an attack that is not legendary', () => {
    const el = open().nativeElement as HTMLElement;
    expect(el.querySelector('.sheet__title')?.textContent?.trim()).toBe('Atacar com Cauda');
  });

  it("rolls with the legendary option, so the server spends its cost and nothing of the creature's turn", async () => {
    const sheet = open(legendary).componentInstance as unknown as {
      s: { targetId: { set(id: string): void } };
      rollAttack(die: object): Promise<void>;
    };
    sheet.s.targetId.set('b');
    await sheet.rollAttack({ inApp: true });
    const effects = rollAttack.mock.calls[0][11];
    expect(effects).toMatchObject({ legendaryOption: 'tail' });
    expect(rollAttack.mock.calls[0][3]).toBe('m#tail');
  });

  it('sends no legendary option for a plain attack', async () => {
    const sheet = open().componentInstance as unknown as {
      s: { targetId: { set(id: string): void } };
      rollAttack(die: object): Promise<void>;
    };
    sheet.s.targetId.set('b');
    await sheet.rollAttack({ inApp: true });
    expect(rollAttack.mock.calls[0][11]).toMatchObject({ legendaryOption: '' });
  });

  it('says what the attack cost when it is done, not that the action was used', () => {
    const spent = (l: AttackSheetData['legendary']) =>
      (open(l).componentInstance as unknown as { spent(): string }).spent();
    expect(spent(legendary)).toBe(
      'Custou 1 ação lendária; a vez de Dragão vermelho adulto não foi gasta.',
    );
    expect(spent({ ...legendary, cost: 2 })).toBe(
      'Custou 2 ações lendárias; a vez de Dragão vermelho adulto não foi gasta.',
    );
    expect(spent(undefined)).toBe('Sua ação foi usada.');
  });

  it('shows that note on the done stage of the sheet', () => {
    const fixture = open(legendary);
    const sheet = fixture.componentInstance as unknown as {
      s: { stage: { set(v: unknown): void } };
    };
    sheet.s.stage.set('done');
    fixture.detectChanges();
    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';
    expect(text).toContain('Custou 1 ação lendária');
    expect(text).not.toContain('Sua ação foi usada');
  });
});
