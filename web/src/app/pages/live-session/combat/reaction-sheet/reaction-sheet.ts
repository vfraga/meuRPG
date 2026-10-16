import { NgTemplateOutlet } from '@angular/common';
import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  ElementRef,
  computed,
  effect,
  inject,
  signal,
  viewChild,
  Injector,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  type ConcentrationSaveResult,
  type LuckyOffer,
  type OutsideInspirationOffer,
  type ReactionResult,
  EncounterBlockedReason,
  type ReactionWindow,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { effectivePreference } from '../../../../core/campaigns/dice-labels';
import {
  type SlotRow,
  defaultSlot,
  lastSlotWarning,
  slotRows,
} from '../../../../core/combat/cast-flow';
import {
  CombatClient,
  type ConcentrationAnswer,
  type ReactionAnswer,
} from '../../../../core/combat/combat-client';
import { combatErrorMessage, encounterBlocked } from '../../../../core/combat/combat-errors';
import { luckyHeldLine, luckyReroll } from '../../../../core/combat/lucky-question';
import type { CombatState } from '../../../../core/combat/combat-state';
import {
  type ConcentrationMode,
  type PromptView,
  type ResultView,
  type SlotAfter,
  concentrationMode,
  concentrationModeLine,
  concentrationResultView,
  dieSidesOf,
  REACTION_NAMES,
  isOpen,
  promptView,
  rebukeRows,
  resultView,
  slotChoices,
  throwText,
} from '../../../../core/combat/reactions';
import {
  isOptionalReaction,
  playerAutoPassText,
  playerRollingSecondsLeft,
  playerRollingText,
  playerSecondsLeft,
} from '../../../../core/combat/reaction-autopass';
import { metersText } from '../../../../core/units';
import { ActionKey } from '../../../../core/connect/idempotency';
import {
  ResourceClient,
  type InspirationRoll,
  type LuckyRoll,
} from '../../../../core/resources/resources-client';
import { classResourceErrorMessage } from '../../../../core/resources/resources-errors';
import { InspirationOffer } from '../../inspiration/inspiration-offer';
import { LuckyPrompt } from '../lucky-prompt/lucky-prompt';
import { SlotPicker } from '../../../../shared/slot-picker/slot-picker';
import { ExtraDiceState } from '../../../../core/effects/extra-dice-state';
import {
  RollAnimator,
  type RollShow,
  showOfConcentration,
  showOfDie,
  showOfReaction,
} from '../../../../shared/roll-overlay/roll-animator';
import { ExtraDice } from '../../effects/extra-dice/extra-dice';
import { MultiRoll, type RollField } from '../multi-roll/multi-roll';
import { RollPicker } from '../../../../shared/roll-picker/roll-picker';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../../shared/sheet/sheet-host';

/** What the page hands the reaction sheet. */
export interface ReactionSheetData {
  readonly campaignId: string;
  readonly encounterId: string;
  /** The window to answer, as the server sent it (the sheet keeps its prompt for the result). */
  readonly window: ReactionWindow;
  readonly round: number;
  readonly usage: readonly {
    readonly level: number;
    readonly total: number;
    readonly used: number;
  }[];
  readonly pact: {
    readonly slotLevel: number;
    readonly total: number;
    readonly used: number;
  } | null;
  readonly state: CombatState;
  /** The player's armor class from their own sheet, for "Sua CA é 18"; `null` when unknown. */
  readonly armorClass: number | null;
  readonly diceMode: DiceMode;
  readonly preference: DicePreference;
}

/** What closes the sheet: the monk asked to throw the missile back (the page opens the attack with this window). */
export interface ReactionSheetResult {
  readonly throwWindowId: string;
}

type Step = 'ask' | 'roll' | 'result';

/** How often the prompt's countdown is redrawn. */
const AUTO_PASS_TICK_MS = 1000;

/** The pseudo level of the Infernal Legacy's row, which has no slot. */
const RACIAL_LEVEL = 0;

/** Counterspell is cast with a slot of the 3rd level or above. */
const COUNTERSPELL_MIN_LEVEL = 3;
/** The die of a saving throw or an ability check. */
const D20 = 20;

const ARROWS = new Set(['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight']);

/**
 * The reaction sheet (PM-04): "Você foi atingido: usar Escudo Arcano?", and the same question for Esquiva
 * Sobrenatural, Repreensão Infernal, Contramágica, Palavras de Interrupção, Defletir Projéteis, Queda Suave and the
 * concentration save. It opens by itself when the combat has a window for the player (`Encounter.reaction_windows`,
 * `for_you`) and is an alert dialog that has to be answered: focus starts on "Deixar passar" (the safe choice; on
 * "Rolar no app" for the concentration save), Esc passes, the arrows change the slot, and the two buttons have the
 * same size. The player decides without the total or the armor class, as at a table. What the answer did comes back
 * as a result sheet with "Fechar"; the monk's second step ("Devolver" or "Guardar a flecha") stands in its place.
 * The master may answer for the player: the window is then gone and this says so. Every answer has its own key, made
 * once, so a repeated tap never answers twice. Nothing is kept in the browser.
 */
