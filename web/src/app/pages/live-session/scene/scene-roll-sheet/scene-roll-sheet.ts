import { Component, ElementRef, computed, effect, inject, signal, viewChild } from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import type { Observable } from 'rxjs';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type { HeldRollOffer } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { AnswerHeldRollResponse } from '../../../../../gen/meurpg/play/v1/resources_pb';
import type { SceneActionView, SceneRoll } from '../../../../../gen/meurpg/play/v1/scene_pb';
import { effectivePreference } from '../../../../core/campaigns/dice-labels';
import { ActionKey } from '../../../../core/connect/idempotency';
import { ExtraDiceState } from '../../../../core/effects/extra-dice-state';
import { rollDiceFor, sceneRollKind } from '../../../../core/effects/roll-die';
import type { RollDie } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { RollDieOffer } from '../../../../shared/roll-die-offer/roll-die-offer';
import { checkFaces, hasModeInfo, needsTwoD20 } from '../../../../core/play/check-roll';
import { HeldRollController } from '../../../../core/held-rolls/held-roll-controller';
import type { HeldAnswer } from '../../../../core/held-rolls/held-roll-offer';
import { ResourceClient } from '../../../../core/resources/resources-client';
import { SceneClient, type SceneDie } from '../../../../core/play/scene-client';
import { sceneBlocked, sceneErrorMessage } from '../../../../core/play/scene-errors';
import type { SceneState } from '../../../../core/play/scene-state';
import { treatedPreview } from '../../../../core/combat/combat-dice';
import { luckyText } from '../../../../core/combat/lucky';
import {
  passText,
  sceneRollFormula,
  sceneRollMark,
  sceneRollNote,
  sceneRollSpeech,
  signedBonus,
} from '../../../../core/play/scene-view';
import { actionSubtitle, actionTitle } from '../../../../core/maps/scene-actions';
import { joinDots } from '../../../../core/format/text';
import { HeldRollPrompt } from '../../../../shared/held-roll-prompt/held-roll-prompt';
import { rerollShow } from '../../../../shared/held-roll-prompt/held-roll-shows';
import { RollAnimator, showOfDice } from '../../../../shared/roll-overlay/roll-animator';
import { mediaQuery } from '../../../../shared/map-view/media-query';
import { ExtraDice } from '../../effects/extra-dice/extra-dice';
import { MultiRoll, type RollField } from '../../../../shared/multi-roll/multi-roll';
import { RollPicker } from '../../../../shared/roll-picker/roll-picker';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../../../../shared/sheet/sheet-host';
import { CheckMode } from '../check-mode/check-mode';

/** What the player's block hands the roll sheet. */
export interface SceneRollSheetData {
  readonly campaignId: string;
  readonly action: SceneActionView;
  readonly diceMode: DiceMode;
  readonly preference: DicePreference;
  readonly state: SceneState;
  /** The keys of the skills Talento Confiável raises for this character (empty without it): the typed die's preview shows "6 → 10". */
  readonly reliableTalent?: readonly string[];
  /** The table dice the character may add to a roll (`DerivedSheet.roll_dice`); the sheet offers those that fit this action. */
  readonly rollDice?: readonly RollDie[];
}

/** The roll sheet of one action: a bottom sheet on a phone, a dialog from a
 * tablet up. It answers `true` when the roll was made. */
export function openSceneRollSheet(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: SceneRollSheetData,
): Observable<boolean | undefined> {
  return openSheet<SceneRollSheet, SceneRollSheetData, boolean>(
    dialog,
    bottomSheet,
    SceneRollSheet,
    {
      data,
      ariaLabel: `Rolar ${actionTitle(data.action)}`,
    },
  );
}

/**
 * Rolling one action of the scene (E7-04, MR-015, RN-18): the check and the
 * player's own bonus, then the two ways to roll as the combat has them
 * ("Rolar no app" is the filled button, "Digitar o resultado" a link; the
 * preference "Como você rola os dados" picks which opens first, a forced mode
 * hides the other). A typed die takes 1 to 20 and the total shows live. The
 * result is the player's own total with its formula, and "Passou · CD 12" or
 * "Não passou · CD 10" only when the master shows the DC (RN-20); otherwise
 * there is no DC and no "passou". The key is made once, so a tap sent again after a lost answer never
 * rolls twice. Focus: the title opens first, and the result's "Voltar à cena".
 */
