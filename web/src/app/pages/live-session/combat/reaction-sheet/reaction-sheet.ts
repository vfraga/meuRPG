import { NgTemplateOutlet } from '@angular/common';
import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  ElementRef,
  Injector,
  computed,
  effect,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type { ReactionWindow } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { effectivePreference } from '../../../../core/campaigns/dice-labels';
import { type SlotRow, defaultSlot, lastSlotWarning } from '../../../../core/combat/cast-flow';
import { CombatClient } from '../../../../core/combat/combat-client';
import { type ReactionAnswer } from '../../../../core/combat/combat-client-types';
import { luckyHeldLine } from '../../../../core/combat/lucky-question';
import {
  type ConcentrationMode,
  type PromptView,
  type ResultView,
  concentrationMode,
  concentrationModeLine,
  concentrationResultView,
  dieSidesOf,
  isOpen,
  promptView,
  resultView,
  throwText,
} from '../../../../core/combat/reactions';
import {
  isOptionalReaction,
  playerAutoPassText,
  playerRollingSecondsLeft,
  playerRollingText,
  playerSecondsLeft,
} from '../../../../core/combat/reaction-autopass';
import { ResourceClient } from '../../../../core/resources/resources-client';
import { InspirationOffer } from '../../inspiration/inspiration-offer';
import { LuckyPrompt } from '../lucky-prompt/lucky-prompt';
import { SlotPicker } from '../../../../shared/slot-picker/slot-picker';
import { RollAnimator } from '../../../../shared/roll-overlay/roll-animator';
import { ExtraDice } from '../../effects/extra-dice/extra-dice';
import { MultiRoll, type RollField } from '../multi-roll/multi-roll';
import { RollPicker } from '../../../../shared/roll-picker/roll-picker';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../../shared/sheet/sheet-host';
import { ReactionFlow } from './reaction-flow';
import type { ReactionSheetData, ReactionSheetResult } from './reaction-sheet-data';
import {
  type Stage,
  answerOf,
  escapeAction,
  fallingRowsOf,
  firstManeuverKey,
  initialPicked,
  maneuverOptionsOf,
  maxTargetsOf,
  nextSlot,
  slotRowsOf,
  throwBackCard,
} from './reaction-sheet-view';

// The data and the result live in their own file; the page imports them from here.
export type { ReactionSheetData, ReactionSheetResult } from './reaction-sheet-data';

/** How often the prompt's countdown is redrawn. */
const AUTO_PASS_TICK_MS = 1000;

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
  templateUrl: './reaction-sheet.html',
  styleUrl: './reaction-sheet.scss',
})
export class ReactionSheet {
  private readonly lazyInjector = inject(Injector);
  private readonly sheet = injectSheet<ReactionSheetData, ReactionSheetResult>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;

  /** The window being asked: the first one, then the second step the answer opened. */
  protected readonly asked = signal<ReactionWindow>(this.data.window);

  /** The calls and what they leave: the step, what is in the air, what came back (`ReactionFlow`). */
  private readonly flow = new ReactionFlow({
    data: this.data,
    api: inject(CombatClient),
    resources: () => this.lazyInjector.get(ResourceClient),
    animator: inject(RollAnimator),
    close: (result) => this.sheet.close(result),
    asked: () => this.asked(),
    chosen: () => this.chosen(),
  });
  protected readonly busy = this.flow.busy;
  protected readonly error = this.flow.error;
  protected readonly typingSave = this.flow.typingSave;
  protected readonly offer = this.flow.offer;
  protected readonly luckyOffer = this.flow.luckyOffer;
  protected readonly extra = this.flow.extra;
  protected readonly rollingAt = this.flow.rollingAt;
  private readonly step = this.flow.step;
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  /** The held d20, above the Lucky question. */
  protected readonly luckyHeld = computed(() => luckyHeldLine(this.luckyOffer()));
  protected readonly picked = signal<ReadonlySet<string>>(initialPicked(this.data.window));

  /** The window came with a read, after a reload: the prompt is back and says so. */
  protected readonly refreshed =
    !this.flow.secondStep && !this.data.state.wasAnnounced(this.data.window.id);
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
  protected readonly rows = computed<readonly SlotRow[]>(() =>
    slotRowsOf(this.asked(), this.data.usage, this.data.pact),
  );
  protected readonly chosen = signal<SlotRow | null>(defaultSlot(this.rows()));
  /** The maneuvers a reduction offers (the first is picked), when the window is a maneuver reduction. */
  protected readonly maneuverOptions = computed(() => maneuverOptionsOf(this.asked()));
  protected readonly maneuverKey = signal(firstManeuverKey(this.asked()));
  protected readonly warning = computed(() =>
    this.asked().prompt.case === 'shield' ? lastSlotWarning(this.chosen(), null) : '',
  );

  protected readonly fallingRows = computed(() => fallingRowsOf(this.asked()));
  protected readonly maxTargets = computed(() => maxTargetsOf(this.asked()));

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
      ?.reactionWindows.find((x) => x.id === this.flow.nextWindowId() && isOpen(x));
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
  protected readonly stage = computed<Stage>(() => {
    if (this.step() === 'result') {
      return 'result';
    }
    return this.present() || this.busy() ? this.step() : 'gone';
  });

  protected readonly shown = computed<ResultView | null>(() => {
    const a = this.flow.answered();
    if (!a) {
      return this.flow.secondStep ? throwBackCard(this.data.round) : null;
    }
    return a.kind === 'concentration'
      ? concentrationResultView(a.result, this.data.round)
      : resultView(a.result, this.data.window, {
          round: this.data.round,
          after: this.flow.afterSlot(),
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
    return answerOf(
      use,
      {
        asked: this.asked(),
        row: this.chosen(),
        creatureIds: this.isFeatherFall() ? [...this.picked()] : null,
        maneuverKey: this.maneuverKey(),
      },
      die,
    );
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
        this.flow.goRoll();
        return;
      }
    }
    await this.flow.send(this.build(true, rolled));
  }

  protected async pass(): Promise<void> {
    if (this.busy()) {
      return;
    }
    await this.flow.send(this.build(false));
  }

  protected readonly concentrate = this.flow.concentrate.bind(this.flow);
  protected readonly answerLucky = this.flow.answerLucky.bind(this.flow);
  protected readonly answerDie = this.flow.answerDie.bind(this.flow);
  protected readonly back = this.flow.back.bind(this.flow);
  protected readonly throwBack = this.flow.throwBack.bind(this.flow);
  protected readonly keepMissile = this.flow.keepMissile.bind(this.flow);

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
    switch (escapeAction(this.stage(), this.isConcentration(), !!this.shown()?.throwBack)) {
      case 'pass':
        void this.pass();
        break;
      case 'back':
        this.back();
        break;
      case 'keep':
        void this.keepMissile();
        break;
      case 'close':
        this.done();
        break;
      default:
      // A concentration save is decided by its owner: Esc does nothing.
    }
  }

  /** An arrow on a radio is the browser's own; from a button it picks the next slot (the list wraps). */
  private arrow(event: KeyboardEvent): void {
    const target = event.target as HTMLElement | null;
    if (this.stage() !== 'ask' || target?.tagName === 'INPUT') {
      return;
    }
    const forward = event.key === 'ArrowDown' || event.key === 'ArrowRight';
    const next = nextSlot(this.rows(), this.chosen(), forward);
    if (next) {
      event.preventDefault();
      this.chosen.set(next);
    }
  }
}
