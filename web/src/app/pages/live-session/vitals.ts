import { PartyMemberInfoVm, VitalsVm } from './live-session.types';

/**
 * Applies a snapshot (`GetLiveSession`) to what is on screen. The snapshot
 * decides which characters are there (one may have died or left), and for
 * each one the copy with the larger `revision` wins: a `vitals_changed` can
 * arrive before the snapshot answers (play.proto, WatchGameSession).
 */
export function applySnapshot(
  current: readonly VitalsVm[],
  snapshot: readonly VitalsVm[],
): VitalsVm[] {
  const byId = new Map(current.map((v) => [v.characterId, v]));
  return snapshot.map((fresh) => {
    const seen = byId.get(fresh.characterId);
    return seen && seen.revision > fresh.revision ? seen : fresh;
  });
}

/**
 * Applies one `vitals_changed` (or a saved correction): only if its
 * revision is newer than the one on screen, or if it is the same copy
 * without the familiar's sight: the server drops that sight on read when the
 * familiar is dismissed, and nothing bumps the revision. A character not on screen yet
 * (approved during the session) joins the end of the list.
 */
export function applyVitals(current: readonly VitalsVm[], incoming: VitalsVm): VitalsVm[] {
  const index = current.findIndex((v) => v.characterId === incoming.characterId);
  if (index < 0) {
    return [...current, incoming];
  }
  const seen = current[index];
  const sightDropped =
    seen.revision === incoming.revision && !!seen.familiarSight && !incoming.familiarSight;
  if (seen.revision >= incoming.revision && !sightDropped) {
    return [...current];
  }
  const next = [...current];
  next[index] = incoming;
  return next;
}

/** The HP bar's fill, 0 to 100. */
export function hitPointsPercent(v: Pick<VitalsVm, 'hitPointsCurrent' | 'hitPointsMax'>): number {
  if (v.hitPointsMax <= 0) {
    return 0;
  }
  return Math.max(0, Math.min(100, (v.hitPointsCurrent / v.hitPointsMax) * 100));
}

/**
 * The word for the master's party row, or `null`: "Inconsciente" at 0 HP,
 * "Abaixo da metade" when current × 2 < max (both proposals of the design,
 * README-A). A word, never a colour alone.
 */
export function hitPointsState(
  v: Pick<VitalsVm, 'hitPointsCurrent' | 'hitPointsMax'>,
): { label: string; tone: 'danger' | 'pending' } | null {
  if (v.hitPointsMax > 0 && v.hitPointsCurrent === 0) {
    return { label: 'Inconsciente', tone: 'danger' };
  }
  if (v.hitPointsCurrent * 2 < v.hitPointsMax) {
    return { label: 'Abaixo da metade', tone: 'pending' };
  }
  return null;
}

/** "2 de 4 usados": every count on these screens says "usados" (README-A). */
export function usedWords(used: number, total: number): string {
  return `${used} de ${total} usados`;
}

/** "1º nível". */
/** "1 livre de 4", "0 livres de 2": the slots still free, as the combat
 * screens count them (timeline decision 7). */
export function freeWords(used: number, total: number): string {
  const free = Math.max(0, total - used);
  return `${free} ${free === 1 ? 'livre' : 'livres'} de ${total}`;
}

export function slotLevelLabel(level: number): string {
  return `${level}º nível`;
}

/** The accessible name of a row of slot dots. */
export function slotRowLabel(label: string, used: number, total: number): string {
  return `${label}: ${freeWords(used, total)}`;
}

/** "Mago 3, de Vinicius" under a name in "Grupo" (or "Mago 3, jogador sem
 * nome" when the player chose no display name). */
export function partyRowSub(info: PartyMemberInfoVm | undefined): string {
  if (!info) {
    return '';
  }
  // A player with no display name is not written about: the class alone (a placeholder says nothing).
  const player = info.playerName ? `de ${info.playerName}` : '';
  return [info.classSummary, player].filter(Boolean).join(', ');
}

/** "Ana vê a mudança na hora." in the adjust sheet (only that character's
 * player sees its HP, decision row 28). Without a display name, the
 * sentence names the character instead, which also keeps it free of a
 * gender we don't store. */
export function whoSeesTheChange(playerName: string | null, characterName: string): string {
  return playerName
    ? `${playerName} vê a mudança na hora.`
    : `Quem joga com ${characterName} vê a mudança na hora.`;
}
