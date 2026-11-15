import {
  computed,
  effect,
  signal,
  untracked,
  type Signal,
  type WritableSignal,
} from '@angular/core';

import type { Combatant, Encounter } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { MountOptions } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import type { Square } from '../../../../core/combat/combat-grid';
import {
  dismountCard,
  dismountLead,
  dismountRefusalText,
  mountedDetail,
  mountedNotice,
  mountRows,
  nearestLanding,
  rideDetail,
} from '../../../../core/combat/mount-plan';
import { type SquareVerdict } from '../../../../core/combat/move-plan';
import type { Reach } from '../../../../shared/combat-map/combat-map';
import { metersFixed } from '../../../../core/units';
import type { Segment } from '../../../../shared/segmented/segmented';
import type { MoveSummary } from './move-status';

/** What the mount part of the "Mover" page reads from the page. */
export interface MountRideDeps {
  readonly encounter: Signal<Encounter>;
  readonly own: Signal<Combatant | null | undefined>;
  readonly mounts: Signal<MountOptions | undefined>;
  readonly readOnly: Signal<boolean>;
  readonly origin: Signal<Square>;
  readonly chosen: WritableSignal<Square | null>;
}

const IDLE: MoveSummary = { kind: 'idle', title: '', detail: '', warning: '', trap: '' };

/**
 * The mount side of the "Mover" page (RN-21): the rider that sits on a mount rides on ("Cavalgar": the mount's own movement,
 * the pair goes along) or picks where to get off ("Desmontar"). Every number and refusal is the server's
 * (`TurnOptions.mount`); this class only chooses which of them the page shows, and words them.
 */
export class MountRide {
  /** Mounted: the rider rides on or picks where to get off. */
  readonly mode = signal<'ride' | 'dismount'>('ride');
  /** The mount the mover sits on, as the viewer sees it. */
  readonly mountedOn = computed(() => {
    const id = this.deps.own()?.mountedOnId;
    return id ? this.deps.encounter().combatants.find((c) => c.id === id) : undefined;
  });
  /** The mount obeys its rider: the rider's walk is the mount's. */
  readonly controlled = computed(() => this.mountedOn()?.mountKind === 'controlled');
  /** Picking where to get off (an independent mount only lets the rider do that, on the rider's own turn). */
  readonly dismounting = computed(
    () => !!this.mountedOn() && (this.mode() === 'dismount' || !this.controlled()),
  );
  /** Walking on the mount: the page speaks of the mount's movement. */
  readonly riding = computed(() => !!this.mountedOn() && !this.dismounting());
  /** Whose movement the numbers are: the mount's while it carries its rider, else the mover's own. */
  readonly counter = computed(() => (this.riding() ? this.mountedOn() : this.deps.own()));
  readonly landing = computed<readonly Square[]>(() =>
    (this.deps.mounts()?.landing ?? []).map((s) => ({ col: s.col, row: s.row })),
  );
  /** Why the segments are off, one line each. */
  readonly why = computed(() =>
    [
      this.controlled() ? '' : 'Montaria independente: anda na vez dela.',
      this.dismountWhy(),
    ].filter(Boolean),
  );
  readonly modes = computed<readonly Segment<'ride' | 'dismount'>[]>(() => [
    { value: 'ride', label: 'Cavalgar', icon: 'pets', disabled: !this.controlled() },
    {
      value: 'dismount',
      label: 'Desmontar',
      icon: 'file_download',
      disabled: this.dismountWhy() !== '',
    },
  ]);
  readonly line = computed(() => {
    const mount = this.mountedOn()?.label;
    return mount
      ? { notice: mountedNotice(mount), detail: mountedDetail(this.deps.own()?.label ?? '') }
      : null;
  });
  /** The rows of the "Montaria" panel: none when mounted, or on the read-only map. */
  readonly rows = computed(() =>
    this.deps.readOnly() || this.mountedOn()
      ? []
      : mountRows(
          this.deps.mounts()?.mounts ?? [],
          this.deps.encounter().combatants,
          this.deps.origin(),
        ),
  );
  /** The page's lead line while picking where to get off; unset otherwise. */
  readonly lead = computed(() =>
    this.dismounting()
      ? dismountLead(this.deps.mounts()?.dismountCostDft ?? 0, this.deps.own()?.label ?? '')
      : undefined,
  );
  /** The name of the mount that carries the rider, for the walk's lead line. */
  readonly carrier = computed(() => (this.riding() ? this.mountedOn()?.label : undefined));
  /** The squares the rider can land on, as the map's reach; unset unless picking where to get off. */
  readonly reach = computed<Reach | undefined>(() =>
    this.dismounting() && !this.deps.readOnly()
      ? { origin: this.deps.origin(), leftDft: 0, squares: this.landing() }
      : undefined,
  );
  readonly goIcon = computed(() => (this.dismounting() ? 'file_download' : undefined));
  readonly goLabel = computed(() => (this.dismounting() ? 'Desmontar aqui' : undefined));
  /** The card of a chosen landing square. */
  private readonly card = computed<MoveSummary>(() => {
    const to = this.deps.chosen();
    const m = this.deps.mounts();
    if (!to || !m || !this.onLanding(to)) {
      return IDLE;
    }
    const own = this.deps.own();
    const card = dismountCard(
      m.dismountCostDft,
      own?.movementLeftDft ?? 0,
      own?.label ?? '',
      this.controlled(),
    );
    return { ...IDLE, kind: 'ok', title: card.title, detail: card.detail };
  });
  /** The frame on the chosen landing square, with what getting off costs. */
  readonly frame = computed(() => {
    const to = this.deps.chosen();
    const cost = this.deps.mounts()?.dismountCostDft ?? 0;
    if (!this.dismounting()) {
      return undefined;
    }
    return to && this.onLanding(to)
      ? { square: to, refused: false, label: metersFixed(cost / 10) }
      : null;
  });
  private readonly dismountWhy = computed(() => {
    const m = this.deps.mounts();
    return m
      ? dismountRefusalText(
          m.dismountRefusal,
          m.dismountCostDft,
          m.missingDft,
          this.deps.own()?.label ?? '',
        )
      : '';
  });