@Component({
  selector: 'app-scene-roll-sheet',
  imports: [
    CheckMode,
    ExtraDice,
    RollDieOffer,
    HeldRollPrompt,
    MatButtonModule,
    MatIconModule,
    MultiRoll,
    RollPicker,
    SheetFrame,
  ],
  templateUrl: './scene-roll-sheet.html',
  styleUrl: './scene-roll-sheet.scss',
})
export class SceneRollSheet {
  private readonly api = inject(SceneClient);
  private readonly resources = inject(ResourceClient);
  private readonly animator = inject(RollAnimator);
  private readonly sheet = injectSheet<SceneRollSheetData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly action = this.data.action;

  protected readonly typing = signal(false);
  /** A short screen (320×568): the typed form uses the picker's tighter size, so the field,
   * the live total and the footer all fit with nothing covered (E7-04). */
  protected readonly short = mediaQuery('(max-height: 600px)');
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');
  protected readonly roll = signal<SceneRoll | null>(null);
  /** The d20 was rolled and kept and the roll waits for a question: the halfling's Lucky about a 1 ("Tirou 1: rolar de
   * novo?") first, then the Bardic Inspiration die's; the player decides before the result. */
  protected readonly held = new HeldRollController({
    send: (holdId, answer, key) =>
      this.resources.answerHeldRoll(this.data.campaignId, holdId, answer, key),
    settle: (res, done) => this.answeredHeld(res, done.answer),
    busy: this.busy,
    error: this.error,
  });
  /** The server said the roll takes two d20 (advantage or disadvantage with a real die): the form shows two fields. */
  protected readonly pair = signal(false);
  /** The d4 an effect adds (Orientação, Bênção) with physical dice: asked when the server says the roll takes them. */
  protected readonly extra = new ExtraDiceState();
  /** The table dice that fit this action: a maneuver limited to other checks is not offered. */
  protected readonly dice = rollDiceFor(
    this.data.rollDice ?? [],
    sceneRollKind(this.data.action.key),
  );
  protected readonly pairFields: readonly RollField[] = [
    { key: 'first', label: 'Primeiro d20', min: 1, max: 20 },
    { key: 'second', label: 'Segundo d20', min: 1, max: 20 },
  ];
  protected readonly faces = computed(() => checkFaces(this.roll()?.roll));
  protected readonly modeInfo = computed(() => {
    const r = this.roll();
    return !!r && hasModeInfo(r.mode, r.sources, this.faces());
  });

  private readonly key = new ActionKey();
  private readonly back = viewChild('back', { read: ElementRef<HTMLButtonElement> });
  private readonly frame = viewChild(SheetFrame);

  protected readonly canApp = this.data.diceMode !== DiceMode.PHYSICAL;
  protected readonly canType = this.data.diceMode !== DiceMode.APP;
  protected readonly preferApp =
    effectivePreference(this.data.diceMode, this.data.preference) === DicePreference.APP;
  protected readonly bonus = this.action.bonus ?? 0;
  protected readonly bonusText = signedBonus(this.bonus);
  /** "1d20 + 6", the signs and numbers held together. */
  protected readonly diceLine = `1d20\u00a0${this.bonus < 0 ? '−' : '+'}\u00a0${Math.abs(this.bonus)}`;
  protected readonly name = actionTitle(this.action);
  protected readonly checkLine = actionSubtitle(this.action);

  protected readonly title = computed(() => {
    if (this.roll()) {
      return this.name;
    }
    return this.typing() ? 'Digite o resultado do dado' : this.name;
  });
  protected readonly subtitle = computed(() => {
    const r = this.roll();
    if (r) {
      return joinDots([this.checkLine, r.roll?.physical ? 'dado físico' : 'rolado no app']);
    }
    return this.typing()
      ? joinDots([this.name, this.action.checkName])
      : joinDots([this.checkLine, `bônus ${this.bonusText}`]);
  });
  /** "Sortudo: 1 → 14": the d20 the halfling's Lucky rolled again; the formula and the total are already the new ones. */
  protected readonly lucky = computed(() => luckyText(this.roll()?.roll));
  /** "20 natural" / "1 natural" of the player's own d20; information only. */
  protected readonly natural = computed(() => {
    const r = this.roll();
    return r ? sceneRollMark(r) : '';
  });
  protected readonly formula = computed(() => {
    const r = this.roll();
    return r ? sceneRollFormula(r) : '';
  });
  /** A feature raised the d20 (Talento Confiável): the sentence under the result and what a screen reader reads in the formula's place. */
  protected readonly treatedNote = computed(() => {
    const r = this.roll();
    return r ? sceneRollNote(r) : '';
  });
  protected readonly treatedSpeech = computed(() => {
    const r = this.roll();
    return r ? sceneRollSpeech(r) : '';
  });
  /** "Passou · CD 12", only when the master shows the DC: the server sends `passed` then and not otherwise. */
  protected readonly pass = computed(() => {
    const passed = this.roll()?.passed;
    return passed === undefined ? null : { ok: passed, text: passText(passed, this.action.dc) };
  });
  /** The typed d20's live total, with Talento Confiável's account where the character's feature raises this check and the number is below 10. */
  protected readonly typedFormula = this.data.reliableTalent?.includes(this.action.key)
    ? (face: number) => treatedPreview(face, this.bonus, 'feature:reliable-talent')
    : null;
  protected readonly rollLabel = `Role 1d20 para ${this.action.checkName} (${this.bonusText})`;

