import { Component, ElementRef, computed, effect, inject, signal, viewChild } from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import type { Observable } from 'rxjs';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type {
  LuckyOffer,
  OutsideInspirationOffer,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import type {
  RollSceneCheckResponse,
  SceneActionView,
  SceneRoll,
} from '../../../../../gen/meurpg/play/v1/scene_pb';
import { effectivePreference } from '../../../../core/campaigns/dice-labels';
import { ActionKey } from '../../../../core/connect/idempotency';
import { ExtraDiceState } from '../../../../core/effects/extra-dice-state';
import { checkFaces, hasModeInfo, needsTwoD20 } from '../../../../core/play/check-roll';
import {
  ResourceClient,
  type InspirationRoll,
  type LuckyRoll,
} from '../../../../core/resources/resources-client';
import { classResourceErrorMessage } from '../../../../core/resources/resources-errors';
import { SceneClient, type SceneDie } from '../../../../core/play/scene-client';
import { sceneBlocked, sceneErrorMessage } from '../../../../core/play/scene-errors';
import type { SceneState } from '../../../../core/play/scene-state';
import { treatedPreview } from '../../../../core/combat/combat-dice';
import { LUCKY_TRAIT, luckyText } from '../../../../core/combat/lucky';
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
import { RollAnimator, showOfDice, showOfDie } from '../../../../shared/roll-overlay/roll-animator';
import { mediaQuery } from '../../../../shared/map-view/media-query';
import { ExtraDice } from '../../effects/extra-dice/extra-dice';
import { MultiRoll, type RollField } from '../../combat/multi-roll/multi-roll';
import { RollPicker } from '../../../../shared/roll-picker/roll-picker';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../../../../shared/sheet/sheet-host';
import { CheckMode } from '../check-mode/check-mode';
import { InspirationOffer } from '../../inspiration/inspiration-offer';
import { LuckyOfferCard } from '../../inspiration/lucky-offer';

/** What the player's block hands the roll sheet. */
export interface SceneRollSheetData {
  readonly campaignId: string;
  readonly action: SceneActionView;
  readonly diceMode: DiceMode;
  readonly preference: DicePreference;
  readonly state: SceneState;
  /** The keys of the skills Talento Confiável raises for this character (empty without it): the typed die's preview shows "6 → 10". */
  readonly reliableTalent?: readonly string[];
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
    InspirationOffer,
    LuckyOfferCard,
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
  /** The d20 was rolled and kept: the character holds a Bardic Inspiration die, and the player decides before the result. */
  protected readonly offer = signal<OutsideInspirationOffer | null>(null);
  /** The d20 was rolled and kept because a halfling's d20 shows a 1 ("Tirou 1: rolar de novo?"): asked before the die's. */
  protected readonly luckyOffer = signal<LuckyOffer | null>(null);
  /** The server said the roll takes two d20 (advantage or disadvantage with a real die): the form shows two fields. */
  protected readonly pair = signal(false);
  /** The d4 an effect adds (Orientação, Bênção) with physical dice: asked when the server says the roll takes them. */
  protected readonly extra = new ExtraDiceState();
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
  private readonly answerKey = new ActionKey();
  /** One key per Lucky answer (roll again this way, or keep the 1): the same answer again is a retry. */
  private readonly luckyKey = new ActionKey();
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
        this.key.keyFor({ action: this.action.id, die, extra }),
        extra,
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
  private hold(question: OutsideInspirationOffer | LuckyOffer): void {
    if (question.$typeName === 'meurpg.play.v1.LuckyOffer') {
      this.luckyOffer.set(question);
      this.offer.set(null);
    } else {
      this.offer.set(question);
      this.luckyOffer.set(null);
    }
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
    this.offer.set(null);
    this.luckyOffer.set(null);
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

  /** The answer about the die: the roll is written now, with the die when it was used. */
  protected async answer(use: boolean, die: InspirationRoll | null): Promise<void> {
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
        this.answerKey.keyFor({ hold: offer.holdId, use, die }),
      );
      if (res.result.case !== 'sceneCheck' || !res.result.value.roll) {
        throw new Error('AnswerOutsideInspiration answered without its scene check');
      }
      this.finish(res.result.value.roll);
    } catch (err) {
      this.error.set(classResourceErrorMessage(err, 'responder sobre a Inspiração de Bardo'));
    } finally {
      this.busy.set(false);
    }
  }

  /** "Rolar de novo" (rolled in the app, or the typed face) or "Ficar com o 1": the roll is written now with the new d20,
   * or, when the character holds a Bardic Inspiration die, the die's question comes next, on the new roll. */
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
      if (res.result.case !== 'sceneCheck') {
        throw new Error('AnswerOutsideLucky answered without its scene check');
      }
      this.luckyKey.renew();
      this.afterLucky(res.result.value);
    } catch (err) {
      this.error.set(classResourceErrorMessage(err, 'responder ao Sortudo'));
    } finally {
      this.busy.set(false);
    }
  }

  /** What the Lucky answer came to: the Bardic Inspiration question on the new d20 (the d20 Lucky rolled again tumbles,
   * "1 → 14"), or the roll written. */
  private afterLucky(res: RollSceneCheckResponse): void {
    const next = res.inspirationOffer;
    if (next) {
      this.luckyOffer.set(null);
      this.offer.set(next);
      this.typing.set(false);
      const again = next.d20?.rerolled.find((r) => r.sourceKey === LUCKY_TRAIT);
      const show = again
        ? showOfDie('Sortudo', next.d20?.diceSides ?? 0, again.to, `${again.from} → ${again.to}`)
        : null;
      if (show) {
        this.animator.play(show);
      }
      return;
    }
    if (!res.roll) {
      throw new Error('AnswerOutsideLucky answered without its scene check');
    }
    this.finish(res.roll);
  }

  protected close(): void {
    this.sheet.close(this.roll() !== null || this.offer() !== null || this.luckyOffer() !== null);
  }
}
