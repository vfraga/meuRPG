import {
  afterNextRender,
  Component,
  computed,
  effect,
  ElementRef,
  inject,
  Injector,
  signal,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { RouterLink } from '@angular/router';

import { type MapPoint, MapPointKind } from '../../../../gen/meurpg/maps/v1/maps_pb';
import type { Map as MapMessage } from '../../../../gen/meurpg/maps/v1/maps_pb';
import type { EncounterEvaluation } from '../../../../gen/meurpg/play/v1/encounters_pb';
import type { MonsterGroupSpec } from '../../../core/combat/combat-client';
import { EncountersClient, type PartyNpcSpec } from '../../../core/encounters/encounters-client';
import {
  GUIDE_LABEL,
  encounterErrorMessage,
  headline,
} from '../../../core/encounters/encounter-text';
import { formatInt } from '../../../core/format/text';
import { MapsClient } from '../../../core/maps/maps-client';
import { mapErrorMessage } from '../../../core/maps/map-errors';
import { SheetFrame } from '../../live-session/combat/sheet-frame/sheet-frame';
import { injectSheet } from '../../live-session/combat/sheet-host';

/** What the builder hands the sheet. */
export interface SaveData {
  readonly campaignId: string;
  readonly entries: readonly MonsterGroupSpec[];
  readonly party: readonly PartyNpcSpec[];
  /** The last measure, for the line under the title. */
  readonly evaluation: EncounterEvaluation | null;
  /** The map and the point to start on (a link from a point of the editor), or empty. */
  readonly mapId?: string;
  readonly pointId?: string;
}

/** What closes the sheet when the encounter was kept: where, and how it measures. */
export interface SaveResult {
  readonly mapId: string;
  readonly pointId: string;
  readonly pointName: string;
  readonly evaluation: EncounterEvaluation | null;
}

/** A battle point of the chosen map, with whether it already keeps an encounter and how many creatures that has. */
interface PointRow {
  readonly id: string;
  readonly name: string;
  readonly kept: number | null;
}

/**
 * "Guardar no ponto de batalha" (MR-043, E10-09 state 6): the one way to keep an encounter. The master picks the map and
 * a battle point of it; a point that already keeps one says so ("Já guarda um encontro: guardar de novo troca o anterior"),
 * and choosing it asks again in place before it replaces ("Trocar o encontro guardado?", the focus on "Voltar"). The
 * encounter, the creatures and the XP stay hidden from the players until the master reveals them; a player never gets the
 * point's encounter (RN-10). A map with no battle point says so and offers "Abrir o mapa".
 */
@Component({
  selector: 'app-save-sheet',
  imports: [MatButtonModule, MatIconModule, RouterLink, SheetFrame],
  templateUrl: './save-sheet.html',
  styleUrl: './save-sheet.scss',
})
export class SaveSheet {
  private readonly maps = inject(MapsClient);
  private readonly api = inject(EncountersClient);
  private readonly sheet = injectSheet<SaveData, SaveResult>();
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly guide = GUIDE_LABEL;

  protected readonly state = signal<'loading' | 'ready' | 'error'>('loading');
  protected readonly mapList = signal<readonly MapMessage[]>([]);
  protected readonly mapId = signal('');
  protected readonly points = signal<readonly PointRow[] | null>(null);
  protected readonly pointId = signal('');
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');
  protected readonly asking = signal(false);

  protected readonly line = computed(() => {
    const ev = this.data.evaluation;
    return ev ? `${headline(ev)} · ${formatInt(ev.creatureCount)} criaturas` : '';
  });
  protected readonly chosen = computed(
    () => this.points()?.find((p) => p.id === this.pointId()) ?? null,
  );
  protected readonly mapName = computed(
    () => this.mapList().find((m) => m.id === this.mapId())?.name ?? '',
  );
  protected readonly blocked = computed(() => {
    if (this.state() !== 'ready' || this.points() === null) {
      return 'Lendo os mapas.';
    }
    if (this.data.entries.length === 0) {
      return 'Ponha pelo menos uma criatura no encontro.';
    }
    if (this.pointId() === '') {
      return 'Escolha um ponto de batalha.';
    }
    return '';
  });

  private seq = 0;

  constructor() {
    void this.load();
  }

  private async load(): Promise<void> {
    try {
      const list = await this.maps.list(this.data.campaignId);
      this.mapList.set(list);
      const first =
        this.data.mapId && list.some((m) => m.id === this.data.mapId)
          ? this.data.mapId
          : (list[0]?.id ?? '');
      this.state.set('ready');
      if (first) {
        await this.chooseMap(first, this.data.pointId ?? '');
      } else {
        this.points.set([]);
      }
    } catch (err) {
      this.error.set(mapErrorMessage(err, 'abrir os mapas'));
      this.state.set('error');
    }
  }

  /** A map was picked: its battle points, and which of them keep an encounter. */
  protected async chooseMap(mapId: string, pointId = ''): Promise<void> {
    const mine = ++this.seq;
    this.mapId.set(mapId);
    this.points.set(null);
    this.pointId.set('');
    this.asking.set(false);
    this.error.set('');
    try {
      const [map, kept] = await Promise.all([
        this.maps.get(this.data.campaignId, mapId),
        this.api.list(this.data.campaignId, mapId),
      ]);
      if (mine !== this.seq) {
        return;
      }
      const keeps = new Map(kept.map((k) => [k.mapPointId, k.creatureCount]));
      const rows = map.points
        .filter((p: MapPoint) => p.kind === MapPointKind.BATTLE)
        .map((p: MapPoint) => ({ id: p.id, name: p.name, kept: keeps.get(p.id) ?? null }));
      this.points.set(rows);
      if (pointId && rows.some((r) => r.id === pointId)) {
        this.pointId.set(pointId);
      } else if (rows.length === 1) {
        this.pointId.set(rows[0].id);
      }
    } catch (err) {
      if (mine === this.seq) {
        this.points.set([]);
        this.error.set(encounterErrorMessage(err, 'read'));
      }
    }
  }

  protected pick(id: string): void {
    this.pointId.set(id);
    this.asking.set(false);
  }

  /** "Guardar": a point with an encounter asks first, in place. */
  protected async save(): Promise<void> {
    if (this.busy() || this.blocked() !== '') {
      return;
    }
    if (this.chosen()?.kept != null && !this.asking()) {
      this.asking.set(true);
      // The question is drawn on the next render: it comes into view and "Voltar" takes the focus.
      afterNextRender(
        () => {
          const ask = this.host.nativeElement.querySelector<HTMLElement>('.ask');
          ask?.scrollIntoView?.({ block: 'nearest' });
          this.host.nativeElement.querySelector<HTMLElement>('[data-cancel]')?.focus();
        },
        { injector: this.injector },
      );
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const saved = await this.api.save(
        this.data.campaignId,
        this.pointId(),
        { monsters: this.data.entries, hp: 'average', hidden: true },
        this.data.party,
      );
      this.sheet.close({
        mapId: this.mapId(),
        pointId: this.pointId(),
        pointName: this.chosen()?.name ?? '',
        evaluation: saved.evaluation ?? null,
      });
    } catch (err) {
      this.asking.set(false);
      this.error.set(encounterErrorMessage(err, 'save'));
    } finally {
      this.busy.set(false);
    }
  }

  protected back(): void {
    this.asking.set(false);
  }

  protected close(): void {
    this.sheet.close();
  }
}
