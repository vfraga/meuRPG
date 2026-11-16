import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';

import { CombatantSchema } from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  AttackKind,
  AttackOptionSchema,
  TurnOptionsSchema,
  VersatileGrip,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { ActionGroups } from './action-groups';

const plain = (t: string | null | undefined) => (t ?? '').replace(/\s+/g, ' ').trim();

function axe(grip: VersatileGrip) {
  return create(AttackOptionSchema, {
    enabled: true,
    attack: {
      key: 'equipment:woodcutter-axe',
      name: 'Woodcutter axe',
      namePt: 'Machado de lenhador',
      kind: AttackKind.WEAPON,
      attackBonus: 4,
      rangeFt: 5,
      damage: '1d6 + 3',
      versatileDamage: '1d8 + 3',
      damageTypePt: 'cortante',
      grip,
    },
  });
}

function setup(grip: VersatileGrip) {
  const fixture = TestBed.createComponent(ActionGroups);
  fixture.componentRef.setInput('options', create(TurnOptionsSchema, { attacks: [axe(grip)] }));
  fixture.componentRef.setInput(
    'own',
    create(CombatantSchema, { id: 'me', label: 'Tor', mine: true }),
  );
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  return {
    line: plain(el.querySelector('[data-testid="versatile-line"]')?.textContent),
    stats: plain(el.querySelector('.row__detail')?.textContent),
  };
}

describe('ActionGroups: the two-handed line of a versatile weapon (board 580-14)', () => {
  it('keeps the one-handed dice and says what two hands need while a shield is equipped', () => {
    const { stats, line } = setup(VersatileGrip.ONE_HAND_SHIELD);
    expect(stats).toContain('1d6 + 3 cortante');
    expect(line).toBe('Com duas mãos, sem o escudo: 1d8 + 3');
  });

  it('says another weapon in hand is what stops the two hands', () => {
    expect(setup(VersatileGrip.ONE_HAND_WEAPON).line).toBe(
      'Com duas mãos, sem a outra arma: 1d8 + 3',
    );
  });

  it('keeps the one-handed dice for a lone weapon of a Dueling character, and the line gives the two-handed ones', () => {
    const { stats, line } = setup(VersatileGrip.ONE_HAND_DUELING);
    expect(stats).toContain('1d6 + 3 cortante');
    expect(line).toBe('Com duas mãos, sem o Duelismo: 1d8 + 3');
  });

  it('shows the two-handed dice on the stats and the one-handed ones on the line with both hands free', () => {
    const { stats, line } = setup(VersatileGrip.TWO_HANDS);
    expect(stats).toContain('1d8 + 3 cortante');
    expect(line).toBe('Com duas mãos; com uma: 1d6 + 3');
  });

  it('shows no line for a weapon that is not versatile', () => {
    expect(setup(VersatileGrip.UNSPECIFIED).line).toBe('');
  });
});
