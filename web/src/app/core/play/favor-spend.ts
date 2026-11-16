import { computed, effect, signal } from '@angular/core';

import {
  type AdvantageSource,
  AdvantageSourceKind,
  RollMode,
} from '../../../gen/meurpg/play/v1/combat_rolls_pb';
import { RollModeKind, type RollNote } from '../../../gen/meurpg/play/v1/contest_types_pb';
import { FavorBlockedReason } from '../../../gen/meurpg/play/v1/favor_pb';
import type { Combatant } from '../../../gen/meurpg/play/v1/combat_pb';
import { isPlayer } from '../combat/combat-view';
import { orNormal } from '../combat/roll-mode';
import { sourceName } from './favor-client';
import { favorBlocked, favorBlockedMessage } from './favor-errors';
import { favorPassPendingText } from './favor-pass';
import type { FavorState } from './favor-state';

/** The sentence under the box when the Inspiração is the master's to hold, not the master's to spend (board 581-07, item 7). */
export const FAVOR_ONLY_PLAYER = 'Só quem joga com este personagem pode usar a Inspiração dele.';
/** The read of who has it failed: the box is there, dashed, and says so (581-07, item 13). */
export const FAVOR_READ_FAILED = 'Não deu para ler a Inspiração agora. Tente de novo em instantes.';
/** A mode asked of the master is on its way: the Inspiração stays out of this roll (the app's own words, not on a board). */
export const FAVOR_MODE_ASKED = 'Há um pedido de modo ao mestre nesta rolagem.';
/** The words of the lock the roll-mode segment wears while the box is ticked (581-03). */
export const FAVOR_LOCK_WORD = 'pela Inspiração';

/** "O mestre agindo por Brisa". */
export function favorByMasterText(name: string): string {
  return `O mestre agindo por ${name}`;
}

/** What a roll's circumstances say, as the roller reads them: the sources they read, by side, and what the ones they may not read
 * ("Outra fonte") do. The spend is judged as the server judges it (`favorMode`, `favorAgainst`, `visibleSources`): ADDS_NOTHING on the
 * sources the roller reads alone, so a refusal never tells them a hidden source exists; the mode the roll then has on every source,
 * hidden ones included. */
export interface FavorCircumstances {
  /** The mode the roll would have without the Inspiração, on every source. */
  readonly mode: RollMode;
  /** The names of the sources the roller reads, by side. */
  readonly advantages: readonly string[];
  readonly disadvantages: readonly string[];
  /** A source the roller may not read gives disadvantage: it counts toward the mode, never toward the refusal. */
  readonly hiddenDisadvantage?: boolean;
  /** A source the roller reads has no side the wire says: the sheet cannot judge it, the server does. */
  readonly undecided?: boolean;
}

/** The circumstances of an attack or a spell attack: the server's sources (`TargetInReach.sources`) and its suggested mode. A
 * hidden source (`OTHER_SOURCE`) keeps its side on the wire; the Inspiração itself is not a circumstance. */
export function circumstancesOfSources(
  mode: RollMode,
  sources: readonly AdvantageSource[],
): FavorCircumstances {
  const seen = sources.filter(
    (s) => s.kind !== AdvantageSourceKind.OTHER_SOURCE && s.kind !== AdvantageSourceKind.FAVOR,
  );
  const named = (effect: RollMode) =>
    seen.filter((s) => s.effect === effect).map((s) => sourceName(s.textPt));
  return {
    mode,
    advantages: named(RollMode.ADVANTAGE),
    disadvantages: named(RollMode.DISADVANTAGE),
    hiddenDisadvantage: sources.some(
      (s) => s.kind === AdvantageSourceKind.OTHER_SOURCE && s.effect === RollMode.DISADVANTAGE,
    ),
  };
}

/** The circumstances of a check: the mode of the check's option and its notes (`RollNote`). A bonus an effect adds says nothing of the
 * mode, a hidden source reads "other" (it has a side, and never counts toward the refusal) and the Inspiração itself is not one of
 * the circumstances. */
export function circumstancesOfNotes(
  mode: RollModeKind,
  notes: readonly RollNote[],
): FavorCircumstances {
  const seen = notes.filter((n) => !['other', 'favor', 'effect_bonus'].includes(n.kind));
  return {
    mode:
      mode === RollModeKind.ADVANTAGE
        ? RollMode.ADVANTAGE
        : mode === RollModeKind.DISADVANTAGE
          ? RollMode.DISADVANTAGE
          : RollMode.NORMAL,
    advantages: seen.filter((n) => n.advantage).map((n) => n.labelPt),
    disadvantages: seen.filter((n) => !n.advantage).map((n) => n.labelPt),
    hiddenDisadvantage: notes.some((n) => n.kind === 'other' && !n.advantage),
  };
}

/** The side a sentence of a prompt says ("Esquivando: vantagem", "Outra fonte: desvantagem"), or `null` when it says none. */
function sideOf(text: string): 'advantage' | 'disadvantage' | null {
  if (/desvantagem/i.test(text)) {
    return 'disadvantage';
  }
  return /vantagem/i.test(text) ? 'advantage' : null;
}