  constructor() {
    // After the result the focus goes to its one action, as soon as it is drawn.
    effect(() => this.back()?.nativeElement.focus());
    // An error opens at the top of the scrolling body, where it is seen.
    effect(() => {
      if (this.error()) {
        this.frame()?.scrollToTop();
      }
    });
  }

  /** "Usar <dado>": the die ticked joins the roll; none ticked leaves it as it is. */
  protected useDie(key: string): void {
    this.extra.maneuver.set(this.dice.find((d) => d.key === key) ?? null);
  }

  protected async rollWith(die: SceneDie): Promise<void> {
    if (this.busy()) {
      return;
    }
    const extra = this.extra.take(!('inApp' in die));
    if (extra === null) {
      this.error.set(this.extra.missingText());
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const made = await this.api.roll(
        this.data.campaignId,
        this.action.id,
        die,
        this.key.keyFor({ action: this.action.id, die, extra, maneuver: this.extra.maneuverKey() }),
        extra,
        this.extra.maneuverKey(),
      );
      if (made.$typeName !== 'meurpg.play.v1.SceneRoll') {
        this.hold(made);
        return;
      }
      this.finish(made);
    } catch (err) {
      if (needsTwoD20(err) && 'face' in die) {
        // Not an error: this roll has advantage or disadvantage, and the form asks for both dice.
        this.pair.set(true);
        this.typing.set(true);
        return;
      }
      const more = this.extra.fromRefusal(err);
      if (more) {
        this.error.set(more);
        return;
      }
      this.error.set(sceneErrorMessage(err, 'rolar a ação'));
      if (sceneBlocked(err)) {
        // The scene is not what the sheet's row shows (closed, no attempts left): read it again.
        void this.data.state.refresh();
      }
    } finally {
      this.busy.set(false);
    }
  }

  /** The d20 is held for a question, the halfling's Lucky about a 1 (first) or the Bardic Inspiration die: its face alone
   * tumbles, no total and no word. */
  private hold(question: HeldRollOffer): void {
    this.held.ask(question);
    this.typing.set(false);
    const held = showOfDice(this.name, question.d20);
    if (held) {
      this.animator.play(held);
    }
  }

  /** The roll written, as the sheet now shows it: the d20 (with the die added in the total, or the one Lucky rolled
   * again); "Passou"/"Falhou" only when the sheet shows it (the master shows the DC). */
  private finish(made: SceneRoll): void {
    this.roll.set(made);
    this.held.ask(null);
    this.typing.set(false);
    const show = showOfDice(this.name, made.roll, {
      withTotal: true,
      outcome:
        made.passed === undefined
          ? undefined
          : { word: made.passed ? 'Passou' : 'Falhou', good: made.passed },
    });
    if (show) {
      this.animator.play(show);
    }
    // The row under the sheet turns into "Rolada" as soon as the read comes back.
    void this.data.state.refresh();
  }

  /** What the answer to the question brought: the roll written now (with the die when it was used, or the new d20), or,
   * when the character holds a Bardic Inspiration die, the die's question next, on the new roll (the d20 the app rolled
   * again tumbles, "1 → 14"). */
  private answeredHeld(res: AnswerHeldRollResponse, answer: HeldAnswer): void {
    const settled = res.result.case === 'sceneCheck' ? res.result.value : null;
    if (!settled || (!settled.roll && !res.next)) {
      throw new Error('AnswerHeldRoll answered without its scene check');
    }
    if (res.next) {
      this.typing.set(false);
      const show = answer.kind === 'lucky' ? rerollShow(res.next.d20, answer.roll) : null;
      if (show) {
        this.animator.play(show);
      }
      return;
    }
    this.finish(settled.roll!);
  }

  protected close(): void {
    this.sheet.close(this.roll() !== null || this.held.offer() !== null);
  }
}