@Component({
  selector: 'app-reaction-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [
    ExtraDice,
    InspirationOffer,
    LuckyPrompt,
    MatButtonModule,
    MatIconModule,
    MultiRoll,
    NgTemplateOutlet,
    RollPicker,
    SheetFrame,
    SlotPicker,
  ],
  host: { '(keydown)': 'onKey($event)' },
  template: `
    <app-sheet-frame
      [title]="title()"
      [subtitle]="subtitle()"
      [icon]="icon()"
      [phone]="inSheet"
      [closable]="false"
    >
      @if (error()) {
        <div class="mr-notice mr-notice--danger" role="alert">
          <mat-icon aria-hidden="true">error</mat-icon>
          <p>{{ error() }}</p>
        </div>
      }
      @switch (stage()) {
        @case ('ask') {
          @if (luckyOffer(); as l) {
            <!-- The save's d20 shows a 1 and the reactor is a halfling: it is kept, and Lucky asks before the die's question (SRD 5.1). -->
            <p class="what">{{ luckyHeld() }}</p>
            <app-lucky-prompt
              [offer]="l"
              [canApp]="canApp"
              [canType]="canType"
              [preferApp]="preferApp"
              [busy]="busy()"
              (reroll)="answerLucky(true, $event)"
              (keep)="answerLucky(false, null)"
            />
          } @else if (offer(); as o) {
            <app-inspiration-offer
              [offer]="o"
              [canApp]="canApp"
              [canType]="canType"
              [busy]="busy()"
              (use)="answerDie(true, $event)"
              (keep)="answerDie(false, null)"
            />
          } @else if (view(); as v) {
            <p class="what">{{ v.question }}</p>
            @if (isFeatherFall()) {
              <ng-container *ngTemplateOutlet="falling" />
            }
            @if (rows().length > 0) {
              <app-slot-picker
                [rows]="rows()"
                [chosen]="chosen()"
                [label]="pickerLabel()"
                (pick)="chosen.set($event); error.set('')"
              />
            }
            @if (maneuverOptions().length > 1) {
              <fieldset class="falling">
                <legend class="falling__cap">Qual manobra</legend>
                @for (o of maneuverOptions(); track o.key) {
                  <label class="falling__row">
                    <input
                      type="radio"
                      class="falling__input"
                      name="reaction-maneuver"
                      [checked]="maneuverKey() === o.key"
                      (change)="maneuverKey.set(o.key)"
                    />
                    <span class="falling__text">
                      <b>{{ o.namePt }}</b>
                      <small>d{{ o.dieSides }} + {{ o.flatBonus }} · {{ o.usesLeft }} usos</small>
                    </span>
                  </label>
                }
              </fieldset>
            }
            @if (warning()) {
              <div class="mr-notice mr-notice--warning" role="status">
                <mat-icon aria-hidden="true">info</mat-icon>
                <p><strong>{{ warning() }}</strong></p>
              </div>
            }
            @if (v.costs.length > 0) {
              <ul class="chips" aria-label="O que a reação custa">
                @for (c of v.costs; track c; let first = $first) {
                  <li class="chip" [class.chip--accent]="first">{{ c }}</li>
                }
              </ul>
            }
            @if (v.note) {
              <p class="small">{{ v.note }}</p>
            }
            @if (concentrationModeText()) {
              <p class="what" role="status">{{ concentrationModeText() }}</p>
            }
            @if (typingSave()) {
              @if (extra.fields().length > 0) {
                <app-extra-dice [fields]="extra.fields()" [(faces)]="extra.faces" />
              }
              @if (concentrationDice() === 1) {
                <app-roll-picker
                  [canApp]="false"
                  [canType]="true"
                  [min]="1"
                  [max]="20"
                  [modifier]="saveBonus()"
                  label="Role 1d20 para o teste de resistência de Constituição"
                  hint="Role o seu dado e digite o número que saiu (1 a 20)."
                  totalNote="Teste de Constituição"
                  [busy]="busy()"
                  (typed)="concentrate({ kind: 'typed', face: $event })"
                />
              } @else {
                <app-multi-roll
                  [fields]="pair"
                  [canApp]="false"
                  [canType]="true"
                  [modifier]="saveBonus()"
                  [combine]="concentrationMode() === 'advantage' ? 'higher' : 'lower'"
                  totalNote="Teste de Constituição"
                  hint="Role os dois d20 e digite os dois números, na ordem em que saíram (1 a 20 cada)."
                  [busy]="busy()"
                  (typed)="concentrate({ kind: 'typed', face: $event[0], second: $event[1] })"
                />
              }
            }
          }
        }
        @case ('roll') {
          <p class="what">{{ rollQuestion() }}</p>
          <app-roll-picker
            [canApp]="canApp"
            [canType]="canType"
            [preferApp]="preferApp"
            [min]="1"
            [max]="rollSides()"
            [modifier]="0"
            [label]="rollLabel()"
            hint="Role o seu dado e digite o número que saiu."
            [totalNote]="name()"
            [appLabel]="'Rolar no app'"
            [busy]="busy()"
            (app)="use({ inApp: true })"
            (typed)="use({ typed: $event })"
          />
        }
        @case ('gone') {
          <p class="what" role="status">{{ goneText() }}</p>
        }
        @default {
          @if (shown(); as r) {
            <div class="res" role="status" aria-live="polite">
              @if (r.pill; as pill) {
                <span class="pill" [class.pill--bad]="!pill.good">
                  <mat-icon aria-hidden="true">{{ pill.good ? 'check' : 'close' }}</mat-icon>{{ pill.word }}
                </span>
              }
              @for (t of r.text; track $index; let first = $first) {
                <p class="what" [class.what--strong]="first">{{ t }}</p>
              }
              @if (r.throwBack && throwQuestion()) {
                <p class="what">{{ throwQuestion() }}</p>
              }
              @if (r.chips.length > 0) {
                <ul class="chips" aria-label="O que a reação gastou">
                  @for (c of r.chips; track c) {
                    <li class="chip">{{ c }}</li>
                  }
                </ul>
              }
              @if (r.note) {
                <p class="small">{{ r.note }}</p>
              }
            </div>
          }
        }
      }
      <div foot>
        @switch (stage()) {
          @case ('ask') {
            @if (offer() || luckyOffer()) {
              <!-- The question about the die (or Lucky's) is the whole step: its own buttons answer it. -->
            } @else if (isConcentration()) {
              <div class="stack">
                <button
                  mat-flat-button
                  type="button"
                  class="pair__btn"
                  data-initial-focus
                  [disabled]="busy()"
                  (click)="concentrate({ kind: 'app' })"
                >
                  <mat-icon aria-hidden="true">casino</mat-icon>Rolar no app
                </button>
                <button
                  mat-stroked-button
                  type="button"
                  class="pair__btn"
                  [disabled]="busy()"
                  (click)="typingSave.set(true)"
                >
                  Digitar o resultado
                </button>
                <button
                  mat-stroked-button
                  type="button"
                  class="pair__btn"
                  [disabled]="busy()"
                  (click)="concentrate({ kind: 'hand' })"
                >
                  Deixar o mestre rolar por mim
                </button>
              </div>
            } @else {
              <div class="pair">
                <button
                  mat-flat-button
                  type="button"
                  class="pair__btn"
                  [disabled]="busy() || !canUse()"
                  disabledInteractive
                  (click)="use()"
                >
                  {{ view()?.useLabel }}
                </button>
                <button
                  mat-stroked-button
                  type="button"
                  class="pair__btn"
                  data-initial-focus
                  [disabled]="busy()"
                  (click)="pass()"
                >
                  Deixar passar
                </button>
              </div>
              <p class="fine">
                <mat-icon aria-hidden="true">info</mat-icon>{{ fineText() }}
              </p>
              @if (refreshed) {
                <p class="fine fine--now" role="status">
                  <mat-icon aria-hidden="true">sync</mat-icon>Combate atualizado agora.
                </p>
              }
            }
          }
          @case ('roll') {
            <button mat-stroked-button type="button" class="pair__btn pair__btn--one" data-initial-focus (click)="back()">
              Voltar
            </button>
            @if (rollingAt() !== null) {
              <p class="fine">
                <mat-icon aria-hidden="true">info</mat-icon>{{ fineText() }}
              </p>
            }
          }
          @default {
            @if (stage() === 'result' && shown()?.throwBack) {
              <div class="pair">
                <button
                  mat-flat-button
                  type="button"
                  class="pair__btn"
                  [disabled]="busy()"
                  (click)="throwBack()"
                >
                  Devolver (1 de chi)
                </button>
                <button
                  #keep
                  mat-stroked-button
                  type="button"
                  class="pair__btn"
                  data-initial-focus
                  [disabled]="busy()"
                  (click)="keepMissile()"
                >
                  Guardar a flecha
                </button>
              </div>
            } @else {
              <button #close mat-stroked-button type="button" class="pair__btn pair__btn--one" data-initial-focus (click)="done()">
                Fechar
              </button>
            }
          }
        }
      </div>
    </app-sheet-frame>

    <ng-template #falling>
      <fieldset class="falling">
        <legend class="falling__cap">Quem cai, e você vê (até {{ maxTargets() === 5 ? 'cinco' : maxTargets() }})</legend>
        @for (f of fallingRows(); track f.id) {
          <label class="falling__row">
            <input
              type="checkbox"
              class="falling__input"
              [checked]="picked().has(f.id)"
              [disabled]="!picked().has(f.id) && picked().size >= maxTargets()"
              (change)="toggle(f.id)"
            />
            <span class="falling__box" aria-hidden="true">
              @if (picked().has(f.id)) {
                <mat-icon>check</mat-icon>
              }
            </span>
            <span class="falling__text">
              <b>{{ f.label }}</b>
              <small>{{ f.detail }}</small>
            </span>
          </label>
        }
      </fieldset>
    </ng-template>
  `,
  styleUrl: './reaction-sheet.scss',
})
export class ReactionSheet {
  private readonly api = inject(CombatClient);
  /** Read when the player answers about the die: a sheet that never asks does not build the client. */
  private readonly lazyInjector = inject(Injector);
  private get resources(): ResourceClient {
    return this.lazyInjector.get(ResourceClient);
  }
  private readonly animator = inject(RollAnimator);
  private readonly sheet = injectSheet<ReactionSheetData, ReactionSheetResult>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;

