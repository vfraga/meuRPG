import type { DerivedSheet as GenDerivedSheet } from '../../../gen/meurpg/rules/v1/rules_pb';
import { metersWithFeet } from '../../core/units';

/** The other speeds, in the table's order, then "Respira na água" (`DerivedSheet`). */
export function speedExtrasOf(derived: GenDerivedSheet): readonly string[] {
  const speeds: [number, string][] = [
    [derived.speedFlyFt, 'Voo'],
    [derived.speedSwimFt, 'Natação'],
    [derived.speedClimbFt, 'Escalada'],
    [derived.speedBurrowFt, 'Escavação'],
  ];
  const lines = speeds
    .filter(([ft]) => ft > 0)
    .map(([ft, name]) => `${name} ${metersWithFeet(ft)}`);
  return derived.breathesWater ? [...lines, 'Respira na água'] : lines;
}
