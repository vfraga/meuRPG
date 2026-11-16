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

import type { ResourceTarget } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { ActionEconomy, type ActionOption } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { combatErrorMessage, encounterBlocked } from '../../../../core/combat/combat-errors';
import { reasonText } from '../../../../core/combat/combat-options';
import type { CombatState } from '../../../../core/combat/combat-state';
import {
  asksTargets,
  choiceComplete,
  featureRefusalText,
  leadOf,
  missingText,
  targetLimitText,
  useButtonLabel,
  useLine,
  useRows,
} from '../../../../core/combat/feature-use';
import { ActionKey } from '../../../../core/connect/idempotency';
import type { ResourceUsageVm } from '../../../../core/play/resource-usage.types';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../../shared/sheet/sheet-host';
import { ResourcePick } from '../../../../shared/resource-pick/resource-pick';

/** What the page hands the sheet of a feature taken with `UseFeature`. */
export interface UseFeatureSheetData {
  readonly campaignId: string;
  readonly encounterId: string;
  /** The user: the player's character, or the one the master acts as. */
  readonly combatantId: string;
  readonly userLabel: string;
  /** The action as `GetTurnOptions` sends it: its limits, its targets and its choices. */
  readonly option: ActionOption;
  /** The creatures the caller sees for this action, each with the reason it cannot be picked (`GetTurnOptionsResponse.feature_targets`). */
  readonly targets: readonly ResourceTarget[];
  /** The rest resource the use spends from, in the user's vitals, for its name. */
  readonly resource: ResourceUsageVm | undefined;
  /** The master acts: what is done is in the user's name, so the sheet asks to confirm even a use with nothing to choose. */
  readonly master: boolean;
  readonly state: CombatState;
}

/** What the sheet answers when the use worked: what it healed and gave, for the note on the turn. */
export interface UseFeatureSheetResult {
  readonly healed: number;
  readonly tempHp: number;
}

const ECONOMY_WORD: Readonly<Partial<Record<ActionEconomy, string>>> = {
  [ActionEconomy.ACTION]: 'Ação',
  [ActionEconomy.BONUS_ACTION]: 'Ação bônus',
  [ActionEconomy.REACTION]: 'Reação',
  [ActionEconomy.FREE]: 'Sem custo',
};

/**
 * "Usar característica" (#743): the sheet of a feature whose action declares a `use` (`ActionOption.via_use_feature`), taken
 * with `FeatureService.UseFeature`. It shows the limits the option carries (the scope and what is left, whom it takes and how
 * far, what it spends), asks "Em quem" (check boxes for several, cards for one; the server says who cannot be picked and
 * why) and "Qual opção" when the feature offers choices, and calls once. The server decides every refusal: its sentence is
 * written in a red band, the sheet stays open and what was chosen stays. While the call is in the air both buttons stop and
 * the sheet does not close under it. For the master acting as a player's character the sheet is the confirmation, even
 * for a use with nothing to choose. A bottom sheet on a phone and a dialog from a tablet up.
 */
@Component({
  selector: 'app-use-feature-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatButtonModule, MatIconModule, ResourcePick, SheetFrame],
  template: `
    <app-sheet-frame
      icon="auto_awesome"
      [title]="name()"
      [subtitle]="subtitle()"
      [phone]="inSheet"
      (closed)="close()"
    >
      <div class="body">
        @if (error(); as e) {
          <div class="mr-notice mr-notice--danger" role="alert">
            <mat-icon aria-hidden="true">error</mat-icon>
            <p>
              <b>{{ e.lead }}</b>
              @if (e.rest) {
                {{ ' ' + e.rest }}
              }
            </p>
          </div>
        }
        @if (overrideText(); as why) {
          <p class="override" role="status">
            <mat-icon aria-hidden="true">info</mat-icon>{{ why }}
          </p>
        }
        @if (choices().length > 0) {
          <fieldset class="choices">
            <legend class="ask">{{ question() }}</legend>
            <div class="choices__grid">
              @for (c of choices(); track c.key) {
                <button
                  type="button"
                  class="choice"
                  [class.choice--on]="c.key === choice()"
                  [attr.aria-pressed]="c.key === choice()"
                  [disabled]="busy()"
                  disabledInteractive
                  (click)="pickChoice(c.key)"
                >
                  @if (c.key === choice()) {
                    <mat-icon aria-hidden="true">check</mat-icon>
                  }
                  {{ c.textPt }}
                </button>
              }
            </div>
          </fieldset>
        }
        @if (asksTargets) {
          <app-resource-pick
            [label]="'Em quem · ' + limit"
            [rows]="rows()"
            [multi]="many"
            [picked]="targetIds()"
            [chosen]="targetIds().at(0) ?? null"
            empty="Não há ninguém para escolher."
            (pick)="pickTarget($event)"
          />
          @if (many) {
            <p class="small" role="status" aria-live="polite">
              {{ targetIds().length }} de {{ data.option.targetMax }} {{ targetIds().length === 1 ? 'escolhido' : 'escolhidos' }}.
              @if (!data.master) {
                Criaturas escondidas nunca aparecem aqui.
              }
            </p>
          }
        } @else if (data.master) {
          <div class="who">
            <span class="who__mark" aria-hidden="true">{{ initial() }}</span>
            <span class="who__text">
              <b>Em quem: {{ data.userLabel }}</b>
              <span class="small">A característica é só em quem usa.</span>
            </span>
          </div>
        }
        @if (gives()) {
          <p class="what">{{ gives() }}</p>
        }
        @if (missing() && !busy()) {
          <p class="missing" id="use-missing">
            <mat-icon aria-hidden="true">block</mat-icon>{{ missing() }}
          </p>
        }
      </div>
      <div foot>
        <div class="pair">
          <button matButton="outlined" type="button" class="pair__btn" [disabled]="busy()" (click)="close()">
            Cancelar
          </button>
          <button
            matButton="filled"
            type="button"
            class="pair__btn"
            [disabled]="!!missing() || busy()"
            disabledInteractive
            [attr.aria-describedby]="missing() ? 'use-missing' : null"
            (click)="use()"
          >
            {{ busy() ? 'Usando…' : overriding() ? 'Usar mesmo assim' : doLabel() }}
          </button>
        </div>
      </div>
    </app-sheet-frame>
  `,
  styleUrls: ['../resource-pick/resource-sheet.scss', './use-feature-sheet.scss'],
})
export class UseFeatureSheet {
  private readonly api = inject(CombatClient);
  private readonly sheet = injectSheet<UseFeatureSheetData, UseFeatureSheetResult>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;

