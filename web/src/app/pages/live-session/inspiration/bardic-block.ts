import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  input,
  signal,
  untracked,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { DiceMode } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type { HeldRollOffer } from '../../../../gen/meurpg/play/v1/combat_pb';
import type {
  AnswerHeldRollResponse,
  GetOutsideInspirationResponse,
} from '../../../../gen/meurpg/play/v1/resources_pb';
import { luckyText } from '../../../core/combat/lucky';
import { ActionKey } from '../../../core/connect/idempotency';
import { HeldRollController } from '../../../core/held-rolls/held-roll-controller';
import {
  type HeldAnswer,
  asksLucky,
  asksStroke,
  heldOfferOf,
} from '../../../core/held-rolls/held-roll-offer';
import { HeldVanishWatch } from '../../../core/held-rolls/held-vanish';
import { settledRoll } from '../../../core/held-rolls/held-roll-result';
import { ResourceClient } from '../../../core/resources/resources-client';
import { classResourceErrorMessage } from '../../../core/resources/resources-errors';
import { HeldRollPrompt } from '../../../shared/held-roll-prompt/held-roll-prompt';

const SECONDS_PER_MINUTE = 60;

/** "5 min", "1 min": the game time a die has left, rounded up to the minute. */
export function minutesText(seconds: number): string {
  const min = Math.max(1, Math.ceil(seconds / SECONDS_PER_MINUTE));
  return `${min} min`;
}

/** What the block says once a Lucky answer got through: the d20 Lucky rolled again ("Sortudo: 1 → 14") or the 1 kept; then
 * the Bardic Inspiration question when the roll waits for it now (the block shows it on the new roll), the scene check's
 * total, or that the master sees the result. */
export function luckyAnswerText(res: AnswerHeldRollResponse, reroll: boolean): string {
  const again = reroll ? luckyText(settledRoll(res)) || 'Rolado de novo' : 'Você ficou com o 1';
  if (res.next) {
    return `${again}. Agora, a pergunta da Inspiração de Bardo.`;
  }
  const total = res.result.case === 'sceneCheck' ? res.result.value.roll?.roll?.total : undefined;
  return total === undefined
    ? `${again}. O mestre vê o resultado.`
    : `${again}. Seu total: ${total}.`;
}

/** What the rogue reads once Stroke of Luck is answered: that it was used (with the total of a check) or kept. */
export function strokeAnswerText(res: AnswerHeldRollResponse, use: boolean): string {
  const said = use ? 'Você usou o Golpe de Sorte' : 'Você manteve o resultado';
  const total = res.result.case === 'sceneCheck' ? res.result.value.roll?.roll?.total : undefined;
  return total === undefined
    ? `${said}. O mestre vê o resultado.`
    : `${said}. Seu total: ${total}.`;
}

/**
 * "Inspiração de Bardo" on the session panel, outside a combat (SRD 5.1, Bard): the bard sees the uses left and gives the
 * die to a character of the party ("Dar a Tavo"); the character that holds a die reads "Você tem uma Inspiração de Bardo
 * (d8) de Orla" with the game time it has left, and answers the rolls that wait for the die. The master reads the dice
 * held. In a combat the bard gives from the combat's own sheet. It reads again when `tick` moves (the table's hint that
 * the game time or an effect moved) and after each of its own calls. A roll of the caller that waits for the halfling's
 * Lucky question ("Tirou 1: rolar de novo?") is asked here too, in its own "Sortudo" section, so a reload asks again; a
 * halfling with no die reads no "Inspiração de Bardo" heading over it.
 */
