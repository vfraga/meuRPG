import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  signal,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type { Combatant } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { ActionOption } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { effectivePreference } from '../../../../core/campaigns/dice-labels';
import {
  allyRows,
  asksHowMany,
  givesResult,
  healDice,
  spendPreview,
  spendRange,
} from '../../../../core/combat/action-spend';
import { CombatClient } from '../../../../core/combat/combat-client';
import { type DamageDie } from '../../../../core/combat/combat-client-types';
import { rollFormula } from '../../../../core/combat/combat-dice';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { ActionKey } from '../../../../core/connect/idempotency';
import { CountStepper } from '../../../../shared/count-stepper/count-stepper';
import { RollPicker } from '../../../../shared/roll-picker/roll-picker';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../../shared/sheet/sheet-host';
import { type PickRow, ResourcePick } from '../../../../shared/resource-pick/resource-pick';

/** What the page hands the sheet of a table feature's action. */
export interface TableActionSheetData {
  readonly campaignId: string;
  readonly encounterId: string;
  readonly combatantId: string;
  /** The action as `GetTurnOptions` sends it: its cost, its range and what it gives. */
  readonly option: ActionOption;
  /** The combat's combatants as the caller sees them: the allies a heal may go to come from here. */
  readonly combatants: readonly Combatant[];
  readonly diceMode: DiceMode;
  readonly preference: DicePreference;
  readonly state: CombatState;
}

const SELF = '';

/**
 * A table feature's action (MR-025) that asks something before it is used: "Quantos?" when the player chooses how many
 * points to spend (`cost_max` above the cost), "Para quem" when it heals or gives temporary hit points (the actor, or one
 * other player's character the caller sees and that is alive), and the dice of its heal in the two ways of rolling (RN-18).
 * A live line says what the number will cost and give ("Gasta 3 · cura 3d6": a preview, the server decides and refuses a
 * number outside the range, a creature or a dead target). The key is made once for each choice, so a repeated tap never
 * spends twice. A bottom sheet on a phone and a dialog from a tablet up.
 */
@Component({
  selector: 'app-table-action-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [CountStepper, MatButtonModule, MatIconModule, ResourcePick, RollPicker, SheetFrame],
  template: `
    <app-sheet-frame [title]="title()" [subtitle]="subtitle()" [phone]="inSheet" (closed)="close()">
      <div class="body">
        @if (error()) {
          <div class="mr-notice mr-notice--danger" role="alert">
            <mat-icon aria-hidden="true">error</mat-icon>
            <p>{{ error() }}</p>
          </div>
        }
        @if (used()) {
          <div class="res" role="status" aria-live="polite">
            @if (healed() > 0) {
              <p class="what">{{ healed() }} PV recuperados</p>
            }
            @if (tempGained() > 0) {
              <p class="what">{{ tempGained() }} PV temporários</p>
            }
            @if (healed() === 0 && tempGained() === 0) {
              <p class="what">{{ name() }} usado.</p>
            }
            @if (rolled()) {
              <p class="small">Dados: {{ rolled() }}</p>
            }
            <p class="small">Gastou {{ spent() }} {{ spent() === 1 ? 'uso' : 'usos' }}.</p>
          </div>
        } @else {
          @if (asks()) {
            <p class="ask" id="spend-q">Quantos?</p>
            <div class="step">
              <app-count-stepper
                aria-labelledby="spend-q"
                noun="uso"
                minusLabel="Gastar um a menos"
                plusLabel="Gastar um a mais"
                valueLabel="Gastar {n}"
                [value]="n()"
                [min]="range().min"
                [max]="range().max"
                (valueChange)="picked.set($event)"
              />
              <span class="step__unit">de {{ range().min }} a {{ range().max }}</span>
            </div>
          }
          @if (rows().length > 1) {
            <app-resource-pick label="Para quem" [rows]="rows()" [chosen]="target()" (pick)="target.set($event)" />
          }
          <p class="what" role="status" aria-live="polite">{{ preview() }}</p>
          @if (dice(); as d) {
            <app-roll-picker
              [canApp]="canApp"
              [canType]="canType"
              [preferApp]="preferApp"
              [label]="'Role ' + d.count + 'd' + d.sides + ' para ' + name()"
              [hint]="'Digite a soma dos dados (' + d.count + ' a ' + d.count * d.sides + ').'"
              totalNote="Dados"
              [min]="d.count"
              [max]="d.count * d.sides"
              [busy]="busy()"
              [appLabel]="'Rolar ' + d.count + 'd' + d.sides + ' no app'"
              [sticky]="true"
              [(typing)]="typing"
              (app)="use({ inApp: true })"
              (typed)="use({ sum: $event })"
            />
          }
        }
      </div>
      <div foot>
        @if (used()) {
          <button matButton="filled" type="button" class="done" (click)="close()">Voltar à sua vez</button>
        } @else if (!dice()) {
          <div class="pair">
            <button matButton="filled" type="button" class="pair__btn" [disabled]="busy()" disabledInteractive (click)="use(undefined)">
              Usar
            </button>
            <button matButton="outlined" type="button" class="pair__btn" [disabled]="busy()" (click)="close()">Cancelar</button>
          </div>
        }
      </div>
    </app-sheet-frame>
  `,
  styleUrl: '../resource-pick/resource-sheet.scss',
  styles: `
    .done {
      --mat-button-filled-container-height: 48px;
      width: 100%;
    }
  `,
})
export class TableActionSheet {
  private readonly api = inject(CombatClient);
  private readonly sheet = injectSheet<TableActionSheetData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;

