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

import type { ResourceTarget } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { CombatState } from '../../../../core/combat/combat-state';
import { ActionKey } from '../../../../core/connect/idempotency';
import {
  LAY_ON_HANDS_CURE_COST,
  type TouchResult,
  curePreview,
  firstAmount,
  healPreview,
  reachNote,
  touchResult,
  touchTargetRows,
} from '../../../../core/resources/lay-on-hands';
import { LAY_ON_HANDS_RESOURCE, poolOf } from '../../../../core/resources/pools';
import { type LayOnHandsEffect, ResourceClient } from '../../../../core/resources/resources-client';
import { classResourceErrorMessage } from '../../../../core/resources/resources-errors';
import { CountStepper } from '../../../../shared/count-stepper/count-stepper';
import type { VitalsVm } from '../../../../core/play/live-session.types';
import { type PickRow, ResourcePick } from '../resource-pick/resource-pick';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../../shared/sheet/sheet-host';

/** What the page hands "Cura pelas Mãos". */
export interface LayOnHandsSheetData {
  readonly campaignId: string;
  readonly encounterId: string;
  /** The paladin's combatant. */
  readonly actorId: string;
  /** Who the touch can pick (`GetTurnOptionsResponse.resource_targets`), the paladin itself among them. */
  readonly targets: readonly ResourceTarget[];
  /** The paladin's own vitals: the pool is its `lay_on_hands` resource. */
  readonly vitals: Signal<VitalsVm | null>;
  readonly state: CombatState;
}

/** What the touch does: restore hit points, or one of the two cures. */
type Mode = 'heal' | 'poison' | 'disease';

/**
 * "Cura pelas Mãos" (PM-07c 9 and 9b; SRD 5.1, Paladin): the paladin touches a creature within 5 feet (the paladin
 * itself included) and draws from the pool as an action: it restores up to the amount chosen (1 to what the pool has
 * left), or spends 5 points to cure one disease or neutralize one poison. The dialog lists who the server said can
 * be touched, then what the touch does, with the sentence of what it will cost. The answer is only what the server
 * says: a touch that does nothing (the server does not say why) reads "Sem efeito. Nada acontece." for any target,
 * so the app never tells a player what kind of creature an NPC is (RN-10). The key is made once per touch: a repeated
 * tap or a try after a lost answer never spends twice.
 */
@Component({
  selector: 'app-lay-on-hands-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [CountStepper, MatButtonModule, MatIconModule, ResourcePick, SheetFrame],
  template: `
    <app-sheet-frame
      title="Cura pelas Mãos"
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
        @if (result(); as r) {
          <div class="res" role="status" aria-live="polite">
            <span class="pill" [class.pill--none]="!r.worked">
              <mat-icon aria-hidden="true">{{ r.worked ? 'check' : 'remove' }}</mat-icon>{{ r.pill }}
            </span>
            @for (line of r.lines; track $index) {
              <p class="what">{{ line }}</p>
            }
            <p class="small">Ação usada.</p>
          </div>
        } @else if (pool().total === 0 || pool().left === 0) {
          <div class="mr-notice mr-notice--neutral" role="status">
            <mat-icon aria-hidden="true">info</mat-icon>
            <p><strong>A reserva está vazia.</strong> Ela volta num descanso longo.</p>
          </div>
        } @else {
          <app-resource-pick
            label="Quem tocar"
            [rows]="targetPicks()"
            [chosen]="targetId()"
            empty="Não há ninguém ao alcance do toque."
            (pick)="pickTarget($event)"
          />
          <app-resource-pick
            label="O que a cura faz"
            [rows]="modeRows()"
            [chosen]="mode()"
            (pick)="pickMode($event)"
          />
          @if (mode() === 'heal') {
            <p class="ask" id="amount-l">Quanto curar (você gasta o mesmo da reserva)</p>
            <div class="step">
              <app-count-stepper
                aria-labelledby="amount-l"
                noun="ponto"
                minusLabel="Menos um ponto"
                plusLabel="Mais um ponto"
                valueLabel="{n} pontos"
                [value]="amount()"
                [min]="1"
                [max]="pool().left"
                (valueChange)="asked.set($event)"
              />
              <span class="step__unit">pontos · de 1 a {{ pool().left }}</span>
            </div>
          }
          <p class="what" role="status" aria-live="polite">{{ preview() }}</p>
          <p class="small">{{ reach }}</p>
        }
      </div>
      <div foot>
        @if (result()) {
          <button matButton="outlined" type="button" class="pair__btn" (click)="close()">Fechar</button>
        } @else if (pool().left > 0) {
          <div class="pair">
            <button
              matButton="filled"
              type="button"
              class="pair__btn"
              [disabled]="!ready() || busy()"
              disabledInteractive
              (click)="touch()"
            >
              <mat-icon aria-hidden="true">favorite</mat-icon>{{ doLabel() }}
            </button>
            <button matButton="outlined" type="button" class="pair__btn" [disabled]="busy()" (click)="close()">
              Cancelar
            </button>
          </div>
        } @else {
          <button matButton="outlined" type="button" class="pair__btn" (click)="close()">Fechar</button>
        }
      </div>
    </app-sheet-frame>
  `,
  styleUrl: '../resource-pick/resource-sheet.scss',
})
export class LayOnHandsSheet {
  private readonly api = inject(ResourceClient);
  private readonly sheet = injectSheet<LayOnHandsSheetData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly reach = reachNote();

