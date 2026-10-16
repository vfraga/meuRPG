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
  BARDIC_INSPIRATION_REACH_FT,
  useCost,
} from '../../../../core/resources/bardic-inspiration';
import { BARDIC_INSPIRATION_RESOURCE, poolOf } from '../../../../core/resources/pools';
import { resourceRows } from '../../../../core/resources/resource-targets';
import { ResourceClient } from '../../../../core/resources/resources-client';
import { classResourceErrorMessage } from '../../../../core/resources/resources-errors';
import { metersText } from '../../../../core/units';
import { CountPips } from '../../../../shared/count-pips/count-pips';
import type { VitalsVm } from '../../../../core/play/live-session.types';
import { type PickRow, ResourcePick } from '../resource-pick/resource-pick';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../../shared/sheet/sheet-host';

/** What the page hands "Inspiração de Bardo". */
export interface BardicInspirationSheetData {
  readonly campaignId: string;
  readonly encounterId: string;
  /** The bard's combatant. */
  readonly actorId: string;
  /** Who can be picked, and why a creature cannot (`GetTurnOptionsResponse.resource_targets`); never the bard. */
  readonly targets: readonly ResourceTarget[];
  /** The bard's own vitals: the uses of Inspiração de Bardo. */
  readonly vitals: Signal<VitalsVm | null>;
  readonly state: CombatState;
}

/**
 * "Inspiração de Bardo" (PM-07c 12, left; SRD 5.1, Bard): as a bonus action the bard gives one creature other than
 * itself, seen within 60 feet and able to hear, a die for 10 minutes; it spends one use. The list is the server's:
 * who already holds a die ("Já tem um dado") or cannot hear is grey with the reason written, and no reason ever names a
 * creature's type. The size of the die is not said here: the turn options do not carry it before the gift, and the
 * holder reads it on the die card. One key per gift; made again once it worked.
 */
@Component({
  selector: 'app-bardic-inspiration-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [CountPips, MatButtonModule, MatIconModule, ResourcePick, SheetFrame],
  template: `
    <app-sheet-frame
      title="Inspiração de Bardo"
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
        @if (given(); as who) {
          <div class="res" role="status" aria-live="polite">
            <span class="pill"><mat-icon aria-hidden="true">check</mat-icon>Dado entregue</span>
            <p class="what">Você inspirou {{ who }}.</p>
            <p class="small">
              Quem recebe guarda o dado por 10 minutos e pode somá-lo depois de rolar um d20. Ação bônus usada.
            </p>
          </div>
        } @else {
          <p class="what">
            Escolha <b>uma criatura que não seja você</b>, que você veja a até {{ reach }} e que possa
            <b>ouvir</b> você. Ela ganha um dado de Inspiração por 10 minutos.
          </p>
          <app-resource-pick
            label="Quem inspirar"
            [rows]="rows()"
            [chosen]="targetId()"
            [legendHidden]="true"
            empty="Não há ninguém para inspirar."
            (pick)="pick($event)"
          />
          <div class="tags">
            <span class="tag">Ação bônus</span>
            @if (uses(); as u) {
              <span class="tag">{{ cost() }}</span>
              <app-count-pips [left]="u.left" [total]="u.total" />
            }
          </div>
          <p class="small">Quem já tem um dado de Inspiração de Bardo não pode ganhar outro: aparece cinza na lista.</p>
        }
      </div>
      <div foot>
        @if (given()) {
          <button matButton="outlined" type="button" class="pair__btn" (click)="close()">Fechar</button>
        } @else {
          <div class="pair">
            <button
              matButton="filled"
              type="button"
              class="pair__btn"
              [disabled]="targetId() === null || busy()"
              disabledInteractive
              (click)="give()"
            >
              <mat-icon aria-hidden="true">music_note</mat-icon>{{ doLabel() }}
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
export class BardicInspirationSheet {
  private readonly api = inject(ResourceClient);
  private readonly sheet = injectSheet<BardicInspirationSheetData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly reach = metersText(BARDIC_INSPIRATION_REACH_FT);

  protected readonly uses = computed(() =>
    poolOf(this.data.vitals()?.resources, BARDIC_INSPIRATION_RESOURCE),
  );
  protected readonly cost = computed(() => {
    const u = this.uses();
    return u ? useCost(u) : '';
  });
  protected readonly subtitle = computed(() => {
    const u = this.uses();
    return u
      ? `Ação bônus · alcance ${this.reach} · Inspiração de Bardo: ${u.left} de ${u.total}`
      : `Ação bônus · alcance ${this.reach}`;
  });

  private readonly resourceRows = computed(() =>
    resourceRows(this.data.targets, BARDIC_INSPIRATION_REACH_FT),
  );
  protected readonly rows = computed<PickRow[]>(() =>
    this.resourceRows().map((r) => ({ id: r.id, title: r.label, sub: r.sub, blocked: r.blocked })),
  );
  private readonly picked = signal<string | null>(null);
  /** The creature picked; the first one that can be inspired until the player picks another. */
  protected readonly targetId = computed(() => {
    const rows = this.resourceRows();
    const own = rows.find((r) => r.id === this.picked() && !r.blocked);
    return (own ?? rows.find((r) => !r.blocked))?.id ?? null;
  });
  protected readonly targetLabel = computed(
    () => this.resourceRows().find((r) => r.id === this.targetId())?.label ?? '',
  );
  protected readonly doLabel = computed(() =>
    this.targetLabel() ? `Inspirar ${this.targetLabel()}` : 'Inspirar',
  );

  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');
  /** Who got the die, once it did. */
  protected readonly given = signal('');
  private readonly key = new ActionKey();

  protected pick(id: string): void {
    this.picked.set(id);
    this.error.set('');
  }

  protected async give(): Promise<void> {
    const target = this.targetId();
    if (target === null || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.giveBardicInspiration(
        this.data.campaignId,
        this.data.encounterId,
        this.data.actorId,
        target,
        this.key.keyFor({ target }),
      );
      this.key.renew();
      if (res.encounter) {
        this.data.state.apply(res.encounter);
      }
      this.given.set(this.targetLabel());
    } catch (err) {
      this.error.set(classResourceErrorMessage(err, 'dar a Inspiração de Bardo'));
    } finally {
      this.busy.set(false);
    }
  }

  protected close(): void {
    if (this.busy()) {
      return;
    }
    this.sheet.close(this.given() !== '');
  }
}