  /** The window being asked: the first one, then the second step the answer opened. */
  protected readonly asked = signal<ReactionWindow>(this.data.window);
  /** The sheet opened on the monk's second step (after a reload): it starts at "Devolver?". */
  private readonly secondStep = this.data.window.prompt.case === 'deflectThrow';
  protected readonly step = signal<Step>(this.secondStep ? 'result' : 'ask');
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');
  protected readonly typingSave = signal(false);
  /** The concentration save's d20 was rolled and kept: the character holds a Bardic Inspiration die, and the player
   * decides before the result is told. The window stays open. */
  protected readonly offer = signal<OutsideInspirationOffer | null>(null);
  private readonly dieKey = new ActionKey();
  /** The concentration save's d20 was rolled and kept because it shows a 1 and the character is a halfling: the Lucky
   * question ("Tirou 1: rolar de novo?") comes first, before the die's. The window stays open. */
  protected readonly luckyOffer = signal<LuckyOffer | null>(null);
  /** The held d20, above the Lucky question. */
  protected readonly luckyHeld = computed(() => luckyHeldLine(this.luckyOffer()));
  /** One key per Lucky answer (roll again this way, or keep the 1): the same answer again is a retry. */
  private readonly luckyKey = new ActionKey();
  /** The d4 an effect adds to the typed saving throw (Bênção, Perdição): asked when the server says the roll takes them. */
  protected readonly extra = new ExtraDiceState();
  protected readonly picked = signal<ReadonlySet<string>>(new Set());
  /** What the answer did, once the server told it. */
  private readonly answer = signal<
    | { readonly kind: 'reaction'; readonly result: ReactionResult }
    | { readonly kind: 'concentration'; readonly result: ConcentrationSaveResult }
    | null
  >(null);
  private readonly afterSlot = signal<SlotAfter | null>(null);
  /** The monk's second step: the window that asks "Devolver?". */
  private readonly nextWindowId = signal(this.secondStep ? this.data.window.id : '');

