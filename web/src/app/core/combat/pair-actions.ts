import type { Combatant, GetTurnOptionsResponse } from '../../../gen/meurpg/play/v1/combat_pb';
import { DisabledReasonCode } from '../../../gen/meurpg/rules/v1/rules_pb';
import { article } from './combat-log';
import { reasonText } from './combat-options';

/** One of the three actions a controlled mount takes while it carries its rider (SRD 5.1, "Mounted Combat"). */
export interface PairAction {
  /** "standard:dash", the key `TakeAction` takes. */
  readonly key: string;
  readonly name: string;
  readonly icon: string;
  /** The server refused it, or its options are not read yet: dashed, in its place. */
  readonly off: boolean;
}

const THREE: readonly { key: string; name: string; icon: string }[] = [
  { key: 'standard:dash', name: 'Disparada', icon: 'keyboard_double_arrow_right' },
  { key: 'standard:disengage', name: 'Desengajar', icon: 'keyboard_tab' },
  { key: 'standard:dodge', name: 'Esquivar', icon: 'shield' },
];

/**
 * The pair card's three actions from the mount's own options, and the one line that says why they are off: "Caído: o mestre
 * conduz o Cavalo de guerra." for a rider at 0 hit points (COMBATANT_DOWN, 588-10 card 5), else the server's reason. Pure.
 */
export function pairActions(
  res: GetTurnOptionsResponse | null,
  rider: Combatant,
  mount: Combatant,
): { actions: PairAction[]; why: string } {
  const rows = res?.options?.standardActions ?? [];
  let why = '';
  const actions = THREE.map((t) => {
    const row = rows.find((a) => a.action?.key === t.key);
    const off = !row || !row.enabled;
    if (row && off && !why) {
      why =
        row.reason?.code === DisabledReasonCode.COMBATANT_DOWN
          ? `${article(rider.label) === 'a' ? 'Caída' : 'Caído'}: o mestre conduz ${article(mount.label) === 'a' ? 'a' : 'o'} ${mount.label}.`
          : reasonText(row.reason);
    }
    return { key: t.key, name: row?.action?.namePt || t.name, icon: t.icon, off };
  });
  return { actions, why };
}
