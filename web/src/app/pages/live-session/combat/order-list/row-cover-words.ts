import {
  type Combatant,
  CombatantSide,
  CoverDegree,
  type CoverSource,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { article } from '../../../../core/combat/combat-log';
import { coverMark, coverText } from '../../../../core/combat/cover';
import { tieNumbers } from '../../../../core/format/text';

/** The cover a combatant has against whoever has the turn (`GetTurnOptions` of the master's subject). */
export type CoverAgainst = ReadonlyMap<string, { cover: CoverDegree; source: CoverSource }>;

/** "Três quartos (do mapa) contra o Pensantus", or `''` when there is none or nobody has the turn. */
export function coverLineOf(
  c: Combatant,
  against: CoverAgainst,
  who: string,
  side: CombatantSide,
): string {
  const got = against.get(c.id);
  const text = got ? coverText(got.cover, got.source) : '';
  // Only against opponents: a combatant of the same side as whoever has the turn is no target of theirs.
  return text && who && c.label !== who && (side === CombatantSide.UNSPECIFIED || c.side !== side)
    ? tieNumbers(`${text} contra ${article(who)} ${who}`)
    : '';
}

export function coverPictogramOf(c: Combatant, against: CoverAgainst): 'half' | 'three' | null {
  return coverMark(against.get(c.id)?.cover ?? CoverDegree.NONE);
}

/** The master marked a cover of their own on this combatant. */
export function hasCoverMark(c: Combatant): boolean {
  return c.coverMark !== CoverDegree.NONE && c.coverMark !== CoverDegree.UNSPECIFIED;
}