  /** The window came with a read, after a reload: the prompt is back and says so. */
  protected readonly refreshed =
    !this.secondStep && !this.data.state.wasAnnounced(this.data.window.id);
  protected readonly canApp = this.data.diceMode !== DiceMode.PHYSICAL;
  protected readonly canType = this.data.diceMode !== DiceMode.APP;
  protected readonly preferApp =
    effectivePreference(this.data.diceMode, this.data.preference) === DicePreference.APP;

  protected readonly view = computed<PromptView | null>(() =>
    promptView(this.asked(), this.data.round),
  );
  protected readonly isConcentration = computed(
    () => this.asked().prompt.case === 'concentrationSave',
  );
  protected readonly isFeatherFall = computed(() => this.asked().prompt.case === 'featherFall');
  protected readonly name = computed(() => this.view()?.name ?? '');
  protected readonly pickerLabel = computed(() =>
    this.asked().prompt.case === 'hellishRebuke' ? 'Como pagar' : 'Espaço de magia a gastar',
  );

  /** The rows of the radio list: the slots (or, for Repreensão Infernal, the Infernal Legacy and the slots). */
  protected readonly rows = computed<readonly SlotRow[]>(() => {
    const w = this.asked();
    switch (w.prompt.case) {
      case 'hellishRebuke':
        return rebukeRows(w).map((r) => ({
          level: r.racial ? RACIAL_LEVEL : r.level,
          pact: r.pact,
          free: r.free,
          total: null,
          used: 0,
          enabled: r.enabled,
          title: r.title,
          count: r.detail,
        }));
      case 'shield':
        return slotRows(1, slotChoices(w), this.data.usage, this.data.pact);
      case 'counterspell':
        return slotRows(COUNTERSPELL_MIN_LEVEL, slotChoices(w), this.data.usage, this.data.pact);
      case 'featherFall':
        return slotRows(1, slotChoices(w), this.data.usage, this.data.pact);
      default:
        return [];
    }
  });
  protected readonly chosen = signal<SlotRow | null>(defaultSlot(this.rows()));
  /** The maneuvers a reduction offers (the first is picked), when the window is a maneuver reduction. */
  protected readonly maneuverOptions = computed(() => {
    const p = this.asked().prompt;
    return p.case === 'maneuverReduce' ? p.value.options : [];
  });
  protected readonly maneuverKey = signal(
    (() => {
      const p = this.asked().prompt;
      return p.case === 'maneuverReduce' ? (p.value.options[0]?.key ?? '') : '';
    })(),
  );
  protected readonly warning = computed(() =>
    this.asked().prompt.case === 'shield' ? lastSlotWarning(this.chosen(), null) : '',
  );

