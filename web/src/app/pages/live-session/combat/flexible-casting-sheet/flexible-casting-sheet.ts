import {
  ChangeDetectionStrategy,
  Component,
  type Signal,
  computed,
  effect,
  inject,
  signal,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { circleLabel } from '../../../../core/combat/combat-grid';
import type { CombatState } from '../../../../core/combat/combat-state';
import { ActionKey } from '../../../../core/connect/idempotency';
import {
  convertPreview,
  convertRows,
  createPreview,
  createRows,
  fullText,
} from '../../../../core/resources/flexible-casting';
import { SORCERY_POINTS_RESOURCE, pointsText, poolOf } from '../../../../core/resources/pools';
import { ResourceClient } from '../../../../core/resources/resources-client';
import { classResourceErrorMessage } from '../../../../core/resources/resources-errors';
import { type Segment, Segmented } from '../../../../shared/segmented/segmented';
import { toVitalsVm } from '../../../../core/play/live-session-source.live';
import type { VitalsVm } from '../../../../core/play/live-session.types';
import { type PickRow, ResourcePick } from '../resource-pick/resource-pick';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../../shared/sheet/sheet-host';

/** What the page hands "Conjuração Flexível". */
export interface FlexibleCastingSheetData {
  readonly campaignId: string;
  readonly encounterId: string;
  /** The sorcerer's combatant. */
  readonly actorId: string;
  /** The sorcerer's own vitals: the sorcery points and the slots. */
  readonly vitals: Signal<VitalsVm | null>;
  readonly state: CombatState;
  /** The direction the dialog opens on: the two actions of the table are two rows of the turn. */
  readonly direction: Direction;
}

export type Direction = 'create' | 'convert';

const DIRECTIONS: readonly Segment<Direction>[] = [
  { value: 'create', label: 'Pontos → espaço' },
  { value: 'convert', label: 'Espaço → pontos' },
];

/**
 * "Conjuração Flexível" (PM-07c 10; SRD 5.1, Sorcerer): as a bonus action the sorcerer turns sorcery points into a
 * spell slot of level 1 to 5 ("Pontos → espaço", the cost of each level beside it; the ones the points cannot pay
 * are grey, dashed, with the reason written; the slot created vanishes on a long rest), or expends a free slot for as
 * many points as its level ("Espaço → pontos"). Destroying a slot is irreversible, so that button is the app's outlined
 * danger action. With the points at the maximum (the sorcerer's level) the conversion is refused, with the reason
 * written and the button grey; the server refuses it the same way. One key per request, made again once it worked.
 */
@Component({
  selector: 'app-flexible-casting-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatButtonModule, MatIconModule, ResourcePick, Segmented, SheetFrame],
  template: `
    <app-sheet-frame
      title="Conjuração Flexível"
      [subtitle]="subtitle()"
      [phone]="inSheet"
      (closed)="close()"
    >
      <div class="body">
        @if (error()) {
          <div class="mr-notice mr-notice--danger" role="alert">
            <mat-icon aria-hidden="true">error</mat-icon>
            <p>{{ error() }}</p>
          </div>
        }
        @if (done(); as d) {
          <div class="res" role="status" aria-live="polite">
            <span class="pill"><mat-icon aria-hidden="true">check</mat-icon>{{ d.pill }}</span>
            @for (line of d.lines; track $index) {
              <p class="what">{{ line }}</p>
            }
            <p class="small">Ação bônus usada.</p>
          </div>
        } @else {
          <app-segmented
            label="Direção"
            [segments]="directions"
            [value]="direction()"
            (choose)="chooseDirection($event)"
          />
          <app-resource-pick
            [label]="direction() === 'create' ? 'Espaço a criar' : 'Espaço a converter'"
            [rows]="rows()"
            [chosen]="level()"
            [legendHidden]="true"
            empty="Você não tem espaço de magia livre para converter."
            (pick)="chooseLevel($event)"
          />
          @if (refusal()) {
            <div class="mr-notice mr-notice--danger" role="status">
              <mat-icon aria-hidden="true">block</mat-icon>
              <p><strong>{{ refusal() }}</strong></p>
            </div>
          } @else if (preview()) {
            <p class="what" role="status" aria-live="polite">{{ preview() }}</p>
          }
        }
      </div>
      <div foot>
        @if (done()) {
          <button matButton="outlined" type="button" class="pair__btn" (click)="close()">Fechar</button>
        } @else {
          <div class="pair">
            <button
              matButton="outlined"
              type="button"
              class="pair__btn"
              [class.pair__btn--danger]="direction() === 'convert'"
              [disabled]="!ready() || busy()"
              disabledInteractive
              (click)="go()"
            >
              {{ doLabel() }}
            </button>
            <button matButton="outlined" type="button" class="pair__btn" [disabled]="busy()" (click)="close()">
              Cancelar
            </button>
          </div>
        }
      </div>
    </app-sheet-frame>
  `,
  styleUrl: '../resource-pick/resource-sheet.scss',
})
export class FlexibleCastingSheet {
  private readonly api = inject(ResourceClient);
  private readonly sheet = injectSheet<FlexibleCastingSheetData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly directions = DIRECTIONS;

  protected readonly direction = signal<Direction>(this.data.direction);

  /** The newest copy of the vitals: the page's, or the answer of the last call, whichever has the larger revision. */
  private readonly answered = signal<VitalsVm | null>(null);
  private readonly vitals = computed(() => {
    const own = this.data.vitals();
    const answer = this.answered();
    return answer && (!own || answer.revision > own.revision) ? answer : own;
  });
  protected readonly points = computed(
    () => poolOf(this.vitals()?.resources, SORCERY_POINTS_RESOURCE) ?? { left: 0, total: 0 },
  );
  protected readonly subtitle = computed(
    () => `Ação bônus · Pontos de Feitiçaria: ${this.points().left} de ${this.points().total}`,
  );

  private readonly flex = computed(() =>
    this.direction() === 'create'
      ? createRows(this.points())
      : convertRows(this.vitals()?.spellSlots ?? []),
  );
  protected readonly rows = computed<PickRow[]>(() =>
    this.flex().map((r) => ({
      id: String(r.level),
      title: r.title,
      sub: r.sub,
      blocked: r.blocked,
    })),
  );
  private readonly picked = signal<number | null>(null);
  /** The level picked, as the id of its card; the first one that can be chosen until the player picks another. */
  protected readonly level = computed(() => {
    const rows = this.flex();
    const own = rows.find((r) => r.level === this.picked() && !r.blocked);
    const level = (own ?? rows.find((r) => !r.blocked))?.level;
    return level === undefined ? null : String(level);
  });
  private readonly levelNumber = computed(() => Number(this.level()));

  /** The conversion would lose points: said before asking, and the button waits. */
  protected readonly refusal = computed(() =>
    this.direction() === 'convert' &&
    this.points().total > 0 &&
    this.points().left >= this.points().total &&
    this.level() !== null
      ? fullText(this.points())
      : '',
  );
  protected readonly preview = computed(() => {
    if (this.level() === null) {
      return '';
    }
    return this.direction() === 'create'
      ? createPreview(this.levelNumber(), this.points())
      : convertPreview(this.levelNumber(), this.points());
  });
  protected readonly ready = computed(() => this.level() !== null && !this.refusal());
  protected readonly doLabel = computed(() => {
    const level = this.level();
    if (level === null) {
      return this.direction() === 'create' ? 'Criar um espaço' : 'Converter um espaço';
    }
    return this.direction() === 'create'
      ? `Criar o espaço de ${circleLabel(this.levelNumber())}`
      : `Converter o espaço de ${circleLabel(this.levelNumber())}`;
  });

  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');
  protected readonly done = signal<{ pill: string; lines: readonly string[] } | null>(null);
  /** One key per request (direction and level): the same values again are a retry, others another request. */
  private readonly key = new ActionKey();

  /** Another direction lists other rows, so it picks its own first level. */
  protected chooseDirection(direction: Direction): void {
    this.direction.set(direction);
    this.picked.set(null);
    this.error.set('');
  }

  protected chooseLevel(id: string): void {
    this.picked.set(Number(id));
    this.error.set('');
  }

  protected async go(): Promise<void> {
    if (!this.ready() || this.busy()) {
      return;
    }
    const level = this.levelNumber();
    const direction = this.direction();
    // The points as the player saw them when asking: what the answer says is worked out from them.
    const before = this.points();
    this.busy.set(true);
    this.error.set('');
    try {
      const { campaignId, encounterId, actorId } = this.data;
      const key = this.key.keyFor({ direction, level });
      const res =
        direction === 'create'
          ? await this.api.createSpellSlot(campaignId, encounterId, actorId, level, key)
          : await this.api.convertSpellSlot(campaignId, encounterId, actorId, level, key);
      this.key.renew();
      if (res.encounter) {
        this.data.state.apply(res.encounter);
      }
      if (res.vitals) {
        this.answered.set(toVitalsVm(res.vitals));
      }
      this.done.set(
        'cost' in res
          ? {
              pill: 'Espaço criado',
              lines: [
                `Você criou um espaço de ${circleLabel(level)}.`,
                `Gastou ${pointsText(res.cost)}: restam ${Math.max(0, before.left - res.cost)} de ${before.total}. O espaço some no descanso longo.`,
              ],
            }
          : {
              pill: 'Espaço convertido',
              lines: [
                `Você gastou um espaço de ${circleLabel(level)}.`,
                `Ganhou ${pointsText(res.gain)} de feitiçaria: ficam em ${Math.min(before.total, before.left + res.gain)} de ${before.total}.`,
              ],
            },
      );
    } catch (err) {
      this.error.set(classResourceErrorMessage(err, 'usar a Conjuração Flexível'));
    } finally {
      this.busy.set(false);
    }
  }

  protected close(): void {
    if (this.busy()) {
      return;
    }
    this.sheet.close(this.done() !== null);
  }
}
