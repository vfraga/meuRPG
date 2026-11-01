import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  signal,
  Injector,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { Code, ConnectError } from '@connectrpc/connect';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { type ReactionWindow } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { AnswerHeldRollResponse } from '../../../../../gen/meurpg/play/v1/resources_pb';
import type { EffectSaveResult } from '../../../../../gen/meurpg/play/v1/lasting_effects_pb';
import { effectivePreference } from '../../../../core/campaigns/dice-labels';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { luckyText } from '../../../../core/combat/lucky';
import { HeldRollController } from '../../../../core/held-rolls/held-roll-controller';
import type { HeldAnswer } from '../../../../core/held-rolls/held-roll-offer';
import { isOpen } from '../../../../core/combat/reactions';
import { ActionKey } from '../../../../core/connect/idempotency';
import { type EffectSaveAnswer, EffectsClient } from '../../../../core/effects/effects-client';
import {
  type ExtraDieField,
  dieFields,
  effectSaveView,
  genericDieFields,
  missingDice,
  saveFormula,
} from '../../../../core/effects/effects';
import { zoneCasterLabel } from '../../../../core/effects/zone-save';
import { dcLabel } from '../../../../core/format/save-dc';
import { ResourceClient } from '../../../../core/resources/resources-client';
import { HeldRollPrompt } from '../../../../shared/held-roll-prompt/held-roll-prompt';
import { rerollShow } from '../../../../shared/held-roll-prompt/held-roll-shows';
import { RollAnimator, type RollShow } from '../../../../shared/roll-overlay/roll-animator';
import { MultiRoll, type RollField } from '../../../../shared/multi-roll/multi-roll';
import { RollPicker } from '../../../../shared/roll-picker/roll-picker';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../../shared/sheet/sheet-host';
import { ExtraDice } from '../extra-dice/extra-dice';
import { rollDiceFor, rollDieField } from '../../../../core/effects/roll-die';
import type { RollDie } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { RollDieOffer } from '../../../../shared/roll-die-offer/roll-die-offer';

/** What the page hands the sheet of an effect's saving throw. */
export interface EffectSaveSheetData {
  readonly campaignId: string;
  readonly encounterId: string;
  /** The window to answer, kind EFFECT_SAVE, as the server sent it. */
  readonly window: ReactionWindow;
  readonly round: number;
  readonly state: CombatState;
  readonly diceMode: DiceMode;
  readonly preference: DicePreference;
  /** The table dice the character may add to a roll (`DerivedSheet.roll_dice`); the save offers those that fit its ability. */
  readonly rollDice?: readonly RollDie[];
}

type Stage = 'ask' | 'result' | 'gone';

/** The d4 of Bênção and Perdição. */
const EFFECT_DIE_FACES = 4;

/**
 * The saving throw an effect asks at a turn (RN-22, board W7-Ea 3): "Fim do seu turno" or "Começo do seu turno", a
 * window of reaction PM-04 of kind EFFECT_SAVE. It opens by itself for the player of the target and says the ability,
 * the modifier and what a pass does, never a DC. Three ways to answer: "Rolar no app", "Digitar o resultado" (the
 * d20 of a physical die, plus the d4 an effect adds, and the second d20 with advantage or disadvantage) and "Deixar o
 * mestre rolar por mim". A save that fails by itself (Paralisado, Força and Destreza) has no d20: one button, "Continuar".
 * The result says "Passou" or "Falhou", the d20 and the sum, and what changed. The answer has its own key, made once,
 * so a repeated tap never answers twice. The master may answer first: the sheet then says so.
 */
@Component({
  selector: 'app-effect-save-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [
    ExtraDice,
    RollDieOffer,
    HeldRollPrompt,
    MatButtonModule,
    MatIconModule,
    MultiRoll,
    RollPicker,
    SheetFrame,
  ],
  host: { '(keydown.escape)': 'close()' },
  templateUrl: './effect-save-sheet.html',
  styleUrl: './effect-save-sheet.scss',
})
export class EffectSaveSheet {
  private readonly api = inject(EffectsClient);
  /** Read when the player answers about the die: a sheet that never asks does not build the client. */
  private readonly lazyInjector = inject(Injector);
  private get resources(): ResourceClient {
    return this.lazyInjector.get(ResourceClient);
  }
  private readonly animator = inject(RollAnimator);
  private readonly sheet = injectSheet<EffectSaveSheetData, void>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;