  protected readonly fallingRows = computed(() => {
    const p = this.asked().prompt;
    return p.case === 'featherFall'
      ? p.value.falling.map((f) => ({
          id: f.combatantId,
          label: f.label,
          detail: [
            ...(f.ally ? ['aliado'] : []),
            ...(f.distanceFt !== undefined ? [`a ${metersText(f.distanceFt)}`] : []),
          ].join(' · '),
        }))
      : [];
  });
  protected readonly maxTargets = computed(() => {
    const p = this.asked().prompt;
    return p.case === 'featherFall' ? Math.max(1, p.value.maxTargets) : 1;
  });

  /** The mode of the concentration save the server will roll with, and the d20 a typed answer takes. */
  protected readonly concentrationMode = computed<ConcentrationMode>(() => {
    const p = this.asked().prompt;
    return p.case === 'concentrationSave' ? concentrationMode(p.value) : 'normal';
  });
  protected readonly concentrationDice = computed(() =>
    this.concentrationMode() === 'normal' ? 1 : 2,
  );
  protected readonly concentrationModeText = computed(() => {
    const p = this.asked().prompt;
    return p.case === 'concentrationSave' ? concentrationModeLine(p.value) : '';
  });
  protected readonly pair: readonly RollField[] = [
    { key: 'd20-1', label: 'Primeiro d20', min: 1, max: 20 },
    { key: 'd20-2', label: 'Segundo d20', min: 1, max: 20 },
  ];

  protected readonly saveBonus = computed(() => {
    const p = this.asked().prompt;
    return p.case === 'concentrationSave' && p.value.bonusKnown ? p.value.saveBonus : 0;
  });

  /** When the prompt opened: the seconds before it passes by itself count from here (the master's screen sends the pass). */
  private readonly openedAt = Date.now();
  private readonly clock = signal(this.openedAt);
  private readonly clockTimer = setInterval(() => this.clock.set(Date.now()), AUTO_PASS_TICK_MS);
  /** The small print under the buttons: the countdown of an optional reaction, or what the master may do. */
  protected readonly fineText = computed(() => {
    if (!isOptionalReaction(this.asked())) {
      return 'Se você não responder, o mestre pode decidir por você.';
    }
    // Once the player is rolling, the master's clock waits the longer time, and so does this one.
    const rolling = this.rollingAt();
    return rolling === null
      ? playerAutoPassText(playerSecondsLeft(this.openedAt, this.clock()))
      : playerRollingText(playerRollingSecondsLeft(rolling, this.clock()));
  });
  /** When the player moved to the die (ms), after the server was told; `null` before. */
  protected readonly rollingAt = signal<number | null>(null);

  /** "Usar ..." needs what the question asks for: a slot, a creature to save. */
  protected readonly canUse = computed(() => {
    if (this.rows().length > 0 && !this.chosen()?.enabled) {
      return false;
    }
    return !this.isFeatherFall() || this.picked().size > 0;
  });

  protected readonly rollSides = computed(() => dieSidesOf(this.asked()) || D20);
  protected readonly rollLabel = computed(() => `Role 1d${this.rollSides()} para ${this.name()}`);
  protected readonly rollQuestion = computed(() =>
    this.asked().prompt.case === 'counterspell'
      ? 'A magia é de nível maior que o espaço: faça o teste de habilidade de conjuração.'
      : `Role o dado de ${this.name()}.`,
  );

  /** The throw-back question of the monk's second step, from the window the server opened for it. */
  protected readonly throwQuestion = computed(() => {
    const w = this.data.state
      .encounter()
      ?.reactionWindows.find((x) => x.id === this.nextWindowId() && isOpen(x));
    return w?.prompt.case === 'deflectThrow'
      ? throwText(w.prompt.value.kiLeft, w.prompt.value.normalRangeFt, w.prompt.value.longRangeFt)
      : '';
  });

