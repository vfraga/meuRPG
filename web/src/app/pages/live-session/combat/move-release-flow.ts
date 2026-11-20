import { Injector, computed, runInInjectionContext, signal } from '@angular/core';

import {
  type Combatant,
  type Encounter,
  ReadiedKind,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import type { CombatClient } from '../../../core/combat/combat-client';
import type { MoveResult } from '../../../core/combat/combat-client-types';
import type { CombatState } from '../../../core/combat/combat-state';
import { currentCombatant, isPlayer } from '../../../core/combat/combat-view';
import { type MoveRelease, moveReleaseView } from '../../../core/combat/move-release';
import { readiedOfWindow } from '../../../core/combat/reaction-readied';
import { ReleaseMark } from '../../../core/combat/release-mark';
import { turnOfText } from '../../../core/combat/release-text';
import { ReleaseWatch } from '../../../core/combat/release-watch';

/** What the Mover page of a release says when the walk stopped before its first square (588-10 card 4's words, for a release). */
export const RELEASE_BLOCKED_TEXT =
  'Algo bloqueia o caminho: nada foi gasto. Escolha outro quadrado.';

/** What `MoveReleaseFlow` reads from the screen it serves. */
export interface MoveReleaseContext {
  readonly api: Partial<Pick<CombatClient, 'attacks'>>;
  readonly campaignId: () => string;
  readonly encounter: () => Encounter | null;
  readonly state: () => CombatState;
  readonly isMaster: () => boolean;
  readonly own: () => Combatant | null;
  /** Shows the readied window again (the player went back from the map). */
  readonly reopenWindow?: (windowId: string) => void;
}

/**
 * The Mover page of a readied move, off the readier's turn (#856, boards 857-06 to 857-08): which window it releases, the wait mark the
 * reactor's player sets while on the map, what became of the window, and the words the page shows. `MoveFlow` owns one: it keeps the
 * page open while the release lasts and closes it when the move was made or the player went back.
 */
export class MoveReleaseFlow {
  /** The release that is open; `null` when the Mover page is the ordinary one. */
  readonly current = signal<MoveRelease | null>(null);
  private watch: ReleaseWatch | null = null;
  private mark: ReleaseMark | null = null;

  /** What the page shows, once a release is open. */
  readonly view = computed(() => {
    const r = this.current();
    const e = this.ctx.encounter();
    if (!r || !e) {
      return null;
    }
    const on = currentCombatant(e);
    const turnOf = on && on.id !== r.reactorId ? turnOfText(on.label, isPlayer(on)) : '';
    return moveReleaseView(r, turnOf, this.watch?.view() ?? null);
  });

  constructor(
    private readonly ctx: MoveReleaseContext,
    private readonly injector: Injector,
  ) {}

  /** Opens the release of the readied move the window holds; `false` when the window or its move is not in the combat. */
  open(windowId: string): MoveRelease | null {
    const e = this.ctx.encounter();
    const w = e?.reactionWindows.find((x) => x.id === windowId);
    const held = e && w ? readiedOfWindow(e, w) : undefined;
    if (!e || !w || held?.kind !== ReadiedKind.MOVE) {
      return null;
    }
    const release: MoveRelease = {
      windowId,
      readiedId: held.id,
      reactorId: w.reactorId,
      reactor: w.reactorLabel,
      trigger: held.triggerText,
      master: this.ctx.isMaster(),
    };
    this.watch = runInInjectionContext(
      this.injector,
      () =>
        new ReleaseWatch(
          () => this.ctx.encounter(),
          {
            windowId,
            readiedId: held.id,
            reactorId: w.reactorId,
            spellName: '',
            move: true,
          },
          (win) => this.mark?.observe(win),
        ),
    );
    // Only the reactor's player marks the window (the master is refused): the server dates the one wait of 120 s from the first mark.
    const attacks = this.ctx.api.attacks;
    this.mark =
      release.master || !attacks
        ? null
        : new ReleaseMark(
            attacks,
            { campaignId: this.ctx.campaignId(), encounterId: e.id, windowId },
            (next) => this.ctx.state().apply(next),
          );
    this.current.set(release);
    void this.mark?.enter();
    return release;
  }

  /** The walk of the release stopped early with the readier still on its square: nothing is spent, the window stays open (the server's rule). */
  blocked(res: MoveResult, before: Combatant | null, after: Combatant | null): boolean {
    return (
      !!this.current() &&
      !res.lockedDoor &&
      res.stoppedEarly &&
      !!before &&
      before.col === after?.col &&
      before.row === after.row
    );
  }

  /** The window went away under the page. */
  gone(): boolean {
    return this.view()?.gone != null;
  }

  /** "Voltar": takes the mark off, ends the release and shows the window again (unless it is gone, or the master is the one back). */
  async back(): Promise<void> {
    const r = this.current();
    if (!r) {
      return;
    }
    const gone = this.gone();
    if (!gone) {
      await this.mark?.leave();
    }
    this.end();
    if (!gone && !r.master) {
      this.ctx.reopenWindow?.(r.windowId);
    }
  }

  /** The move was made: the window closes with it, and there is nothing to clear. */
  finish(): void {
    this.mark?.finish();
    this.end();
  }

  /**
   * The page leaves the release, whichever way: "Voltar", the move, the turn ending, a creature leaving. The mark comes off when it is
   * still set and the window still open, and the clock of the watch stops.
   */
  end(): void {
    if (this.current() && !this.gone()) {
      void this.mark?.leave();
    }
    this.current.set(null);
    this.watch?.destroy();
    this.watch = null;
    this.mark = null;
  }
}