  protected readonly busy = signal(false);
  protected readonly error = signal('');
  protected readonly typing = signal(false);
  protected readonly result = signal<EffectSaveResult | null>(null);
  private readonly done = signal(false);
  /** The d20 was rolled and kept and the save waits for a question: the halfling's Lucky ("Tirou 1: rolar de novo?")
   * first, then the Bardic Inspiration die's; the player decides before the result. */
  protected readonly held = new HeldRollController({
    send: (holdId, answer, key) =>
      this.resources.answerHeldRoll(this.data.campaignId, holdId, answer, key),
    settle: (res, done) => this.answeredHeld(res, done.answer),
    busy: this.busy,
    error: this.error,
  });
  /** "Sortudo: 1 → 14" under the result: the d20 Lucky rolled again (the faces and the total are the new ones). */
  /** "CD 14" under the result, when the server sent the DC with it. */
  protected readonly resultDc = computed(() => dcLabel(this.result()?.dc));
  protected readonly luckyLine = computed(() => luckyText(this.result() ?? undefined));
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly extraFaces = signal<readonly (number | null)[]>([]);
  /** The d4 asked when the server refused a roll for lack of them (an effect this player does not read). */
  private readonly asked = signal(0);

  protected readonly canApp = this.data.diceMode !== DiceMode.PHYSICAL;
  protected readonly canType = this.data.diceMode !== DiceMode.APP;
  protected readonly preferApp =
    effectivePreference(this.data.diceMode, this.data.preference) === DicePreference.APP;
  /** The fields of the typed answer are open: the person asked to type, or the campaign rolls physical dice only. */
  protected readonly typingNow = computed(() => this.typing() || !this.canApp);
  protected readonly pair: readonly RollField[] = [
    { key: 'd20-1', label: 'Primeiro d20', min: 1, max: 20 },
    { key: 'd20-2', label: 'Segundo d20', min: 1, max: 20 },
  ];

  private readonly prompt = computed(() => {
    const p = this.data.window.prompt;
    if (p.case !== 'effectSave') {
      throw new Error('EffectSaveSheet needs a window of kind EFFECT_SAVE');
    }
    return p.value;
  });
  protected readonly view = computed(() =>
    effectSaveView(
      this.prompt(),
      this.data.round,
      zoneCasterLabel(this.data.state.encounter(), this.prompt().zoneId),
    ),
  );
  /** The fields of the d4 the prompt lists (or the ones a refusal asked for). */
  protected readonly fields = computed<readonly ExtraDieField[]>(() => {
    const listed = dieFields(
      this.prompt().extraDice.map((d) => ({
        name: d.sourceNamePt,
        faces: d.faces,
        sign: d.sign,
      })),
    );
    const all =
      this.asked() > listed.length
        ? [...listed, ...genericDieFields(this.asked() - listed.length, EFFECT_DIE_FACES)]
        : listed;
    // The table die ticked is always the last die of the roll.
    const die = this.maneuver();
    return die ? [...all, rollDieField(die)] : all;
  });
  /** The table dice that fit a save of this ability. */
  protected readonly dice = computed(() =>
    rollDiceFor(this.data.rollDice ?? [], { kind: 'save', ability: this.prompt().ability }),
  );
  /** The die ticked ("Usar <nome>"); none by default. */
  protected readonly maneuver = signal<RollDie | null>(null);
  protected useDie(key: string): void {
    this.maneuver.set(this.dice().find((d) => d.key === key) ?? null);
  }
  /** The modifier to add to the typed d20 in its live total. */
  protected readonly modifier = computed(() => this.prompt().modifier);

  private readonly present = computed(() =>
    (this.data.state.encounter()?.reactionWindows ?? []).some(
      (w) => w.id === this.data.window.id && isOpen(w),
    ),
  );
  protected readonly stage = computed<Stage>(() => {
    if (this.done()) {
      return 'result';
    }
    return this.present() || this.busy() ? 'ask' : 'gone';
  });
  protected readonly goneText = computed(
    () =>
      this.data.state.reactionNotice()?.text ??
      'Este teste já foi respondido: ele não espera mais a sua rolagem.',
  );
  protected readonly formula = saveFormula;

  /** One key per answer (how it is rolled): the same tap again is a retry, another way is a new request. */
  private readonly keys = new ActionKey();

  constructor() {
    // The first thing to read when the sheet opens is its title; the buttons come after it.
    effect(() => {
      if (this.stage() === 'gone' && this.data.state.reactionNotice()) {
        this.sheet.close();
      }
    });
  }