  /** Whether the window is still open on the server. */
  private readonly present = computed(() =>
    (this.data.state.encounter()?.reactionWindows ?? []).some(
      (w) => w.id === this.asked().id && isOpen(w),
    ),
  );

  /** What the sheet shows: the question, the die, the answer, or that the question is gone. */
  protected readonly stage = computed<'ask' | 'roll' | 'result' | 'gone'>(() => {
    if (this.step() === 'result') {
      return 'result';
    }
    return this.present() || this.busy() ? this.step() : 'gone';
  });

  protected readonly shown = computed<ResultView | null>(() => {
    const a = this.answer();
    if (!a) {
      return this.secondStep
        ? {
            title: REACTION_NAMES.deflectMissiles,
            subtitle: `Rodada ${this.data.round}`,
            icon: 'swap_calls',
            pill: null,
            text: [],
            chips: [],
            note: '',
            throwBack: true,
          }
        : null;
    }
    return a.kind === 'concentration'
      ? concentrationResultView(a.result, this.data.round)
      : resultView(a.result, this.data.window, {
          round: this.data.round,
          after: this.afterSlot(),
          armorClass: this.data.armorClass,
        });
  });

  protected readonly title = computed(() => {
    if (this.stage() === 'result') {
      return this.shown()?.title ?? '';
    }
    return this.view()?.title ?? 'Reação';
  });
  protected readonly subtitle = computed(() =>
    this.stage() === 'result' ? (this.shown()?.subtitle ?? '') : (this.view()?.subtitle ?? ''),
  );
  protected readonly icon = computed(() =>
    this.stage() === 'result' ? (this.shown()?.icon ?? '') : (this.view()?.icon ?? ''),
  );
  protected readonly goneText = computed(
    () =>
      this.data.state.reactionNotice()?.text ??
      'O mestre respondeu por você: esta reação já não espera a sua resposta.',
  );

  /** One key per answer (its slot, its die): a repeated tap is a retry, another answer is a new request. */
  private readonly keys = new ActionKey();
  private readonly focus = viewChild('close', { read: ElementRef<HTMLButtonElement> });
  private readonly keepButton = viewChild('keep', { read: ElementRef<HTMLButtonElement> });

  constructor() {
    // The countdown stops with the sheet.
    inject(DestroyRef).onDestroy(() => clearInterval(this.clockTimer));
    effect(() => (this.keepButton() ?? this.focus())?.nativeElement.focus());
    // Closed by itself (the reactor can no longer react): the page says why in its status line, and this goes away.
    effect(() => {
      if (this.stage() === 'gone' && this.data.state.reactionNotice()) {
        this.sheet.close();
      }
    });
    // Feather Fall starts with the allies that fall chosen.
    const p = this.data.window.prompt;
    if (p.case === 'featherFall') {
      this.picked.set(
        new Set(
          p.value.falling
            .filter((f) => f.ally)
            .slice(0, this.maxTargets())
            .map((f) => f.combatantId),
        ),
      );
    }
  }

  protected toggle(id: string): void {
    this.picked.update((set) => {
      const next = new Set(set);
      if (!next.delete(id)) {
        next.add(id);
      }
      return next;
    });
    this.error.set('');
  }

  /** The answer the question collects: the slot or the Infernal Legacy, the creatures, the die. */
  private build(use: boolean, die?: ReactionAnswer['die']): ReactionAnswer {
    if (!use) {
      return { use: false };
    }
    const row = this.chosen();
    const hellish = this.asked().prompt.case === 'hellishRebuke';
    const racial = hellish && row?.level === RACIAL_LEVEL;
    return {
      use: true,
      ...(row && !racial ? { slot: { level: row.level, pact: row.pact } } : {}),
      ...(racial ? { useRacial: true } : {}),
      ...(this.isFeatherFall() ? { creatureIds: [...this.picked()] } : {}),
      ...(this.maneuverKey() ? { maneuverKey: this.maneuverKey() } : {}),
      ...(die ? { die } : {}),
    };
  }

  protected async use(die?: ReactionAnswer['die']): Promise<void> {
    if (this.busy() || !this.canUse()) {
      return;
    }
    // The kinds that roll a die at once (Palavras de Interrupção, Defletir Projéteis): the app rolls it unless the
    // person rolls their own dice.
    let rolled = die;
    if (!rolled && dieSidesOf(this.asked()) > 0) {
      if (this.canApp && this.preferApp) {
        rolled = { inApp: true };
      } else {
        this.goRoll();
        return;
      }
    }
    await this.send(this.build(true, rolled));
  }