@Component({
  selector: 'app-bardic-block',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [HeldRollPrompt, MatButtonModule, MatIconModule],
  template: `
    @if (luckyShown()) {
      <section class="block" aria-labelledby="lucky-block-t">
        <h2 id="lucky-block-t" class="t"><mat-icon aria-hidden="true">casino</mat-icon>{{ luckyHeading() }}</h2>
        @if (lucky(); as o) {
          @if (held.error()) {
            <div class="mr-notice mr-notice--danger" role="alert">
              <mat-icon aria-hidden="true">error</mat-icon>
              <p>{{ held.error() }}</p>
            </div>
          }
          <app-held-roll-prompt
            [offer]="o"
            look="card"
            [canApp]="canApp()"
            [canType]="canType()"
            [preferApp]="canApp()"
            [busy]="busy()"
            (answer)="answer($event)"
          />
        }
        @if (luckyAnswered(); as text) {
          <p class="mine" role="status">{{ text }}</p>
        }
      </section>
    }
    @if (vanish.notice(); as notice) {
      <p class="mine" role="status" data-testid="held-vanished">{{ notice }}</p>
    }
    @if (shown()) {
      <section class="block" aria-labelledby="bardic-t">
        <h2 id="bardic-t" class="t"><mat-icon aria-hidden="true">music_note</mat-icon>Inspiração de Bardo</h2>
        @if (error() || dieError()) {
          <div class="mr-notice mr-notice--danger" role="alert">
            <mat-icon aria-hidden="true">error</mat-icon>
            <p>{{ error() || dieError() }}</p>
          </div>
        }
        @if (state(); as s) {
          @if (s.mine; as die) {
            <p class="mine" role="status">
              Você tem uma Inspiração de Bardo (d{{ die.sides }}){{ die.fromName ? ' de ' + die.fromName : '' }}, por mais
              {{ minutes(die.secondsLeft) }} de jogo. Você pode somá-la a uma prova de habilidade, a uma jogada de ataque ou a um teste de resistência que você mesmo rola; ao rolar, o app pergunta. Os testes que o app rola por você, como o de uma magia que mira o seu personagem, não perguntam.
            </p>
          }
          @if (die(); as o) {
            <app-held-roll-prompt
              [offer]="o"
              look="card"
              [canApp]="canApp()"
              [canType]="canType()"
              [busy]="busy()"
              (answer)="answer($event)"
            />
          }
          @if (answered(); as text) {
            <p class="mine" role="status">{{ text }}</p>
          }
          @if (s.isBard && !s.combatOpen) {
            <p class="uses">
              Usos: <b>{{ s.usesLeft }}</b> de {{ s.usesMax }} · dado d{{ s.sides }} · ação bônus
            </p>
            <ul class="targets">
              @for (t of s.targets; track t.characterId) {
                <li>
                  <button
                    matButton="outlined"
                    type="button"
                    [disabled]="busy() || s.usesLeft === 0 || t.disabledReasonPt !== ''"
                    disabledInteractive
                    (click)="give(t.characterId, t.name)"
                  >
                    Dar a {{ t.name }}
                  </button>
                  @if (t.disabledReasonPt) {
                    <span class="small">{{ t.disabledReasonPt }}</span>
                  }
                </li>
              } @empty {
                <li class="small">Não há ninguém para inspirar.</li>
              }
            </ul>
          }
          @if (s.held.length > 0) {
            <ul class="held">
              @for (d of s.held; track d.characterId) {
                <li>{{ d.characterName }}: d{{ d.sides }}{{ d.fromName ? ' de ' + d.fromName : '' }}, {{ minutes(d.secondsLeft) }} de jogo</li>
              }
            </ul>
          }
        }
      </section>
    }
  `,
  styles: `
    .block { display: grid; gap: 12px; padding: 16px; border: 1px solid var(--mr-border, #ccc); border-radius: 12px; margin-block: 12px; }
    .t { display: flex; align-items: center; gap: 8px; margin: 0; font-size: 1.05rem; }
    .mine, .uses { margin: 0; }
    .targets, .held { display: grid; gap: 8px; margin: 0; padding: 0; list-style: none; }
    .targets li { display: grid; gap: 4px; }
    .small { font-size: 0.875rem; color: var(--mr-text-muted, inherit); }
  `,
})
export class BardicBlock {
  private readonly api = inject(ResourceClient);

  readonly campaignId = input.required<string>();
  readonly diceMode = input<DiceMode>(DiceMode.PLAYERS_CHOOSE);
  /** Moves when the table's game time or effects move: read the state again. */
  readonly tick = input(0);
  /** The player's block also asks the rogue's Stroke of Luck question of a roll outside a combat, which only the list of
   * held rolls carries (the master's block does not: the master reads the questions on the held-rolls panel). */
  readonly strokeQuestions = input(false);