  /** Build it in an injection context: getting on or off changes whose move this is, and the square picked before means nothing. */
  constructor(private readonly deps: MountRideDeps) {
    let before: string | undefined;
    effect(() => {
      const now = this.mountedOn()?.id ?? '';
      untracked(() => {
        if (before !== undefined && before !== now) {
          this.deps.chosen.set(null);
          this.mode.set('ride');
        }
        before = now;
      });
    });
  }

  /** "Cavalgar" or "Desmontar": the square picked for the other one is dropped. */
  pick(mode: 'ride' | 'dismount'): void {
    this.mode.set(mode);
    this.deps.chosen.set(null);
  }

  /** Getting off lands only on the squares the server lists. */
  allows(square: Square): boolean {
    return !this.dismounting() || this.onLanding(square);
  }

  /** The chosen landing square can be confirmed; unset when the rider is not getting off. */
  canLand(busy: boolean): boolean | undefined {
    const to = this.deps.chosen();
    return this.dismounting() ? !busy && !!to && this.onLanding(to) : undefined;
  }

  onLanding(to: Square): boolean {
    return this.landing().some((s) => s.col === to.col && s.row === to.row);
  }

  /** The arrows go to the nearest square the rider can land on. */
  aim(to: Square): Square {
    return this.dismounting() ? (nearestLanding(to, this.landing()) ?? to) : to;
  }

  /** The walk's card when the mount carries the rider: what is left for the mount, and for getting off. */
  rideCard(walk: MoveSummary, verdict: SquareVerdict | null): MoveSummary {
    if (this.dismounting()) {
      return this.card();
    }
    const mount = this.mountedOn();
    const own = this.deps.own();
    if (!this.riding() || !mount || walk.kind !== 'ok' || verdict?.kind !== 'ok') {
      return walk;
    }
    return {
      ...walk,
      detail: rideDetail(
        verdict.square.leftAfterDft,
        mount.label,
        own?.movementLeftDft ?? 0,
        own?.label ?? '',
      ),
    };
  }
}
