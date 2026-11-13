import type { Attack, DerivedSheet } from '../../../gen/meurpg/rules/v1/rules_pb';
import type { FullSheet } from '../../../gen/meurpg/characters/v1/characters_pb';
import { type GearVm, type MarksVm, gearWithMarks } from '../../core/characters/gear';
import type { WeaponMarkVm } from './character-sheet.types';

/** An attack's marks as the engine derived them. */
export function marksOf(a: Pick<Attack, 'silvered' | 'adamantine' | 'magical'>): MarksVm {
  return { silvered: a.silvered, adamantine: a.adamantine, magical: a.magical };
}

/** The marks the sheet itself sets on its carried weapons. */
export function weaponMarksOf(full: FullSheet): WeaponMarkVm[] {
  return full.weaponMarks.map((m) => ({ weaponKey: m.weaponKey, ...marksOf(m) }));
}

/** The gear rows with their weapons' marks, as the combat's read gives them. */
export function sheetGear(derived: DerivedSheet, full: FullSheet): GearVm[] {
  return gearWithMarks(derived.gear, derived.attacks, full.weaponMarks);
}
