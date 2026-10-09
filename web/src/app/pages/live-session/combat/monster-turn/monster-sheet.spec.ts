import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';

import {
  CreatureLegendarySchema,
  CreatureTurnSchema,
} from '../../../../../gen/meurpg/play/v1/creatures_pb';
import {
  CreatureSheetSchema,
  CreatureTraitSchema,
} from '../../../../../gen/meurpg/play/v1/creature_types_pb';
import { combatant } from '../../../../core/combat/combat-testing';
import { MonsterSheet } from './monster-sheet';

function render(sheet: Parameters<typeof create<typeof CreatureSheetSchema>>[1]) {
  TestBed.resetTestingModule();
  const fixture = TestBed.createComponent(MonsterSheet);
  fixture.componentRef.setInput(
    'turn',
    create(CreatureTurnSchema, {
      creature: {
        summary: {
          namePt: 'Lich',
          sizePt: 'Médio',
          typePt: 'morto-vivo',
          challengeRating: '21',
          xp: 33000,
        },
        alignment: 'any evil alignment',
        armorClass: 17,
        hitPoints: 135,
        passivePerception: 19,
      },
      sheet: create(CreatureSheetSchema, sheet),
      legendary: create(CreatureLegendarySchema, {
        state: {
          perRound: 3,
          left: 3,
          resistanceUses: 3,
          resistanceLeft: 3,
        },
      }),
    }),
  );
  fixture.componentRef.setInput(
    'subject',
    combatant({ id: 'lich', label: 'Lich', hitPointsCurrent: 100, hitPointsMax: 135 }),
  );
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

describe('MonsterSheet', () => {
  it('reads the stat block with the hit points of the combat', () => {
    const el = render({});
    const text = el.textContent ?? '';
    expect(text).toContain('Ficha do monstro');
    expect(text).toContain('Lich');
    expect(text).toContain('ND 21');
    expect(text).toContain('100 de 135');
    expect(text).toContain('Resistência Lendária (3/dia).');
    expect(text).toContain('Ações lendárias (3 por rodada)');
  });

  it('shows a trait the engine does not read as a reminder, never hidden', () => {
    const el = render({
      traits: [
        create(CreatureTraitSchema, {
          key: 'k1',
          name: 'Rejuvenation',
          namePt: 'Rejuvenescimento',
          text: 'It gains a new body.',
        }),
        create(CreatureTraitSchema, {
          key: 'k2',
          name: 'Pack Tactics',
          text: 'Advantage.',
          engineReads: true,
        }),
      ],
    });
    const cards = Array.from(el.querySelectorAll('.reminder'));
    expect(cards).toHaveLength(2);
    expect(cards[0].textContent).toContain('Lembrete: o app não aplica este texto.');
    expect(cards[1].textContent).toContain('O app aplica como fonte de vantagem.');
  });
});
