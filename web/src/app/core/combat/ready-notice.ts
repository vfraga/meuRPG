import {
  type CombatLogEntry,
  CombatLogKind,
  ReadiedKind,
  ReadiedStatus,
} from '../../../gen/meurpg/play/v1/combat_pb';

/**
 * What the readier is told when the action they held was lost without them asking (#582, board 582-08, case 3): from the
 * `READIED_LOST` line of their own row (the server writes its payload only for them and the master, RN-10). The line has no
 * reason, only whether the owner gave it up (`canceled`), so the notice does not say why. A spell the readier lets pass is lost with
 * its own line (the server writes the fired line of the pass, then the loss, in one change): the readier just chose that, so the
 * notice is not told for it. Words and bookkeeping only.
 */

/** A lost row to tell. */
export interface ReadiedLost {
  /** The held action's name ("Espada longa", "Mísseis Mágicos"); `''` when the content has none. */
  readonly held: string;
  /** The held action was a spell: its slot and its concentration go with it. */
  readonly spell: boolean;
}

/** "A sua ação preparada se perdeu." and what is left of it: the lead is bold on screen. */
export function readiedLostNoticeText(lost: ReadiedLost): {
  readonly lead: string;
  readonly rest: string;
} {
  return lost.spell
    ? {
        lead: `${lost.held || 'A magia preparada'} se perdeu.`,
        rest: 'A ação preparada acabou. O espaço gasto não volta.',
      }
    : {
        lead: 'A sua ação preparada se perdeu.',
        rest: `${lost.held ? `${lost.held}. ` : ''}A ação gasta não volta.`,
      };
}

/**
 * Reads the combat log for the loss of the player's own readied action. What the log already held when the combat was first
 * read is not news (a reconnect or a re-read never repeats a notice), the owner's own `CancelReadied` says nothing, and another
 * combat starts clean.
 */
export class ReadiedNotices {
  private seen: Set<string> | null = null;
  private combat = '';

  /** `reset` says the combat changed (clear what was on screen); `notice` is the new loss to tell, if any. */
  read(
    encounterId: string,
    ownId: string,
    loaded: boolean,
    entries: readonly CombatLogEntry[],
  ): { reset: boolean; notice: ReadiedLost | null } {
    const reset = encounterId !== this.combat;
    if (reset) {
      this.combat = encounterId;
      this.seen = null;
    }
    if (!loaded || !ownId) {
      return { reset, notice: null };
    }
    const lines = entries.filter(
      (en) => en.kind === CombatLogKind.READIED_LOST && en.actorId === ownId && !!en.readiedLost,
    );
    if (this.seen === null) {
      this.seen = new Set(lines.map((l) => l.id));
      return { reset, notice: null };
    }
    // The rows the readier's own pass let go: the loss that follows is theirs, not news.
    const ownPass = new Set(
      entries
        .filter(
          (en) =>
            en.kind === CombatLogKind.READIED_FIRED &&
            en.actorId === ownId &&
            !en.byMaster &&
            en.readiedFired?.status === ReadiedStatus.IGNORED,
        )
        .map((en) => en.readiedFired?.readiedId),
    );
    let notice: ReadiedLost | null = null;
    for (const line of lines) {
      if (this.seen.has(line.id) || !line.readiedLost) {
        continue;
      }
      this.seen.add(line.id);
      if (!line.readiedLost.canceled && !ownPass.has(line.readiedLost.readiedId)) {
        notice = {
          held: line.readiedLost.actionNamePt,
          spell: line.readiedLost.kind === ReadiedKind.SPELL,
        };
      }
    }
    return { reset, notice };
  }
}
