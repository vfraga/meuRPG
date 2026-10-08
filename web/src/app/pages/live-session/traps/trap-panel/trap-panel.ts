import { DOCUMENT } from '@angular/common';
import {
  Component,
  DestroyRef,
  computed,
  effect,
  inject,
  input,
  signal,
  untracked,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';

import { type MapPoint, TrapState } from '../../../../../gen/meurpg/maps/v1/maps_pb';
import { EncounterStatus, type TrapFiring } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { CombatState } from '../../../../core/combat/combat-state';
import { focusWithRing } from '../../../../core/creatures/focus-ring';
import { MapsClient } from '../../../../core/maps/maps-client';
import type { MapState } from '../../../../core/maps/map-state';
import type { CluePlayer } from '../../../../core/maps/scene-clues';
import { listNames } from '../../../../core/maps/scene-clues';
import { trapMapErrorMessage } from '../../../../core/traps/trap-errors';
import type { TrapBoard } from '../../../../core/traps/trap-board';
import { clockOf, trapPoints, trapVisibility } from '../../../../core/traps/trap-text';
import type { PickRow } from '../../../../shared/person-pick/person-pick';
import { isPlayer } from '../../../../core/combat/combat-view';
import type { VitalsVm } from '../../live-session.types';
import { TrapActivityList } from '../trap-activity/trap-activity';
import { TrapCard } from '../trap-card/trap-card';
import { TrapDamages } from '../trap-damages/trap-damages';
import { openTrapFire } from '../trap-fire-sheet/trap-fire-sheet';
import { openTrapReveal } from '../trap-reveal-sheet/trap-reveal-sheet';

/**
 * "Armadilhas do mapa" on the master's session page (E9-08 1 to 5, MR-035, RN-10): the card of each trap of
 * the current map (state, who knows it, the DCs, "Quem notaria" and the actions), the damage a fired trap
 * did to a player's character, which waits for the master, and the "Registro" of what traps did. Every
 * number, who stands in an area and what a trap does is the server's: the panel reads them and calls
 * `RevealTrap`, `FireTrap`, `DisarmTrap`; the map and the board are read again after each. One card is
 * open at a time (the first armed one to begin with); the panel exists while the map has a trap, or
 * while a trap's damage waits.
 */
@Component({
  selector: 'app-trap-panel',
  imports: [MatIconModule, TrapActivityList, TrapCard, TrapDamages],
  templateUrl: './trap-panel.html',
  styleUrl: './trap-panel.scss',
})
export class TrapPanel {
  private readonly api = inject(MapsClient);
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);
  private readonly destroyRef = inject(DestroyRef);
  private readonly document = inject(DOCUMENT);

  readonly campaignId = input.required<string>();
  readonly state = input.required<MapState>();
  readonly board = input.required<TrapBoard>();
  readonly players = input<readonly CluePlayer[]>([]);
  readonly vitals = input<readonly VitalsVm[]>([]);
  /** The combat on screen: the damage of a trap that fired in it is applied through it. */
  readonly combat = input<CombatState | null>(null);
  /** The damage that waits is drawn here; in a combat the combat's own page draws it, and the panel does not repeat it. */
  readonly withDamages = input(true);

  protected readonly mapId = computed(() => this.state().map()?.id ?? '');
  protected readonly traps = computed(() => trapPoints(this.state().points()));
  protected readonly armedCount = computed(
    () => this.traps().filter((p) => (p.trap?.state ?? TrapState.ARMED) === TrapState.ARMED).length,
  );
  protected readonly openId = signal<string | null>(null);
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  protected readonly announcement = signal('');
  /** A combat runs on this map: who can be caught are its combatants. */
  private readonly encounter = computed(() => {
    const e = this.combat()?.encounter();
    return e && e.status !== EncounterStatus.ENDED && e.mapId === this.mapId() ? e : null;
  });

  protected readonly count = computed(() => {
    const n = this.armedCount();
    return n === 0 ? 'nenhuma armada' : n === 1 ? '1 armada' : `${n} armadas`;
  });

  constructor() {
    // The first armed card is open to begin with; a card that went away is not open any more.
    effect(() => {
      const traps = this.traps();
      const open = untracked(() => this.openId());
      if (open === null || !traps.some((p) => p.id === open)) {
        const first =
          traps.find((p) => (p.trap?.state ?? TrapState.ARMED) === TrapState.ARMED) ?? traps[0];
        untracked(() => this.openId.set(first?.id ?? null));
      }
    });
    // "Quem notaria" is read for the card that is open, and again whenever the board is.
    let watched: string | null = null;
    effect(() => {
      const id = this.openId();
      untracked(() => {
        if (watched !== null) {
          this.board().unwatchNoticers(watched);
        }
        watched = id;
        if (id !== null) {
          void this.board().watchNoticers(id);
        }
      });
    });
    this.destroyRef.onDestroy(() => {
      if (watched !== null) {
        this.board().unwatchNoticers(watched);
      }
    });
  }

  protected firedAt(p: MapPoint): Date | null {
    return clockOf(this.board().firingOf(p.id)?.at);
  }

  protected canExtend(p: MapPoint): boolean {
    return this.board().firingOf(p.id) !== null && this.encounter() === null;
  }

  protected toggle(id: string): void {
    this.openId.update((open) => (open === id ? null : id));
  }

  protected reveal(p: MapPoint): void {
    // The card's button is only dimmed (it keeps focus), so a click still comes: nothing to reveal to a table that sees it.
    if (this.busy() || trapVisibility(p).kind === 'all') {
      return;
    }
    const opener = this.document.activeElement as HTMLElement | null;
    openTrapReveal(this.dialog, this.bottomSheet, {
      campaignId: this.campaignId(),
      mapId: this.mapId(),
      point: p,
      players: this.players(),
    })
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe((point) => {
        focusWithRing(opener);
        if (point) {
          this.error.set('');
          this.state().upsertPoint(point);
          this.announcement.set(`${point.name}: revelada.`);
          void this.board().refresh();
        }
      });
  }

  /** Who can be caught: the combatants of a combat on this map, otherwise the characters with a token on it; `null`
   * while the trap's "Quem notaria" is still being read. */
  private targetsFor(p: MapPoint): readonly PickRow[] | null {
    const e = this.encounter();
    if (e) {
      return e.combatants.map((c) => ({
        id: c.id,
        name: c.label,
        sub: isPlayer(c) ? 'Personagem' : 'NPC',
      }));
    }
    const read = this.board().noticers().get(p.id);
    if (!read) {
      return this.board().noticersFailed().has(p.id) ? [] : null;
    }
    return read.noticers
      .filter((n) => n.onMap)
      .map((n) => ({
        id: n.characterId,
        name: n.characterName,
        sub: n.inRange ? 'Perto da armadilha, até 3 m' : 'Longe da armadilha',
      }));
  }

  protected fire(p: MapPoint, extend = false): void {
    if (this.busy()) {
      return;
    }
    const firing = extend ? this.board().firingOf(p.id) : null;
    const opener = this.document.activeElement as HTMLElement | null;
    // The list follows the board: the dialog can open before the trap's "Quem notaria" arrives (a snapshot taken
    // here stayed empty, "Ninguém com token no mapa", when the master was quicker than the read).
    const stop = this.encounter() ? () => undefined : this.board().watchWhileFiring(p.id);
    openTrapFire(this.dialog, this.bottomSheet, {
      campaignId: this.campaignId(),
      mapId: this.mapId(),
      point: p,
      targets: computed(() => this.targetsFor(p)),
      extendFiringId: firing?.id ?? '',
      targetsFailed: computed(
        () => this.encounter() === null && this.board().noticersFailed().has(p.id),
      ),
    })
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe((done) => {
        stop();
        focusWithRing(opener);
        if (done) {
          this.fired(p, done, extend);
        }
      });
  }

  private fired(p: MapPoint, firing: TrapFiring, extend: boolean): void {
    this.error.set('');
    const names = firing.caught.map((c) => c.targetLabel).filter(Boolean);
    this.announcement.set(
      names.length > 0
        ? `${p.name} ${extend ? 'pegou também' : 'disparou e pegou'} ${listNames(names)}.`
        : `${p.name} disparou. Ninguém estava na área.`,
    );
    void this.state().refresh();
    void this.board().refresh();
    // The combat on screen (a trap fired in it) reads itself again on `encounter_changed`.
  }

  protected async disarm(p: MapPoint): Promise<void> {
    if (this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const point = await this.api.disarmTrap(this.campaignId(), this.mapId(), p.id);
      this.state().upsertPoint(point);
      this.announcement.set(`${point.name}: desarmada.`);
      void this.board().refresh();
    } catch (err) {
      this.error.set(trapMapErrorMessage(err, 'desarmar a armadilha'));
    } finally {
      this.busy.set(false);
    }
  }
}
