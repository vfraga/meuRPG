// A row inside an unresolved tie still offers "Editar": a mistyped physical die (RN-18) that ties
// is corrected without ordering the tie first.
import { TestBed } from '@angular/core/testing';

import { CombatantKind, EncounterStatus } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { InitiativeSetup } from './initiative-setup';

describe('tied rows can edit the typed die', () => {
  it('shows an edit control on each row of an unresolved tie', () => {
    const fixture = TestBed.createComponent(InitiativeSetup);
    const rolled = (id: string, label: string, kind: CombatantKind) =>
      combatant({
        id,
        label,
        kind,
        initiative: 14,
        initiativeFace: 12,
        initiativeBonus: 2,
        tieUnresolved: true,
      });
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        status: EncounterStatus.SETUP,
        round: 0,
        combatants: [
          rolled('brisa', 'Brisa', CombatantKind.PLAYER),
          rolled('g1', 'Goblin 1', CombatantKind.NPC),
        ],
      }),
    );
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    expect(el.textContent).toContain('Empate em 14');
    const edits = Array.from(el.querySelectorAll('li.row')).map((row) =>
      Array.from(row.querySelectorAll('button')).some((b) => b.textContent?.includes('Editar')),
    );
    expect(edits).toEqual([true, true]);
  });
});
