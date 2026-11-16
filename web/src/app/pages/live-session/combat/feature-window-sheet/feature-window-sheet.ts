import {
  afterNextRender,
  ChangeDetectionStrategy,
  Component,
  computed,
  DestroyRef,
  effect,
  ElementRef,
  inject,
  Injector,
  signal,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type {
  GetTurnOptionsResponse,
  ReactionWindow,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { ActionOption } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { combatErrorMessage, encounterBlocked } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import {
  asksTargets,
  choiceComplete,
  featureRefusalText,
  leadOf,
  missingText,
  pickable,
  targetLimitText,
  useButtonLabel,
  useRows,
} from '../../../../core/combat/feature-use';
import { article } from '../../../../core/format/article';
import { theName } from '../../../../core/combat/reaction-view';
import { ownCombatant } from '../../../../core/combat/combat-view';
import { playerAutoPassText, playerSecondsLeft } from '../../../../core/combat/reaction-autopass';
import { featureWindowView } from '../../../../core/combat/reaction-feature';
import { ActionKey } from '../../../../core/connect/idempotency';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../../shared/sheet/sheet-host';
import { ResourcePick } from '../../../../shared/resource-pick/resource-pick';

/** How often the prompt's countdown redraws, in ms. */
const CLOCK_TICK_MS = 1000;

/** What the page hands the prompt of a feature window. */
export interface FeatureWindowSheetData {
  readonly campaignId: string;
  readonly encounterId: string;
  readonly window: ReactionWindow;
  readonly round: number;
  /** Reads the reactor's own turn options: the feature's limits and the creatures it may be aimed at. */
  readonly load: () => Promise<GetTurnOptionsResponse>;
  readonly state: CombatState;
}

/**
 * "O Zumbi 1 chegou a 1,5 m de você. Usar o Halo de esporos?" (#743, board 743-02): the player's `alertdialog` for a reaction
 * feature whose trigger happened (`ReactionKind.FEATURE`). The page opens it by itself and it cannot be closed without an
 * answer. It names the creature and says what the reaction costs, lists whom the feature may be aimed at (read from the
 * reactor's own turn options; the creature that triggered it is chosen), and answers with `AnswerReaction`: "Usar no Zumbi 1"
 * or "Não usar", the safe one first and with the focus. The master's screen passes it after 30 s, so the footer counts the
 * seconds. If the list cannot be read, the creature that triggered it is still used on. If the window closed meanwhile (the
 * time ran out, or the master answered), it says so and only closes. The server decides every refusal, and its sentence is
 * written in a red band with the sheet kept open.
 */
@Component({
  selector: 'app-feature-window-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatButtonModule, MatIconModule, ResourcePick, SheetFrame],
  template: `
    <app-sheet-frame
      icon="auto_awesome"
      [title]="view.title"
      [subtitle]="view.subtitle"
      [phone]="inSheet"
      [closable]="false"
      [focusableBody]="true"
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
        @if (gone()) {
          <p class="what" role="status">{{ goneText() }}</p>
        } @else {
          <p class="what">
            <b>{{ view.lead }}</b> {{ view.question }}
          </p>
          <p class="small">{{ view.note }}</p>
          @if (option(); as o) {
            @if (asks()) {
              <app-resource-pick
                [label]="'Em quem · ' + limit()"
                [rows]="rows()"
                [multi]="many()"
                [picked]="targetIds()"
                [chosen]="targetIds().at(0) ?? null"
                empty="Não há ninguém para escolher."
                (pick)="pickTarget($event)"
              />
              @if (many()) {
                <p class="small" role="status" aria-live="polite">
                  {{ targetIds().length }} de {{ o.targetMax }}
                  {{ targetIds().length === 1 ? 'escolhido' : 'escolhidos' }}.
                </p>
              }
            }
          } @else if (readFailed()) {
            <p class="small" role="status">
              Não deu para ler quem está ao alcance. Dá para usar no {{ actorLabel }}, ou não usar.
            </p>
          } @else {
            <p class="small" role="status">Lendo quem está ao alcance…</p>
          }
          @if (missing() && !busy()) {
            <p class="missing" id="feature-missing">
              <mat-icon aria-hidden="true">block</mat-icon>{{ missing() }}
            </p>
          }
        }
      </div>
      <div foot>
        @if (gone()) {
          <button matButton="outlined" type="button" class="foot__one" data-initial-focus (click)="sheet.close()">
            Fechar
          </button>
        } @else {
          <p class="small foot__clock" role="status">{{ autoPass() }}</p>
          <div class="pair">
            <button
              matButton="outlined"
              type="button"
              class="pair__btn"
              data-initial-focus
              [disabled]="busy()"
              (click)="answer(false)"
            >
              {{ view.declineLabel }}
            </button>
            <button
              matButton="filled"
              type="button"
              class="pair__btn"
              [disabled]="!!missing() || busy()"
              disabledInteractive
              [attr.aria-describedby]="missing() ? 'feature-missing' : null"
              (click)="answer(true)"
            >
              {{ busy() ? 'Usando…' : doLabel() }}
            </button>
          </div>
        }
      </div>
    </app-sheet-frame>
  `,
  styleUrls: ['../resource-pick/resource-sheet.scss', './feature-window-sheet.scss'],
})
export class FeatureWindowSheet {
  private readonly api = inject(CombatClient);
  private readonly injector = inject(Injector);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  protected readonly sheet = injectSheet<FeatureWindowSheetData, void>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;

  protected readonly view = featureWindowView(this.data.window, this.data.round);
  private readonly actorId = this.data.window.trigger?.actorId ?? '';
  protected readonly actorLabel = this.data.window.trigger?.actorLabel ?? '';

  /** The feature's option and the creatures it may be aimed at, once read; `null` while they come or when the read failed. */
  protected readonly option = signal<ActionOption | null>(null);
  private readonly targets = signal<GetTurnOptionsResponse['featureTargets'][number]['targets']>(
    [],
  );
  protected readonly readFailed = signal(false);
  protected readonly targetIds = signal<readonly string[]>([]);

  protected readonly asks = computed(() => {
    const o = this.option();
    return !!o && asksTargets(o);
  });
  protected readonly many = computed(() => (this.option()?.targetMax ?? 1) > 1);
  protected readonly limit = computed(() => {
    const o = this.option();
    return o ? targetLimitText(o, 'head') : '';
  });
  protected readonly rows = computed(() => {
    const o = this.option();
    if (!o) {
      return [];
    }
    const full = this.many() && this.targetIds().length >= o.targetMax;
    return useRows(this.targets(), o).map((r) => ({
      id: r.id,
      title: r.label,
      sub: r.sub,
      blocked: r.blocked,
      off: full && !this.targetIds().includes(r.id),
    }));
  });
  private readonly picked = computed(() => ({ targetIds: this.targetIds(), choice: '' }));
  protected readonly missing = computed(() => {
    const o = this.option();
    return o ? missingText(o, this.picked()) : '';
  });
  protected readonly doLabel = computed(() => {
    const o = this.option();
    return o
      ? useButtonLabel(o, this.picked(), (id) => this.labelOf(id))
      : this.view.useLabel(this.actorLabel);
  });

  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal<{ readonly lead: string; readonly rest: string } | null>(null);
  private readonly key = new ActionKey();

  /** The master's screen passes the window after 30 s without an answer, like any optional reaction. */
  private readonly openedAt = Date.now();
  private readonly clock = signal(this.openedAt);
  private readonly clockTimer = setInterval(() => this.clock.set(Date.now()), CLOCK_TICK_MS);
  protected readonly autoPass = computed(() =>
    playerAutoPassText(playerSecondsLeft(this.openedAt, this.clock())),
  );

  /** The window is no longer in the combat: the time ran out, or the master answered for the player. */
  protected readonly gone = computed(
    () =>
      !this.busy() &&
      !(this.data.state.encounter()?.reactionWindows ?? []).some(
        (w) => w.id === this.data.window.id,
      ),
  );
  /** What the player reads of a window that closed: the reaction is spent only when the master used it for them. */
  protected readonly goneText = computed(() => {
    const e = this.data.state.encounter();
    const own = e ? ownCombatant(e) : null;
    return own?.reactionUsed
      ? `O mestre respondeu por você: a reação foi usada ${article(this.actorLabel) === 'a' ? 'na' : 'no'} ${this.actorLabel}.`
      : `Você não respondeu a tempo: ${theName(this.actorLabel)} seguiu. A sua reação continua livre.`;
  });

  constructor() {
    inject(DestroyRef).onDestroy(() => clearInterval(this.clockTimer));
    void this.read();
    // The safe answer has the focus, and its ring: the sheet opens for a keyboard-style answer.
    afterNextRender(
      () =>
        (
          this.host.nativeElement.querySelector('[data-initial-focus]') as HTMLButtonElement | null
        )?.focus({ focusVisible: true } as FocusOptions),
      { injector: this.injector },
    );
  }

  private async read(): Promise<void> {
    const key = this.data.window.trigger?.actionKey ?? '';
    try {
      const res = await this.data.load();
      const option = res.options?.featureActions.find((a) => a.action?.key === key);
      if (!option) {
        this.readFailed.set(true);
        return;
      }
      this.targets.set(res.featureTargets.find((t) => t.actionKey === key)?.targets ?? []);
      this.option.set(option);
      // The creature that triggered the window is chosen, when the feature may be aimed at it.
      const rows = useRows(this.targets(), option);
      if (asksTargets(option) && pickable(rows).includes(this.actorId)) {
        this.targetIds.set([this.actorId]);
      }
    } catch {
      this.readFailed.set(true);
    }
  }

  /** The name of a creature of the list, or of the one that triggered the window. */
  private labelOf(id: string): string {
    return id === this.actorId
      ? this.actorLabel
      : (this.targets().find((t) => t.target?.combatantId === id)?.target?.label ?? '');
  }

  /** One creature replaces the choice; several toggle, up to the most the use takes. */
  protected pickTarget(id: string): void {
    this.error.set(null);
    const max = this.option()?.targetMax ?? 1;
    if (max <= 1) {
      this.targetIds.set([id]);
      return;
    }
    const now = this.targetIds();
    if (now.includes(id)) {
      this.targetIds.set(now.filter((t) => t !== id));
    } else if (now.length < max) {
      this.targetIds.set([...now, id]);
    }
  }

  /** "Usar no Zumbi 1" or "Não usar": the answer to the window. */
  protected async answer(use: boolean): Promise<void> {
    const o = this.option();
    if (this.busy() || (use && o && !choiceComplete(o, this.picked()))) {
      return;
    }
    // With the list read, the creatures are the ones picked; without it, the creature that triggered the window.
    const creatureIds = use ? (o ? this.targetIds() : [this.actorId]) : [];
    const answer = { use, creatureIds };
    this.busy.set(true);
    this.error.set(null);
    try {
      const res = await this.api.attacks.answerReaction(
        this.data.campaignId,
        this.data.encounterId,
        this.data.window.id,
        answer,
        this.key.keyFor({ window: this.data.window.id, answer }),
      );
      this.key.renew();
      this.data.state.apply(res.encounter);
      this.busy.set(false);
      this.sheet.close();
    } catch (err) {
      this.error.set(this.refusal(err));
      this.busy.set(false);
    }
  }

  /** The server's sentence for a refused creature, naming who; any other failure in the usual words. */
  private refusal(err: unknown): { readonly lead: string; readonly rest: string } {
    const blocked = encounterBlocked(err);
    const own = blocked ? featureRefusalText(blocked, (id) => this.labelOf(id)) : '';
    return leadOf(own || combatErrorMessage(err, 'responder à reação'));
  }
}