/** The circumstances of a saving throw a window asks (the concentration save, an effect's save): the prompt's `mode` ("normal",
 * "advantage" or "disadvantage", on every source) and its sources in words, each "<name>: vantagem|desvantagem"; a hidden one reads
 * "Outra fonte: …" and is not counted for the refusal. A sentence that names no side takes the side of the mode when the mode is
 * not normal (every source then has it); under a normal mode it is `undecided`, and the server judges. */
export function circumstancesOfPrompt(
  mode: string,
  sourcesPt: readonly string[],
): FavorCircumstances {
  const resolved =
    mode === 'advantage'
      ? RollMode.ADVANTAGE
      : mode === 'disadvantage'
        ? RollMode.DISADVANTAGE
        : RollMode.NORMAL;
  const fallback = mode === 'advantage' || mode === 'disadvantage' ? mode : null;
  const advantages: string[] = [];
  const disadvantages: string[] = [];
  let hiddenDisadvantage = mode === 'disadvantage';
  let undecided = false;
  for (const text of sourcesPt) {
    const side = sideOf(text) ?? fallback;
    if (text.startsWith('Outra fonte')) {
      hiddenDisadvantage ||= side === 'disadvantage';
    } else if (side === 'advantage') {
      advantages.push(sourceName(text));
    } else if (side === 'disadvantage') {
      disadvantages.push(sourceName(text));
    } else {
      undecided = true;
    }
  }
  return { mode: resolved, advantages, disadvantages, hiddenDisadvantage, undecided };
}

/** What the Inspiração would do to a roll: nothing (and why), or the mode the roll then has and the sentence that says it. */
export type FavorJudgement =
  | { readonly ok: false; readonly reason: string }
  | { readonly ok: true; readonly mode: RollMode | null; readonly line: string };

/** The one place that works out the mode a roll has with the Inspiração, as the server does (`spendFavor`, `favorMode`): it adds
 * nothing (`ADDS_NOTHING`) when the sources the roller reads include an advantage, whatever else is there; otherwise it changes
 * the roll, to Normal when any source (read or not) gives disadvantage, else to advantage. A source whose side the wire does not
 * say leaves the mode to the server (`mode: null`). */
export function judgeFavor(c: FavorCircumstances): FavorJudgement {
  const nothing = 'A Inspiração não mudaria esta rolagem:';
  if (c.advantages.length > 0 && c.disadvantages.length > 0) {
    return {
      ok: false,
      reason: `${nothing} vantagem (${c.advantages.join(', ')}) e desvantagem (${c.disadvantages.join(', ')}) se anulam.`,
    };
  }
  if (c.advantages.length > 0) {
    return { ok: false, reason: `${nothing} ela já tem vantagem (${c.advantages.join(', ')}).` };
  }
  if (c.undecided) {
    return { ok: true, mode: null, line: '' };
  }
  if (
    orNormal(c.mode) === RollMode.DISADVANTAGE ||
    c.disadvantages.length > 0 ||
    c.hiddenDisadvantage
  ) {
    const names = c.disadvantages.length > 0 ? ` (${c.disadvantages.join(', ')})` : '';
    return {
      ok: true,
      mode: RollMode.NORMAL,
      line: `Desvantagem${names} e Inspiração se anulam: um d20. Desmarque a caixa para voltar a desvantagem.`,
    };
  }
  return {
    ok: true,
    mode: RollMode.ADVANTAGE,
    line: 'Vantagem pela Inspiração: rola dois d20 e fica com o maior. Desmarque a caixa para escolher o modo.',
  };
}

/** What the box shows. `open`: it can be ticked (`mode` is what the roll then reads, `null` when the sheet does not know the
 * circumstances beforehand). `off`: dashed, with `sub` under the label and `reason` in its own line. */
export type FavorView =
  | { readonly kind: 'open'; readonly mode: RollMode | null; readonly line: string }
  | { readonly kind: 'off'; readonly sub: string; readonly reason: string };

/** The character a roll is for. An empty `characterId` is the player's own, for the sheets only a player opens (a scene check, a
 * group check, a trap search, a hint): the read has none but their characters. */
export interface FavorWho {
  readonly characterId: string;
  readonly name: string;
}

/** The character of a combatant when it is a player's (an NPC or a creature has no Inspiração: `null`). */
export function favorWho(c: Combatant | undefined | null): FavorWho | null {
  return c && isPlayer(c) ? { characterId: c.characterId, name: c.label } : null;
}

