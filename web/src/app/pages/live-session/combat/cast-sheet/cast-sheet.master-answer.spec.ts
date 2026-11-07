// A spell attack held for a question: when the master answers it for the caster, the sheet drops the question and says so.
import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { ActionEconomy } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { heldLucky } from '../../../../core/held-rolls/held-roll-testing';
import { SpellCatalog } from '../../../../core/combat/spell-catalog';
import { ResourceClient } from '../../../../core/resources/resources-client';
import { CastSheet, type CastSheetData } from './cast-sheet';

const flat = (n: Element | null | undefined) => n?.textContent?.replace(/\s+/g, ' ').trim() ?? '';

// A hold of its own: the page remembers which questions this browser answered, across specs.
const lucky = heldLucky([1], 5, { holdId: 'hold-cast-master' });

describe('the cast sheet when the master answers the question', () => {
  function setup() {
    TestBed.resetTestingModule();
    const state = {
      encounter: signal(encounter({ combatants: [combatant({ id: 's', label: 'Pipo' })] })),
      apply: vi.fn(),
    };
    const data = {
      campaignId: 'c',
      encounterId: 'enc',
      casterId: 's',
      round: 1,
      spellKey: 'spell:guiding-bolt',
      name: 'Raio Guiador',
      level: 1,
      concentration: false,
      cantripDice: '',
      economy: ActionEconomy.ACTION,
      slots: [],
      usage: [],
      pact: null,
      targets: undefined,
      shieldFree: null,
      shieldName: '',
      attackBonus: 5,
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
      state,
    } as unknown as CastSheetData;
    TestBed.configureTestingModule({
      providers: [
        { provide: CombatClient, useValue: {} },
        { provide: ResourceClient, useValue: { answerHeldRoll: vi.fn() } },
        { provide: SpellCatalog, useValue: { details: () => Promise.resolve(null) } },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: vi.fn() } },
      ],
    });
    const fixture = TestBed.createComponent(CastSheet);
    fixture.detectChanges();
    const sheet = fixture.componentInstance as unknown as { offer: { set(v: unknown): void } };
    const el = fixture.nativeElement as HTMLElement;
    const buttons = () => Array.from(el.querySelectorAll('button')).map((b) => flat(b));
    const settle = async () => {
      await fixture.whenStable();
      fixture.detectChanges();
    };
    return { fixture, el, state, sheet, buttons, settle };
  }

  beforeEach(() => {
    Element.prototype.scrollTo = vi.fn();
  });

  it('drops the question and says the master answered when the caster no longer carries it', async () => {
    const { fixture, el, state, sheet, buttons, settle } = setup();
    // The roll was held: the combat carries the question on the caster, as the server sends it.
    state.encounter.set(
      encounter({ combatants: [combatant({ id: 's', label: 'Pipo', heldRollOffer: lucky })] }),
    );
    sheet.offer.set(lucky);
    fixture.detectChanges();
    await settle();
    expect(buttons()).toContain('Ficar com o 1');
    // The master answered: the next read of the combat has no question on the caster.
    state.encounter.set(encounter({ combatants: [combatant({ id: 's', label: 'Pipo' })] }));
    await settle();
    expect(flat(el.querySelector('[data-testid="sheet-held-vanished"]'))).toContain(
      'O mestre respondeu por você.',
    );
    expect(buttons()).not.toContain('Ficar com o 1');
    expect(buttons().filter((b) => b.startsWith('Rolar de novo'))).toEqual([]);
  });

  it('keeps the question while the combat still carries it', async () => {
    const { fixture, el, state, sheet, buttons, settle } = setup();
    state.encounter.set(
      encounter({ combatants: [combatant({ id: 's', label: 'Pipo', heldRollOffer: lucky })] }),
    );
    sheet.offer.set(lucky);
    fixture.detectChanges();
    await settle();
    expect(el.querySelector('[data-testid="sheet-held-vanished"]')).toBeNull();
    expect(buttons()).toContain('Ficar com o 1');
  });
});
