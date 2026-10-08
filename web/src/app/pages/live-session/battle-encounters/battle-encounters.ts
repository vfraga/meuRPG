import {
  Component,
  computed,
  effect,
  inject,
  input,
  output,
  signal,
  untracked,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import { RouterLink } from '@angular/router';

import { MapPointKind } from '../../../../gen/meurpg/maps/v1/maps_pb';
import type { Encounter } from '../../../../gen/meurpg/play/v1/combat_pb';
import type { EncounterEvaluation } from '../../../../gen/meurpg/play/v1/encounters_pb';
import { EncountersClient, type SavedEncounter } from '../../../core/encounters/encounters-client';
import { EncounterWarning } from '../../../../gen/meurpg/play/v1/encounters_pb';
import {
  GUIDE_LABEL,
  GUIDE_CAVEAT,
  encounterErrorMessage,
  headline,
  warningLines,
} from '../../../core/encounters/encounter-text';
import { formatInt, tight } from '../../../core/format/text';
import type { MapState } from '../../../core/maps/map-state';
import { CreatureArt } from '../../../shared/creatures/creature-art';
import { PHONE_QUERY, mediaQuery } from '../../../shared/map-view/media-query';
import { combatMapInfo, openStartCombat } from '../combat/combat-launch';

/** A battle point of the session's map that keeps an encounter, read against the party of today. */
interface Kept {
  readonly pointId: string;
  readonly name: string;
  readonly encounter: SavedEncounter;
  readonly evaluation: EncounterEvaluation | null;
  /** Saved creatures that are no longer in the SRD (the content changed): the combat cannot start with them. */
  readonly unknown: readonly string[];
}

/**
 * "Encontro guardado" (MR-043, E10-09 state 7): on the master's session page, a battle point of the current map that keeps an
 * encounter shows it (the band against the party of today, the creatures with their ND and XP) with "Começar este combate",
 * the one filled button of the page. It opens "Iniciar combate" already filled from the point (the name, the monsters, the hit
 * points mode and hidden), which the master can still change; on confirm the combat starts from the point with the monsters
 * in, under one idempotency key. Only the master gets any of it: a player's session never reads a saved encounter (RN-10).
 *
 * A saved creature the SRD no longer has (a content change) is said, and the button waits: the builder takes it out.
 */
@Component({
  selector: 'app-battle-encounters',
  imports: [CreatureArt, MatButtonModule, MatIconModule, RouterLink],
  templateUrl: './battle-encounters.html',
  styleUrl: './battle-encounters.scss',
})
export class BattleEncounters {
  private readonly api = inject(EncountersClient);
  private readonly dialog = inject(MatDialog);
  private readonly phone = mediaQuery(PHONE_QUERY);

  readonly campaignId = input.required<string>();
  readonly state = input.required<MapState>();
  /** The combat that was started: the page shows it. */
  readonly started = output<Encounter>();

  protected readonly kept = signal<readonly Kept[]>([]);
  protected readonly error = signal('');
  private readonly opening = signal(false);
  protected readonly guide = GUIDE_LABEL;
  protected readonly caveat = GUIDE_CAVEAT;
  protected readonly format = formatInt;

  private seq = 0;
  /** Which battle points the map has, as a key: the points are read again when it changes, not at every reveal. */
  private readonly battleKey = computed(
    () =>
      `${this.state().map()?.id ?? ''}|${this.state()
        .points()
        .filter((p) => p.kind === MapPointKind.BATTLE)
        .map((p) => p.id)
        .join(',')}`,
  );

  constructor() {
    effect(() => {
      const key = this.battleKey();
      untracked(() => void this.load(key));
    });
  }

  /** The warning the server already sends for a saved encounter that does not fit one combat of 40 with today's party. */
  protected tooMany(k: Kept): boolean {
    return k.evaluation?.warnings.includes(EncounterWarning.TOO_MANY) ?? false;
  }

  protected tooManyText(k: Kept): string {
    return (
      warningLines(k.evaluation!).find((w) => w.kind === EncounterWarning.TOO_MANY)?.text ?? ''
    );
  }

  /** The builder on this point's map and point, where the creature that left the SRD is taken out. */
  protected readonly mapId = computed(() => this.state().map()?.id ?? '');

  protected head(k: Kept): string {
    return k.evaluation ? headline(k.evaluation) : '';
  }

  protected lines(k: Kept): { key: string; text: string; nd: string; xp: string; type: string }[] {
    return (k.evaluation?.lines ?? []).map((l) => ({
      key: l.creature?.key ?? '',
      type: l.creature?.type ?? '',
      text: `${l.count} × ${l.creature?.namePt ?? ''}`,
      nd: `ND ${l.creature?.challengeRating ?? ''}`,
      xp: tight(`${formatInt(l.subtotalXp)} XP`),
    }));
  }

  private async load(key: string): Promise<void> {
    const mine = ++this.seq;
    const mapId = this.state().map()?.id;
    this.error.set('');
    if (!mapId || key.split('|')[1] === '') {
      this.kept.set([]);
      return;
    }
    try {
      const list = await this.api.list(this.campaignId(), mapId);
      const points = new Map(
        this.state()
          .points()
          .map((p) => [p.id, p]),
      );
      const reads = await Promise.all(
        list
          .filter((k) => points.has(k.mapPointId))
          .map(async (k): Promise<Kept | null> => {
            const read = await this.api.get(this.campaignId(), k.mapPointId);
            return read.encounter
              ? {
                  pointId: k.mapPointId,
                  name: points.get(k.mapPointId)!.name,
                  encounter: read.encounter,
                  evaluation: read.evaluation,
                  unknown: read.unknownKeys,
                }
              : null;
          }),
      );
      if (mine === this.seq) {
        this.kept.set(reads.filter((r): r is Kept => r !== null));
      }
    } catch (err) {
      if (mine === this.seq) {
        this.kept.set([]);
        this.error.set(encounterErrorMessage(err, 'read'));
      }
    }
  }

  /** The encounter a point keeps right now: another tab can have changed it since the card was read. */
  private async readOne(k: Kept): Promise<Kept | null> {
    const read = await this.api.get(this.campaignId(), k.pointId);
    return read.encounter
      ? { ...k, encounter: read.encounter, evaluation: read.evaluation, unknown: read.unknownKeys }
      : null;
  }

  /** "Começar este combate": "Iniciar combate" opens filled from the point, as the point keeps it at this moment. */
  protected async begin(kept: Kept): Promise<void> {
    if (kept.unknown.length > 0 || this.opening()) {
      // The button is only dimmed (it keeps focus), so the click still comes: a creature the SRD lost cannot start.
      return;
    }
    this.opening.set(true);
    this.error.set('');
    let k: Kept | null;
    try {
      k = await this.readOne(kept);
    } catch (err) {
      this.error.set(encounterErrorMessage(err, 'read'));
      return;
    } finally {
      this.opening.set(false);
    }
    if (!k) {
      // It was cleared meanwhile: the card goes away.
      this.kept.update((list) => list.filter((x) => x.pointId !== kept.pointId));
      return;
    }
    const fresh = k;
    this.kept.update((list) => list.map((x) => (x.pointId === fresh.pointId ? fresh : x)));
    if (k.unknown.length > 0) {
      return;
    }
    const map = combatMapInfo(this.state());
    const byKey = new Map(
      (k.evaluation?.lines ?? []).map((l) => [l.creature?.key, l.creature?.namePt ?? '']),
    );
    openStartCombat(this.dialog, this.phone(), {
      campaignId: this.campaignId(),
      mode: 'start',
      map,
      saved: {
        pointId: k.pointId,
        pointName: k.name,
        hp: k.encounter.hp,
        hidden: k.encounter.hidden,
        groups: k.encounter.monsters.map((m) => ({
          key: m.creatureKey,
          namePt: byKey.get(m.creatureKey) ?? m.creatureKey,
          count: m.count,
          name: m.name,
        })),
      },
    }).subscribe((encounter) => {
      if (encounter) {
        this.started.emit(encounter);
      }
    });
  }
}
