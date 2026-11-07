import { type Attack, AttackKind } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { THROW_BACK_KEY } from '../attack-sheet/attack-sheet-data';

/** A weapon attack (not the throw-back of a thrown weapon) is still on offer: the rows a maneuver die may go into. */
export const offersWeaponAttack = (open: readonly { readonly attack?: Attack }[]): boolean =>
  open.some((a) => a.attack?.kind === AttackKind.WEAPON && a.attack.key !== THROW_BACK_KEY);