  protected readonly canApp = this.data.diceMode !== DiceMode.PHYSICAL;
  protected readonly canType = this.data.diceMode !== DiceMode.APP;
  protected readonly preferApp =
    effectivePreference(this.data.diceMode, this.data.preference) === DicePreference.APP;

  protected readonly name = computed(() => this.data.option.action?.namePt ?? '');
  protected readonly title = computed(() =>
    this.typing() ? 'Digite o resultado do dado' : this.name(),
  );
  protected readonly subtitle = computed(() => 'Ação de uma característica da mesa');
  protected readonly asks = computed(() => asksHowMany(this.data.option));
  protected readonly range = computed(() => spendRange(this.data.option) ?? { min: 1, max: 1 });
  protected readonly picked = signal<number | null>(null);
  /** The points: the number picked, held inside the range, else the least; 0 for a fixed cost (the server takes the cost). */
  protected readonly n = computed(() => {
    const { min, max } = this.range();
    return Math.min(Math.max(this.picked() ?? min, min), max);
  });
  protected readonly preview = computed(() => spendPreview(this.data.option, this.n()));
  protected readonly dice = computed(() => healDice(this.data.option, this.n()));

  /** The actor first, then the other player characters the caller sees: only when the action gives a result. */
  protected readonly rows = computed<PickRow[]>(() =>
    givesResult(this.data.option)
      ? [
          { id: SELF, title: 'Você', sub: '', blocked: '' },
          ...allyRows(this.data.combatants, this.data.combatantId).map((a) => ({
            id: a.id,
            title: a.label,
            sub: a.sub,
            blocked: '',
          })),
        ]
      : [],
  );
  protected readonly target = signal(SELF);

  protected readonly typing = signal(false);
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');
  protected readonly used = signal(false);
  protected readonly healed = signal(0);
  protected readonly tempGained = signal(0);
  protected readonly spent = signal(0);
  /** "11 (3d6)": the dice the heal rolled, once used. */
  protected readonly rolled = signal('');
  private readonly key = new ActionKey();

  protected async use(die: DamageDie | undefined): Promise<void> {
    if (this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    // A fixed cost sends 0 (the server takes the cost); a choice sends the number picked.
    const spend = this.asks() ? this.n() : 0;
    const target = this.target();
    try {
      const res = await this.api.turn.takeAction(
        this.data.campaignId,
        this.data.encounterId,
        this.data.combatantId,
        this.data.option.action?.key ?? '',
        die,
        this.key.keyFor([this.data.option.action?.key, die, spend, target]),
        false,
        false,
        spend,
        target,
      );
      this.key.renew();
      this.data.state.apply(res.encounter);
      this.healed.set(res.healed ?? res.roll?.total ?? 0);
      this.tempGained.set(res.tempHpGained ?? 0);
      this.spent.set(res.spent ?? spend);
      this.rolled.set(res.roll ? rollFormula(res.roll) : '');
      this.used.set(true);
      this.typing.set(false);
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'usar a habilidade'));
    } finally {
      this.busy.set(false);
    }
  }

  protected close(): void {
    if (this.busy()) {
      return;
    }
    this.sheet.close(this.used());
  }
}