  protected readonly pool = computed(
    () => poolOf(this.data.vitals()?.resources, LAY_ON_HANDS_RESOURCE) ?? { left: 0, total: 0 },
  );
  protected readonly targetRows = computed(() =>
    touchTargetRows(this.data.targets, this.data.actorId),
  );
  protected readonly targetPicks = computed<PickRow[]>(() =>
    this.targetRows().map((r) => ({ id: r.id, title: r.label, sub: r.sub, blocked: r.blocked })),
  );

  private readonly pickedTarget = signal<string | null>(null);
  /** The target picked; the first one that can be touched until the player picks another. */
  protected readonly targetId = computed(() => {
    const rows = this.targetRows();
    const own = rows.find((r) => r.id === this.pickedTarget() && !r.blocked);
    return (own ?? rows.find((r) => !r.blocked))?.id ?? null;
  });
  protected readonly targetLabel = computed(
    () =>
      this.data.targets.find((t) => t.target?.combatantId === this.targetId())?.target?.label ?? '',
  );
  protected readonly mode = signal<Mode>('heal');
  /** The points asked for; cut to the pool when it is smaller (the server limits it too). */
  protected readonly asked = signal<number | null>(null);
  protected readonly amount = computed(() =>
    Math.min(this.asked() ?? firstAmount(this.pool()), Math.max(1, this.pool().left)),
  );

  protected readonly modeRows = computed<PickRow[]>(() => {
    const short =
      this.pool().left < LAY_ON_HANDS_CURE_COST
        ? `custa ${LAY_ON_HANDS_CURE_COST} pontos (restam ${this.pool().left})`
        : '';
    return [
      {
        id: 'heal',
        title: 'Restaurar PV',
        sub: `gasta de 1 a ${this.pool().left} pontos`,
        blocked: '',
      },
      {
        id: 'poison',
        title: 'Neutralizar um veneno',
        sub: short ? '' : `gasta ${LAY_ON_HANDS_CURE_COST} pontos`,
        blocked: short,
      },
      {
        id: 'disease',
        title: 'Curar uma doença',
        sub: short ? '' : `gasta ${LAY_ON_HANDS_CURE_COST} pontos`,
        blocked: short,
      },
    ];
  });
  protected readonly preview = computed(() => {
    const label = this.targetLabel();
    return this.mode() === 'heal'
      ? healPreview(label || 'O alvo', this.amount(), this.pool())
      : curePreview(this.pool());
  });
  protected readonly doLabel = computed(() => {
    const label = this.targetLabel();
    switch (this.mode()) {
      case 'poison':
        return `Neutralizar o veneno${label ? ` de ${label}` : ''}`;
      case 'disease':
        return `Curar a doença${label ? ` de ${label}` : ''}`;
      default:
        return `Curar ${this.amount()} PV${label ? ` em ${label}` : ''}`;
    }
  });
  protected readonly ready = computed(() => this.targetId() !== null);

  protected readonly subtitle = computed(() => {
    const p = this.pool();
    return `Ação · toque · Reserva: ${p.left} de ${p.total} pontos`;
  });

  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');
  protected readonly result = signal<TouchResult | null>(null);
  /** One key per touch (who and what): the same values again are a retry, others another touch. */
  private readonly key = new ActionKey();

  protected pickTarget(id: string): void {
    this.pickedTarget.set(id);
    this.error.set('');
  }

  protected pickMode(id: string): void {
    this.mode.set(id as Mode);
    this.error.set('');
  }

  private chosenEffect(): LayOnHandsEffect {
    const mode = this.mode();
    return mode === 'heal' ? { amount: this.amount() } : { cure: mode };
  }

  protected async touch(): Promise<void> {
    const target = this.targetId();
    if (target === null || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    const what = this.chosenEffect();
    try {
      const total = this.pool().total;
      const res = await this.api.useLayOnHands(
        this.data.campaignId,
        this.data.encounterId,
        this.data.actorId,
        target,
        what,
        this.key.keyFor({ target, what }),
      );
      this.key.renew();
      if (res.encounter) {
        this.data.state.apply(res.encounter);
      }
      this.result.set(
        touchResult(
          {
            spent: res.spent,
            poolLeft: res.poolLeft,
            healed: res.healed,
            nothingHappened: res.nothingHappened,
          },
          what,
          this.targetLabel(),
          total,
        ),
      );
    } catch (err) {
      this.error.set(classResourceErrorMessage(err, 'usar a Cura pelas Mãos'));
    } finally {
      this.busy.set(false);
    }
  }

  protected close(): void {
    if (this.busy()) {
      return;
    }
    this.sheet.close(this.result() !== null);
  }
}
