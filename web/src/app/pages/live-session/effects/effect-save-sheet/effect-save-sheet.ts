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
import {
  type DiceRoll,
  type LuckyOffer,
  type OutsideInspirationOffer,
  type ReactionWindow,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { EffectSaveResult } from '../../../../../gen/meurpg/play/v1/lasting_effects_pb';
import { effectivePreference } from '../../../../core/campaigns/dice-labels';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { luckyText } from '../../../../core/combat/lucky';
import { luckyHeldLine, luckyReroll } from '../../../../core/combat/lucky-question';
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
import {
  ResourceClient,
  type InspirationRoll,
  type LuckyRoll,
} from '../../../../core/resources/resources-client';
import { classResourceErrorMessage } from '../../../../core/resources/resources-errors';
import { InspirationOffer } from '../../inspiration/inspiration-offer';
import {
  RollAnimator,
  type RollShow,
  showOfDie,
} from '../../../../shared/roll-overlay/roll-animator';
import { LuckyPrompt } from '../../combat/lucky-prompt/lucky-prompt';
import { MultiRoll, type RollField } from '../../combat/multi-roll/multi-roll';
import { RollPicker } from '../../../../shared/roll-picker/roll-picker';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../../shared/sheet/sheet-host';
import { ExtraDice } from '../extra-dice/extra-dice';

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
    InspirationOffer,
    LuckyPrompt,
    MatButtonModule,
    MatIconModule,
    MultiRoll,
    RollPicker,
    SheetFrame,
  ],
  host: { '(keydown.escape)': 'close()' },
  template: `
    <app-sheet-frame
      [title]="view().title"
      [subtitle]="view().subtitle"
      [phone]="inSheet"
      [closable]="!busy()"
      (closed)="close()"
    >
      <ol class="steps" aria-label="Passos do teste">
        <li class="step" [attr.aria-current]="stage() === 'ask' ? 'step' : null">
          <span class="step__dot" [class.step__dot--done]="stage() === 'result'">
            @if (stage() === 'result') {
              <mat-icon aria-hidden="true">check</mat-icon>
            } @else {
              1
            }
          </span>
          Teste
        </li>
        <li class="step" [class.step--todo]="stage() !== 'result'" [attr.aria-current]="stage() === 'result' ? 'step' : null">
          <span class="step__dot">2</span>
          Resultado
        </li>
      </ol>
      @if (error()) {
        <div class="mr-notice mr-notice--danger" role="alert">
          <mat-icon aria-hidden="true">error</mat-icon>
          <p>{{ error() }}</p>
        </div>
      }
      @switch (stage()) {
        @case ('ask') {
          @if (view().autoFail) {
            <div class="mr-notice mr-notice--danger" data-testid="auto-fail">
              <mat-icon aria-hidden="true">error_outline</mat-icon>
              <p><strong>Falha automática.</strong> Não há d20 a rolar neste teste de resistência de {{ view().ability }}.</p>
            </div>
          } @else if (luckyOffer(); as l) {
            <!-- The d20 shows a 1 and the roller is a halfling: it is kept, and Lucky asks before the die's question (SRD 5.1). -->
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
          } @else if (view().handed) {
            <div class="mr-notice mr-notice--neutral" role="status">
              <mat-icon aria-hidden="true">hourglass_top</mat-icon>
              <p>Esperando o mestre: você deixou a rolagem com ele.</p>
            </div>
          } @else {
            <div class="mr-notice mr-notice--neutral">
              <mat-icon aria-hidden="true">info</mat-icon>
              <p>Teste de resistência de <b>{{ view().ability }}</b>. {{ view().consequence }}</p>
            </div>
            <p class="what">
              @if (view().modifier) {
                {{ view().modifier }}
              }
              {{ view().modeLine }}
            </p>
            @if (view().dice.length > 0) {
              <ul class="dice-list" aria-label="Dados que os efeitos somam">
                @for (d of view().dice; track $index) {
                  <li>
                    {{ d.sourceNamePt }}: {{ d.sign < 0 ? 'subtrai' : 'soma' }} 1d{{ d.faces }} a este teste.
                  </li>
                }
              </ul>
            }
            @if (typingNow()) {
              <app-extra-dice [fields]="fields()" [(faces)]="extraFaces" />
              @if (view().mode === 'normal') {
                <app-roll-picker
                  [canApp]="false"
                  [canType]="true"
                  [min]="1"
                  [max]="20"
                  [modifier]="modifier()"
                  [label]="'Resultado do d20 do teste de resistência de ' + view().ability"
                  hint="Role o seu dado e digite o número que saiu (1 a 20)."
                  totalNote="Teste de resistência"
                  [busy]="busy()"
                  (typed)="answerTyped([$event])"
                />
              } @else {
                <app-multi-roll
                  [fields]="pair"
                  [canApp]="false"
                  [canType]="true"
                  [combine]="view().mode === 'advantage' ? 'higher' : 'lower'"
                  hint="Role os dois d20 e digite os dois números, na ordem em que saíram (1 a 20 cada)."
                  [busy]="busy()"
                  (typed)="answerTyped($event)"
                />
              }
            }
          }
        }
        @case ('result') {
          @if (result(); as r) {
            <div class="res" role="status" aria-live="polite">
              @if (r.autoFail) {
                <span class="pill pill--bad"><mat-icon aria-hidden="true">close</mat-icon>Falhou</span>
                <p class="what">Sem rolagem.</p>
              } @else {
                <p class="res__row">
                  <span class="res__die">{{ r.d20 }}</span>
                  <span class="res__total">{{ r.total }}</span>
                  <span class="pill" [class.pill--bad]="!r.saved">
                    <mat-icon aria-hidden="true">{{ r.saved ? 'check' : 'close' }}</mat-icon>{{ r.saved ? 'Passou' : 'Falhou' }}
                  </span>
                </p>
                <p class="what what--small">{{ formula(r) }}{{ r.physical ? ' · dado físico' : '' }}</p>
                @if (luckyLine()) {
                  <p class="what what--small">{{ luckyLine() }}</p>
                }
              }
              @if (r.textPt) {
                <div class="mr-notice" [class.mr-notice--success]="r.effectEnded" [class.mr-notice--neutral]="!r.effectEnded">
                  <mat-icon aria-hidden="true">{{ r.effectEnded ? 'check_circle' : 'info' }}</mat-icon>
                  <p>{{ r.textPt }}</p>
                </div>
              }
            </div>
          }
        }
        @default {
          <p class="what" role="status">{{ goneText() }}</p>
        }
      }
      <div foot>
        @switch (stage()) {
          @case ('ask') {
            @if (view().autoFail) {
              <button mat-flat-button type="button" class="btn" data-initial-focus [disabled]="busy()" (click)="answer({ kind: 'app' })">
                Continuar
              </button>
            } @else if (offer() || luckyOffer()) {
              <button mat-stroked-button type="button" class="btn" data-initial-focus [disabled]="busy()" (click)="close()">Fechar</button>
            } @else if (!view().handed) {
              <div class="stack">
                @if (canApp) {
                  <button mat-flat-button type="button" class="btn" data-initial-focus [disabled]="busy()" (click)="answer({ kind: 'app' })">
                    <mat-icon aria-hidden="true">casino</mat-icon>Rolar no app
                  </button>
                }
                @if (canType && !typingNow()) {
                  <button mat-stroked-button type="button" class="btn" [attr.data-initial-focus]="canApp ? null : ''" [disabled]="busy()" (click)="typing.set(true)">
                    Digitar o resultado
                  </button>
                }
                <button mat-button type="button" class="btn btn--link" [disabled]="busy()" (click)="answer({ kind: 'hand' })">
                  Deixar o mestre rolar por mim
                </button>
              </div>
            } @else {
              <button mat-stroked-button type="button" class="btn" data-initial-focus (click)="close()">Fechar</button>
            }
          }
          @default {
            <button mat-stroked-button type="button" class="btn" data-initial-focus (click)="close()">Fechar</button>
          }
        }
      </div>
    </app-sheet-frame>
  `,
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
  /** The d20 was rolled and kept: the character holds a Bardic Inspiration die, and the player decides before the result. */
  protected readonly offer = signal<OutsideInspirationOffer | null>(null);
  private readonly dieKey = new ActionKey();
  /** The d20 was rolled and kept because it shows a 1 and the character is a halfling: the Lucky question ("Tirou 1:
   * rolar de novo?") comes first, before the die's. */
  protected readonly luckyOffer = signal<LuckyOffer | null>(null);
  /** The held d20, above the Lucky question. */
  protected readonly luckyHeld = computed(() => luckyHeldLine(this.luckyOffer()));
  /** One key per Lucky answer (roll again this way, or keep the 1): the same answer again is a retry. */
  private readonly luckyKey = new ActionKey();
  /** "Sortudo: 1 → 14" under the result: the d20 Lucky rolled again (the faces and the total are the new ones). */
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
  protected readonly view = computed(() => effectSaveView(this.prompt(), this.data.round));
  /** The fields of the d4 the prompt lists (or the ones a refusal asked for). */
  protected readonly fields = computed<readonly ExtraDieField[]>(() => {
    const listed = dieFields(
      this.prompt().extraDice.map((d) => ({
        name: d.sourceNamePt,
        faces: d.faces,
        sign: d.sign,
      })),
    );
    return this.asked() > listed.length
      ? [...listed, ...genericDieFields(this.asked() - listed.length, EFFECT_DIE_FACES)]
      : listed;
  });
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

  protected async answer(how: EffectSaveAnswer): Promise<void> {
    if (this.busy()) {
      return;
    }
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
      if (res.lucky || res.offer) {
        // Nothing was written: the d20 alone, and the result after the answer (Lucky's question first).
        this.luckyOffer.set(res.lucky ?? null);
        this.offer.set(res.offer ?? null);
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
      const settled = res.result.case === 'effectSave' ? res.result.value : null;
      if (!settled?.encounter || (!settled.result && !settled.inspirationOffer)) {
        throw new Error('AnswerOutsideLucky answered without its saving throw');
      }
      this.data.state.apply(settled.encounter);
      this.luckyKey.renew();
      this.luckyOffer.set(null);
      if (settled.inspirationOffer) {
        this.offer.set(settled.inspirationOffer);
        this.animateLucky(settled.inspirationOffer.d20, die);
        return;
      }
      this.keys.renew();
      this.result.set(settled.result!);
      this.animate(settled.result!);
      this.done.set(true);
    } catch (err) {
      this.error.set(classResourceErrorMessage(err, 'responder ao Sortudo'));
    } finally {
      this.busy.set(false);
    }
  }

  /** The d20 the app rolled again for Lucky, tumbling, when the die's question comes next (a told result animates its
   * own d20): the new face alone, "1 → 14". */
  private animateLucky(roll: DiceRoll | undefined, die: LuckyRoll | null): void {
    const again = die && 'inApp' in die ? luckyReroll(roll) : undefined;
    const show = again
      ? showOfDie('Sortudo', roll?.diceSides ?? 0, again.to, `${again.from} → ${again.to}`)
      : null;
    if (show) {
      this.animator.play(show);
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
      if (
        res.result.case !== 'effectSave' ||
        !res.result.value.encounter ||
        !res.result.value.result
      ) {
        throw new Error('AnswerOutsideInspiration answered without its saving throw');
      }
      this.data.state.apply(res.result.value.encounter);
      this.offer.set(null);
      this.keys.renew();
      this.result.set(res.result.value.result);
      this.animate(res.result.value.result);
      this.done.set(true);
    } catch (err) {
      this.error.set(classResourceErrorMessage(err, 'responder sobre a Inspiração de Bardo'));
    } finally {
      this.busy.set(false);
    }
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