  protected readonly state = signal<GetOutsideInspirationResponse | null>(null);
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  protected readonly answered = signal('');
  /** What the last Lucky answer came to. */
  protected readonly luckyAnswered = signal('');
  /** The question that waits for the character, Lucky's or the die's: a character has one at a time. */
  protected readonly held = new HeldRollController({
    send: (holdId, answer, key) => this.api.answerHeldRoll(this.campaignId(), holdId, answer, key),
    settle: (res, done) => this.say(res, done.answer),
    busy: this.busy,
  });
  /** Tells the player their question was answered by the master when it is gone and they did not answer it here. */
  protected readonly vanish = new HeldVanishWatch();
  protected readonly lucky = computed(() => {
    const asked = this.held.offer();
    return asksLucky(asked) || asksStroke(asked) ? asked : null;
  });
  protected readonly die = computed(() => {
    const asked = this.held.offer();
    return asked && !asksLucky(asked) && !asksStroke(asked) ? asked : null;
  });
  /** The heading of the section: Lucky's, or the rogue's Stroke of Luck. */
  protected readonly luckyHeading = computed(() =>
    asksStroke(this.held.offer()) ? 'Golpe de Sorte' : 'Sortudo',
  );
  /** The die section's error: the give's, or the answer's when the question is the die's. */
  protected readonly dieError = computed(() => (this.die() ? this.held.error() : ''));
  protected readonly minutes = minutesText;
  protected readonly canApp = computed(() => this.diceMode() !== DiceMode.PHYSICAL);
  protected readonly canType = computed(() => this.diceMode() !== DiceMode.APP);
  protected readonly shown = computed(() => {
    const s = this.state();
    return (
      !!s &&
      (!!s.mine ||
        this.die() !== null ||
        (s.isBard && !s.combatOpen) ||
        s.held.length > 0 ||
        this.answered() !== '')
    );
  });
  private readonly keys = new ActionKey();
  /** The Lucky section: a roll waits for its question, or the last answer is still said. */
  protected readonly luckyShown = computed(
    () => this.lucky() !== null || this.luckyAnswered() !== '',
  );

  constructor() {
    effect(() => {
      this.tick();
      this.campaignId();
      untracked(() => void this.load());
    });
  }

  /** The first Stroke of Luck question that waits for the caller, from the list of held rolls; `undefined` for none. */
  private async strokeOffer(): Promise<HeldRollOffer | undefined> {
    if (!this.strokeQuestions()) {
      return undefined;
    }
    try {
      const res = await this.api.listHeldRolls(this.campaignId());
      return res.offers.find((o) => asksStroke(o));
    } catch {
      return undefined;
    }
  }

  private async load(): Promise<void> {
    try {
      const state = await this.api.outsideInspiration(this.campaignId());
      this.state.set(state);
      if (!this.held.busy()) {
        // The block lists what the server lists as older fields: the question of the first roll that waits, Lucky's first.
        const offer =
          heldOfferOf({ luckyOffer: state.luckyOffers[0] }) ??
          heldOfferOf({ inspirationOffer: state.offers[0] }) ??
          (await this.strokeOffer());
        this.held.ask(offer);
        this.vanish.observe(offer);
      }
    } catch {
      // A hint that did not come back: the block keeps what it shows and reads again on the next one.
    }
  }

  protected async give(characterId: string, name: string): Promise<void> {
    if (this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      await this.api.giveBardicInspirationOutside(
        this.campaignId(),
        characterId,
        this.keys.keyFor({ give: characterId }),
      );
      this.keys.renew();
      this.answered.set(`Você inspirou ${name}.`);
      await this.load();
    } catch (err) {
      this.error.set(classResourceErrorMessage(err, 'dar a Inspiração de Bardo'));
    } finally {
      this.busy.set(false);
    }
  }

  /** The answer to the question: "Rolar de novo" or "Ficar com o 1" (the roll is written by its own path with the new d20,
   * or waits for the Bardic Inspiration question on the new roll, which the block then shows), or the die used or kept. The
   * state is read again either way. */
  protected async answer(answer: HeldAnswer): Promise<void> {
    await this.held.answer(answer);
    await this.load();
  }

  /** What the block says once an answer got through. */
  private say(res: AnswerHeldRollResponse, answer: HeldAnswer): void {
    if (answer.kind === 'stroke') {
      this.luckyAnswered.set(strokeAnswerText(res, answer.use));
      return;
    }
    if (answer.kind !== 'die') {
      this.luckyAnswered.set(luckyAnswerText(res, answer.reroll));
      return;
    }
    const total = res.result.case === 'sceneCheck' ? res.result.value.roll?.roll?.total : undefined;
    const word = answer.use ? 'Dado somado' : 'Dado guardado';
    this.answered.set(
      total === undefined ? `${word}. O mestre vê o resultado.` : `${word}. Seu total: ${total}.`,
    );
  }
}