  protected readonly name = computed(() => this.data.option.action?.namePt ?? '');
  protected readonly choices = computed(() => this.data.option.choices);
  /** The heading over the options: the feature's own question ("Qual forma"), else "Qual opção". */
  protected readonly question = computed(() => this.data.option.useQuestionPt || 'Qual opção');
  /** The master uses a feature whose part of the turn is spent: the server marked the row and lets the call through. */
  protected readonly overriding = computed(
    () => this.data.master && this.data.option.masterMayOverride,
  );
  protected readonly overrideText = computed(() => {
    if (!this.overriding()) {
      return '';
    }
    const why = reasonText(this.data.option.reason, true).replace(/\.$/, '');
    return `${why ? `${why}. ` : ''}O mestre pode usar mesmo assim.`;
  });
  protected readonly asksTargets = asksTargets(this.data.option);
  protected readonly many = this.data.option.targetMax > 1;
  protected readonly limit = targetLimitText(this.data.option, 'head');
  protected readonly subtitle = computed(() => {
    const o = this.data.option;
    const word = ECONOMY_WORD[o.action?.economy ?? ActionEconomy.UNSPECIFIED] ?? '';
    return [word, useLine(o, this.data.resource)].filter(Boolean).join(' · ');
  });
  protected readonly gives = computed(() => {
    const temp = this.data.option.action?.tempHp ?? 0;
    return temp > 0 ? `${this.data.userLabel} ganha ${temp} PV temporários.` : '';
  });
  protected readonly initial = computed(() => this.data.userLabel.slice(0, 1).toUpperCase());

  /** Once the most the use takes is chosen, the others are off (the count above says why); unchecking one frees them. */
  protected readonly rows = computed(() => {
    const full = this.many && this.targetIds().length >= this.data.option.targetMax;
    return useRows(this.data.targets, this.data.option).map((r) => ({
      id: r.id,
      title: r.label,
      sub: r.sub,
      blocked: r.blocked,
      off: full && !this.targetIds().includes(r.id),
    }));
  });
  protected readonly targetIds = signal<readonly string[]>([]);
  protected readonly choice = signal('');
  private readonly picked = computed(() => ({
    targetIds: this.targetIds(),
    choice: this.choice(),
  }));
  protected readonly missing = computed(() => missingText(this.data.option, this.picked()));
  protected readonly doLabel = computed(() =>
    useButtonLabel(this.data.option, this.picked(), (id) => this.labelOf(id) ?? ''),
  );

  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal<{ readonly lead: string; readonly rest: string } | null>(null);
  private readonly key = new ActionKey();

  /** The name of a creature of the list, or of the user. */
  private labelOf(id: string): string | undefined {
    return id === this.data.combatantId
      ? this.data.userLabel
      : this.data.targets.find((t) => t.target?.combatantId === id)?.target?.label;
  }

  protected pickChoice(key: string): void {
    this.choice.set(key);
    this.error.set(null);
  }

  /** One creature replaces the choice; several toggle, up to the most the use takes. */
  protected pickTarget(id: string): void {
    this.error.set(null);
    if (!this.many) {
      this.targetIds.set([id]);
      return;
    }
    const now = this.targetIds();
    if (now.includes(id)) {
      this.targetIds.set(now.filter((t) => t !== id));
    } else if (now.length < this.data.option.targetMax) {
      this.targetIds.set([...now, id]);
    }
  }

  protected async use(): Promise<void> {
    if (this.busy() || !choiceComplete(this.data.option, this.picked())) {
      return;
    }
    const args = {
      campaignId: this.data.campaignId,
      encounterId: this.data.encounterId,
      combatantId: this.data.combatantId,
      actionKey: this.data.option.action?.key ?? '',
      targetIds: this.targetIds(),
      choice: this.choice(),
    };
    this.busy.set(true);
    this.error.set(null);
    try {
      const res = await this.api.features.useFeature(args, this.key.keyFor(args));
      this.key.renew();
      this.data.state.apply(res.encounter);
      this.busy.set(false);
      this.sheet.close({ healed: res.healed, tempHp: res.tempHp });
    } catch (err) {
      this.error.set(this.refusal(err));
      this.busy.set(false);
    }
  }

  /** The server's sentence for a refused target or choice, naming who; any other failure in the usual words. */
  private refusal(err: unknown): { readonly lead: string; readonly rest: string } {
    const blocked = encounterBlocked(err);
    const own = blocked ? featureRefusalText(blocked, (id) => this.labelOf(id)) : '';
    return leadOf(own || combatErrorMessage(err, 'usar a característica'));
  }

  protected close(): void {
    if (this.busy()) {
      return;
    }
    this.sheet.close();
  }
}
