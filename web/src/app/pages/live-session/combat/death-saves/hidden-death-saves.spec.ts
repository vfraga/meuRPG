import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { create } from '@bufbuild/protobuf';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CombatLogEntrySchema,
  CombatLogKind,
  CombatLogRoundSchema,
  CombatantKind,
  CombatantState,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { CombatLogState } from '../../../../core/combat/combat-log-state';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { CombatLogPanel } from '../combat-log/combat-log-panel';
import { OrderList } from '../order-list/order-list';
import { DeathSaves } from './death-saves';

// The death saves a table hides (RN-24, E10-04 state 6): the owner and the master read the marks and who else sees them; the other
// players read only the state in words, and the log says why.

const plain = (t: string | null | undefined) => (t ?? '').replace(/\s+/g, ' ').trim();
const brisaDown = combatant({
  id: 'b',
  label: 'Brisa',
  kind: CombatantKind.PLAYER,
  characterId: 'brisa-c',
  state: CombatantState.DOWN,
  deathSuccesses: 2,
  deathFailures: 1,
  hitPointsCurrent: 0,
  hitPointsMax: 27,
  mine: true,
  deathSaveDue: true,
});

describe("the owner's card of death saves", () => {
  function setup(ownerOnly: boolean) {
    const fixture = TestBed.createComponent(DeathSaves);
    fixture.componentRef.setInput('own', brisaDown);
    fixture.componentRef.setInput('diceMode', DiceMode.PLAYERS_CHOOSE);
    fixture.componentRef.setInput('dicePreference', DicePreference.APP);
    fixture.componentRef.setInput('ownerOnly', ownerOnly);
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }

  it('says "Só você e o mestre" under the title when the table hides them, with the counts', () => {
    const el = setup(true);
    expect(plain(el.querySelector('[data-testid="death-private"]')?.textContent)).toContain(
      'Só você e o mestre',
    );
    expect(plain(el.textContent)).toContain('2 de 3');
    expect(plain(el.textContent)).toContain('1 de 3');
  });

  it('says nothing about who sees them when everybody does', () => {
    expect(setup(false).querySelector('[data-testid="death-private"]')).toBeNull();
  });
});

describe("the master's order", () => {
  function setup(deathsHidden: boolean) {
    const fixture = TestBed.createComponent(OrderList);
    fixture.componentRef.setInput(
      'encounter',
      encounter({ combatants: [brisaDown], currentCombatantId: 'b' }),
    );
    fixture.componentRef.setInput('deathsHidden', deathsHidden);
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }

  it('names who else sees the marks when the table hides them', () => {
    const el = setup(true);
    expect(plain(el.querySelector('app-death-row')?.textContent)).toContain(
      'Testes contra a morte',
    );
    expect(plain(el.querySelector('app-death-row')?.textContent)).toContain('Dono e mestre');
    expect(el.querySelectorAll('app-death-marks').length).toBe(2);
  });

  it('draws the marks alone when they are public', () => {
    const el = setup(false);
    expect(plain(el.querySelector('app-death-row')?.textContent)).not.toContain('Dono e mestre');
  });
});

describe('the log of another player', () => {
  function setup(deathNote: boolean) {
    const log = new CombatLogState();
    const stable = create(CombatLogEntrySchema, {
      id: 'e1',
      kind: CombatLogKind.DEATH_SAVE,
      round: 2,
      actorLabel: 'Brisa',
      deathSave: { stable: true },
    });
    log.rounds.set([create(CombatLogRoundSchema, { round: 2, entries: [stable] })]);
    TestBed.configureTestingModule({
      providers: [
        { provide: CombatClient, useValue: {} },
        { provide: MatDialog, useValue: { open: vi.fn() } },
        { provide: MatBottomSheet, useValue: { open: vi.fn() } },
      ],
    });
    const fixture = TestBed.createComponent(CombatLogPanel);
    fixture.componentRef.setInput('log', log);
    fixture.componentRef.setInput('encounter', encounter({ round: 2, combatants: [brisaDown] }));
    fixture.componentRef.setInput('campaignId', 'c');
    fixture.componentRef.setInput('state', new CombatState());
    fixture.componentRef.setInput('deathNote', deathNote);
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }

  it('says why a save is not in it, and reads a stable result with no roll as "estabilizou"', () => {
    const el = setup(true);
    expect(plain(el.querySelector('[data-testid="death-hidden-note"]')?.textContent)).toContain(
      'Esta mesa só deixa o dono e o mestre verem os testes contra a morte.',
    );
    expect(plain(el.textContent)).toContain('Brisa estabilizou');
    expect(plain(el.textContent)).not.toContain('teste contra a morte:');
  });

  it('has no note when the saves are public', () => {
    expect(setup(false).querySelector('[data-testid="death-hidden-note"]')).toBeNull();
  });
});