  /**
   * The player moves to the die: the server is told (the master's clock would pass this reaction at 30 s, in the
   * middle of the roll) and the longer wait counts from now. Telling is best effort: if it fails, the roll goes on
   * with the 30 s clock, as before.
   */
  private goRoll(): void {
    this.step.set('roll');
    const w = this.asked();
    if (this.rollingAt() !== null || !isOptionalReaction(w)) {
      return;
    }
    this.rollingAt.set(Date.now());
    void this.tellRolling(w.id);
  }

  private async tellRolling(windowId: string): Promise<void> {
    try {
      const e = await this.api.markReactionRolling(
        this.data.campaignId,
        this.data.encounterId,
        windowId,
      );
      this.data.state.apply(e);
    } catch {
      // The window may have been answered meanwhile; the stream shows the truth.
      this.rollingAt.set(null);
    }
  }

  protected async pass(): Promise<void> {
    if (this.busy()) {
      return;
    }
    await this.send(this.build(false));
  }

  private async send(answer: ReactionAnswer): Promise<void> {
    const w = this.asked();
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.answerReaction(
        this.data.campaignId,
        this.data.encounterId,
        w.id,
        answer,
        this.keys.keyFor({ window: w.id, answer }),
      );
      this.data.state.apply(res.encounter);
      if (!answer.use) {
        this.sheet.close();
        return;
      }
      if (res.result) {
        this.afterSlot.set(this.slotAfter(answer));
        this.nextWindowId.set(res.result.nextWindowId);
        this.answer.set({ kind: 'reaction', result: res.result });
        // The dice its result sheet shows, tumbling (a physical die is never animated).
        this.play(() => showOfReaction(res.result));
      }
      this.step.set('result');
      this.typingSave.set(false);
    } catch (err) {
      await this.failed(err, answer, 'usar a reação');
    } finally {
      this.busy.set(false);
    }
  }

  protected async concentrate(how: ConcentrationAnswer): Promise<void> {
    if (this.busy()) {
      return;
    }
    const w = this.asked();
    const extra = this.extra.take(how.kind === 'typed');
    if (extra === null) {
      this.error.set(this.extra.missingText());
      return;
    }
    const sent: ConcentrationAnswer =
      how.kind === 'typed' && extra.length > 0 ? { ...how, extra } : how;
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.resolveConcentrationSave(
        this.data.campaignId,
        this.data.encounterId,
        w.id,
        sent,
        this.keys.keyFor({ window: w.id, how: sent }),
      );
      this.data.state.apply(res.encounter);
      if (res.lucky || res.offer) {
        // Nothing was written: the window stays open and the question comes (Lucky's first).
        this.luckyOffer.set(res.lucky ?? null);
        this.offer.set(res.offer ?? null);
        this.typingSave.set(false);
        return;
      }
      if (!res.result) {
        // Left to the master: nothing more to read here.
        this.sheet.close();
        return;
      }
      this.showSave(res.result);
    } catch (err) {
      const more = this.extra.fromRefusal(err);
      if (more) {
        this.error.set(more);
        return;
      }
      await this.failed(err, null, 'resolver o teste de concentração');
    } finally {
      this.busy.set(false);
    }
  }

  private showSave(result: ConcentrationSaveResult): void {
    this.answer.set({ kind: 'concentration', result });
    this.play(() => showOfConcentration(result));
    this.step.set('result');
    this.typingSave.set(false);
  }

  /** "Rolar de novo" (rolled in the app, or the typed face) or "Ficar com o 1": the save is settled now with the new d20
   * (or the 1), or, when the character holds a Bardic Inspiration die, the die's question comes next, on the new roll. */
  protected async answerLucky(reroll: boolean, die: LuckyRoll | null): Promise<void> {
    const offer = this.luckyOffer();
    if (!offer || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.resources.answerOutsideLucky(
        this.data.campaignId,
        offer.holdId,
        reroll,
        die,
        this.luckyKey.keyFor({ hold: offer.holdId, reroll, die }),
      );
      const settled = res.result.case === 'concentrationSave' ? res.result.value : null;
      if (!settled?.encounter || (!settled.result && !settled.inspirationOffer)) {
        throw new Error('AnswerOutsideLucky answered without its concentration save');
      }
      this.data.state.apply(settled.encounter);
      this.luckyKey.renew();
      this.luckyOffer.set(null);
      if (settled.inspirationOffer) {
        this.offer.set(settled.inspirationOffer);
        const again = die && 'inApp' in die ? luckyReroll(settled.inspirationOffer.d20) : undefined;
        if (again) {
          // The d20 the app rolled again, tumbling: the new face alone, "1 → 14" (the die's question shows the rest).
          this.play(() =>
            showOfDie(
              'Sortudo',
              settled.inspirationOffer?.d20?.diceSides ?? 0,
              again.to,
              `${again.from} → ${again.to}`,
            ),
          );
        }
        return;
      }
      this.showSave(settled.result!);
    } catch (err) {
      this.error.set(classResourceErrorMessage(err, 'responder ao Sortudo'));
    } finally {
      this.busy.set(false);
    }
  }

  /** The answer about the die: the save is settled now, with the d20 it rolled and the die when it was used. */
  protected async answerDie(use: boolean, die: InspirationRoll | null): Promise<void> {
    const offer = this.offer();
    if (!offer || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.resources.answerOutsideInspiration(
        this.data.campaignId,
        offer.holdId,
        use,
        die,
        this.dieKey.keyFor({ hold: offer.holdId, use, die }),
      );
      const settled = res.result.case === 'concentrationSave' ? res.result.value : null;
      if (!settled?.encounter || !settled.result) {
        throw new Error('AnswerOutsideInspiration answered without its concentration save');
      }
      this.data.state.apply(settled.encounter);
      this.offer.set(null);
      this.showSave(settled.result);
    } catch (err) {
      this.error.set(classResourceErrorMessage(err, 'responder sobre a Inspiração de Bardo'));
    } finally {
      this.busy.set(false);
    }
  }

  /** The dice tumble; whatever happens here never turns an answered reaction into an error. */
  private play(make: () => RollShow | null): void {
    try {
      const show = make();
      if (show) {
        this.animator.play(show);
      }
    } catch {
      // The animation is a nicety: the result is already on the sheet.
    }
  }

  /** A refusal: the die Counterspell needed asks for itself; an answer out of order reads the combat again. */
  private async failed(err: unknown, answer: ReactionAnswer | null, what: string): Promise<void> {
    const blocked = encounterBlocked(err);
    if (blocked?.reason === EncounterBlockedReason.REACTION_NEEDS_ROLL && answer && !answer.die) {
      this.goRoll();
      return;
    }
    this.error.set(combatErrorMessage(err, what));
    if (blocked?.reason === EncounterBlockedReason.NOT_YOUR_TURN_TO_ANSWER) {
      try {
        const fresh = await this.api.get(this.data.campaignId);
        if (fresh) {
          this.data.state.apply(fresh);
        }
      } catch {
        // The stream's next event reads it again.
      }
    }
  }

  /** The slot the answer spent, for "Espaços de 3º nível: 1 livre de 2". */
  private slotAfter(answer: ReactionAnswer): SlotAfter | null {
    const row = this.chosen();
    if (!answer.slot || !row) {
      return null;
    }
    return { level: row.level, free: Math.max(0, row.free - 1), total: row.total };
  }

  protected back(): void {
    this.step.set('ask');
    this.error.set('');
  }

  /** "Devolver (1 de chi)": the page opens the attack, which names this window. */
  protected throwBack(): void {
    const id = this.nextWindowId();
    if (id) {
      this.sheet.close({ throwWindowId: id });
    }
  }

  /** "Guardar a flecha": the second step is passed. */
  protected async keepMissile(): Promise<void> {
    const id = this.nextWindowId();
    if (!id || this.busy()) {
      this.sheet.close();
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const answer: ReactionAnswer = { use: false };
      this.data.state.apply(
        (
          await this.api.answerReaction(
            this.data.campaignId,
            this.data.encounterId,
            id,
            answer,
            this.keys.keyFor({ window: id, answer }),
          )
        ).encounter,
      );
      this.sheet.close();
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'guardar a flecha'));
    } finally {
      this.busy.set(false);
    }
  }

  protected done(): void {
    this.sheet.close();
  }

  /** Esc passes (or goes back, or closes the result); the arrows change the slot from the buttons. */
  protected onKey(event: KeyboardEvent): void {
    if (event.key === 'Escape') {
      event.preventDefault();
      this.escape();
    } else if (ARROWS.has(event.key)) {
      this.arrow(event);
    }
  }

  private escape(): void {
    switch (this.stage()) {
      case 'ask':
        if (!this.isConcentration()) {
          void this.pass();
        }
        break;
      case 'roll':
        this.back();
        break;
      case 'result':
        if (this.shown()?.throwBack) {
          void this.keepMissile();
        } else {
          this.done();
        }
        break;
      default:
        this.done();
    }
  }

  /** An arrow on a radio is the browser's own; from a button it picks the next slot (the list wraps). */
  private arrow(event: KeyboardEvent): void {
    const target = event.target as HTMLElement | null;
    if (this.stage() !== 'ask' || target?.tagName === 'INPUT') {
      return;
    }
    const rows = this.rows().filter((r) => r.enabled);
    const at = rows.findIndex(
      (r) => r.level === this.chosen()?.level && r.pact === this.chosen()?.pact,
    );
    if (rows.length < 2 || at < 0) {
      return;
    }
    event.preventDefault();
    const forward = event.key === 'ArrowDown' || event.key === 'ArrowRight';
    this.chosen.set(rows[(at + (forward ? 1 : rows.length - 1)) % rows.length]);
  }
}
