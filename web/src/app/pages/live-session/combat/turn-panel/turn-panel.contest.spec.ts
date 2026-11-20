import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';

import { CombatantKind } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { ContestTurnStateSchema } from '../../../../../gen/meurpg/play/v1/contest_types_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { textOf } from '../../../../core/combat/contest-testing';
import { TurnPanel } from './turn-panel';

function panel(label: string, state: Parameters<typeof create<typeof ContestTurnStateSchema>>[1]) {
  const fixture = TestBed.createComponent(TurnPanel);
  fixture.componentRef.setInput(
    'encounter',
    encounter({
      currentCombatantId: 'me',
      combatants: [
        combatant({ id: 'me', label, kind: CombatantKind.PLAYER, mine: true }),
        combatant({ id: 'g', label: 'Goblin 1' }),
      ],
    }),
  );
  fixture.componentRef.setInput('contest', create(ContestTurnStateSchema, state));
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

describe('TurnPanel: Escondido and Surpresa (W7-X)', () => {
  it('says "Escondido" with the sentence, never from whom (board W7-Xc 8b)', () => {
    const el = panel('Brisa', { hidden: true });
    const tag = el.querySelector('.turn__state .mr-tag');
    expect(textOf(tag!)).toBe('Escondido');
    expect(tag?.classList).toContain('mr-tag--success');
    expect(textOf(el.querySelector('.turn__state')!)).toBe('Escondido Você está escondido.');
    expect(textOf(el)).not.toMatch(/Percepção|notou|vê claramente/);
  });

  it('says "Surpresa" with what it means for the turn (board W7-Xc 11)', () => {
    const el = panel('Nael', { surprised: true });
    expect(textOf(el.querySelector('.turn__state')!)).toBe(
      'Surpresa Surpresa neste turno: você não se move, não age e não reage até o fim do turno.',
    );
  });

  it('turns the end of the turn into "Passar o turno" for a surprised character, with nothing to ask', () => {
    const el = panel('Nael', { surprised: true });
    const end = el.querySelector<HTMLButtonElement>('app-end-turn button');
    expect(textOf(end!)).toBe('Passar o turno');
    expect(end?.classList).toContain('end--filled');
    const normal = panel('Nael', {});
    expect(textOf(normal.querySelector<HTMLButtonElement>('app-end-turn button')!)).toBe(
      'Encerrar turno',
    );
  });

  it('says nothing for a turn with no such state', () => {
    expect(panel('Brisa', {}).querySelector('.turn__state')).toBeNull();
  });
});