  /** "Confirmar" of the typed d20 (and the second one, with advantage or disadvantage). */
  protected answerTyped(faces: readonly number[]): Promise<void> {
    const need = this.fields();
    const typed = this.extraFaces();
    if (need.length > 0 && (typed.length !== need.length || typed.some((f) => f === null))) {
      this.error.set(
        need.length === 1
          ? `Digite o resultado do d${need[0].faces} antes de confirmar.`
          : 'Digite o resultado de cada dado extra antes de confirmar.',
      );
      return Promise.resolve();
    }
    return this.answer({
      kind: 'typed',
      face: faces[0],
      ...(faces.length > 1 ? { second: faces[1] } : {}),
      extra: need.length > 0 ? (typed as readonly number[]) : [],
    });
  }

  protected async answer(asked: EffectSaveAnswer): Promise<void> {
    if (this.busy()) {
      return;
    }
    const maneuverKey = this.maneuver()?.key;
    const how: EffectSaveAnswer =
      maneuverKey && (asked.kind === 'app' || asked.kind === 'typed')
        ? { ...asked, maneuverKey }
        : asked;
    const w = this.data.window;
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.rollEffectSave(
        this.data.campaignId,
        this.data.encounterId,
        w.id,
        how,
        this.keys.keyFor({ window: w.id, how }),
      );
      this.data.state.apply(res.encounter);
      if (res.held) {
        // Nothing was written: the d20 alone, and the result after the answer (Lucky's question first).
        this.held.ask(res.held);
        this.typing.set(false);
        return;
      }
      this.keys.renew();
      if (!res.result) {
        // Left to the master: nothing more to read here.
        this.sheet.close();
        return;
      }
      this.result.set(res.result);
      this.animate(res.result);
      this.done.set(true);
      this.typing.set(false);
    } catch (err) {
      this.failed(err);
    } finally {
      this.busy.set(false);
    }
  }

  /** What the answer to the question brought: the save settled now with the new d20 (or the 1), or, when the character
   * holds a Bardic Inspiration die, the die's question next, on the new roll (the d20 the app rolled again tumbles; a told
   * result animates its own d20). */
  private answeredHeld(res: AnswerHeldRollResponse, answer: HeldAnswer): void {
    const settled = res.result.case === 'effectSave' ? res.result.value : null;
    if (!settled?.encounter || (!settled.result && !res.next)) {
      throw new Error('AnswerHeldRoll answered without its saving throw');
    }
    this.data.state.apply(settled.encounter);
    if (res.next) {
      const show = answer.kind === 'lucky' ? rerollShow(res.next.d20, answer.roll) : null;
      if (show) {
        this.animator.play(show);
      }
      return;
    }
    this.keys.renew();
    this.result.set(settled.result!);
    this.animate(settled.result!);
    this.done.set(true);
  }

  /** The d20 the app just rolled, tumbling: the face(s), the total and "Passou" or "Falhou", all of them on this sheet's result. */
  private animate(r: EffectSaveResult): void {
    if (r.autoFail || r.skipped || r.physical) {
      return;
    }
    const faces = r.d20Faces.length === 2 ? r.d20Faces : [r.d20];
    const counted = Math.max(0, faces.indexOf(r.d20));
    const mod = r.modifier === 0 ? '' : ` ${r.modifier < 0 ? '−' : '+'} ${Math.abs(r.modifier)}`;
    const show: RollShow = {
      label: `Teste de resistência de ${this.view().ability}`,
      dice: faces.map((face, i) => ({
        sides: 20,
        face,
        counts: faces.length === 1 || i === counted,
      })),
      line: r.extraDice.length > 0 ? `Total ${r.total}` : `${r.d20}${mod} = ${r.total}`,
      outcome: { word: r.saved ? 'Passou' : 'Falhou', good: r.saved },
    };
    this.animator.play(show);
  }

  /** A refusal: physical dice and a d4 the sheet did not list are asked for; any other says what it is. */
  private failed(err: unknown): void {
    const connectErr = ConnectError.from(err, Code.Unavailable);
    const more = connectErr.code === Code.InvalidArgument ? missingDice(connectErr.rawMessage) : 0;
    if (more > 0) {
      this.asked.set(more);
      this.typing.set(true);
      this.error.set(
        more === 1
          ? 'Este teste leva mais um d4: role-o e digite o resultado.'
          : `Este teste leva mais ${more} d4: role-os e digite os resultados.`,
      );
      return;
    }
    this.error.set(combatErrorMessage(err, 'rolar o teste de resistência'));
  }

  protected close(): void {
    if (!this.busy()) {
      this.sheet.close();
    }
  }
}