/** What a roll sheet tells the box. */
export interface FavorSpendHost {
  readonly favor: FavorState;
  /** Who the roll is for, or `null` for one that has no Inspiração (an NPC, a creature): then nothing is drawn. */
  readonly who: () => FavorWho | null;
  /** The master opened the sheet: he acts for the player and cannot spend what is the player's. */
  readonly byMaster?: () => boolean;
  /** The circumstances the sheet knows before the roll; without them the server judges alone. */
  readonly circumstances?: () => FavorCircumstances | null;
  /** The text of a pass that waits for the master, or `''`. A sheet leaves it out: the box reads the player's own ask from `favor` and
   * turns dashed with "Você pediu para passar a Inspiração a Tor. Cancele o pedido para usar." (581-07, item 4). */
  readonly passPending?: () => string;
  /** A reason that comes from the sheet itself: a mode asked of the master. */
  readonly sheetReason?: () => string;
  /** The tick changed: the request is another one, so a sheet that keeps one key for a retry makes a new one. */
  readonly changed?: () => void;
}

/**
 * The spend of the master's Inspiração on one roll, as a roll sheet holds it (581-03, 581-07): whether the box shows and how, the
 * tick, and the mode the roll reads while it is ticked. The sheet sends `spendFavor: favor.spend()` and no `rollMode` while it is
 * ticked (the server refuses a worse mode beside the spend), and words a refusal with `refused`.
 */
export class FavorSpend {
  /** The box is ticked. Only counts while it can be (`spend`). */
  readonly ticked = signal(false);

  readonly view = computed<FavorView | null>(() => {
    const { favor } = this.host;
    const who = this.host.who();
    if (favor.campaignId() === '' || !who) {
      return null;
    }
    if (favor.failed()) {
      return { kind: 'off', sub: '', reason: FAVOR_READ_FAILED };
    }
    const holds = who.characterId === '' ? favor.count() > 0 : favor.has(who.characterId);
    if (!holds) {
      return null;
    }
    const off = this.sheetOff(who);
    if (off) {
      return off;
    }
    const c = this.host.circumstances?.();
    if (!c) {
      return { kind: 'open', mode: null, line: '' };
    }
    const j = judgeFavor(c);
    return j.ok
      ? { kind: 'open', mode: j.mode, line: j.line }
      : { kind: 'off', sub: '', reason: j.reason };
  });

  /** The reasons the sheet itself knows to dash the box: the master acts for the player, an ask to pass it waits, a mode was asked. */
  private sheetOff(who: FavorWho): FavorView | null {
    if (this.host.byMaster?.()) {
      const name = who.name || this.host.favor.holders().get(who.characterId) || '';
      return { kind: 'off', sub: favorByMasterText(name), reason: FAVOR_ONLY_PLAYER };
    }
    const reason =
      (this.host.passPending?.() ?? '') || this.askedText(who) || (this.host.sheetReason?.() ?? '');
    return reason === '' ? null : { kind: 'off', sub: '', reason };
  }

  /** The reason of the player's own ask that waits for the master: the character's, or (a sheet only a player opens) any of theirs. */
  private askedText(who: FavorWho): string {
    const { favor } = this.host;
    const ask = who.characterId === '' ? favor.passes()[0] : favor.passOf(who.characterId);
    return ask ? favorPassPendingText(ask.toName) : '';
  }

  /** The box is ticked and can be: the request carries `spend_favor`. */
  readonly spend = computed(() => this.ticked() && this.view()?.kind === 'open');

  /** The mode the roll reads while the box is ticked, `null` when it is not (or the sheet does not know). */
  readonly mode = computed<RollMode | null>(() => {
    const v = this.view();
    return this.spend() && v?.kind === 'open' ? v.mode : null;
  });

  /** What the roll-mode segment wears while the box is ticked and the mode is known: it only reads the mode (`app-roll-mode-picker`). */
  readonly lock = computed(() => {
    const v = this.view();
    const mode = this.mode();
    return v?.kind === 'open' && mode !== null
      ? { mode, word: FAVOR_LOCK_WORD, line: v.line }
      : null;
  });

  /** Built in an injection context (a sheet's field): a tick never outlives the box being open, so a box that closes (another target,
   * a mode asked, a read that failed) and opens again is not ticked, and cannot spend on circumstances the player never saw. */
  constructor(private readonly host: FavorSpendHost) {
    effect(() => {
      if (this.view()?.kind !== 'open') {
        this.ticked.set(false);
      }
    });
  }

  set(ticked: boolean): void {
    this.ticked.set(ticked);
    this.host.changed?.();
  }

  /** The roll was made with the Inspiração (or a question holds it): it is spent, so the box is unticked and who has it is read
   * again, which takes the box away at once instead of on the next hint. A roll that did not use it changes nothing. */
  spent(): void {
    if (this.ticked()) {
      this.ticked.set(false);
      const who = this.host.who();
      if (who) {
        this.host.favor.spentBy(who.characterId);
      }
      void this.host.favor.refresh();
    }
  }

  /** What a refused roll says when the server refused the Inspiração (any `FavorBlocked`), or `null` for another error. The box
   * is unticked, so the next try goes without it; with no Inspiração left the read is made again, which takes the box away. */
  refused(err: unknown): string | null {
    const blocked = favorBlocked(err);
    if (!blocked) {
      return null;
    }
    this.ticked.set(false);
    if (blocked.reason === FavorBlockedReason.HAS_NONE) {
      void this.host.favor.refresh();
    }
    return favorBlockedMessage(blocked.reason);
  }
}
