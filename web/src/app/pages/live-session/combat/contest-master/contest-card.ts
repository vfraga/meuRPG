import {
  ChangeDetectionStrategy,
  Component,
  computed,
  inject,
  input,
  output,
  signal,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type { Encounter } from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  type ContestView,
  ContestPurpose,
  ContestSkill,
  ContestStatus,
  RollModeKind,
  ShoveOutcome,
} from '../../../../../gen/meurpg/play/v1/contest_types_pb';
import { RollAnimator, showOfCheck } from '../../../../shared/roll-overlay/roll-animator';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { type CheckDie, ContestClient } from '../../../../core/combat/contest-client';
import {
  answerWait,
  byWho,
  masterContestTitle,
  masterPushLine,
  optionLabel,
  rollHead,
  suggestedSkill,
  totalLine,
  waitingForPlayer,
  winnerLine,
} from '../../../../core/combat/contest-master';
import type { ContestState } from '../../../../core/combat/contest-state';
import {
  named,
  ofNamed,
  proneLine,
  proneWord,
  skillLine,
  skillName,
  whoIs,
  checkNaturalMark,
} from '../../../../core/combat/contest-view';
import { ActionKey } from '../../../../core/connect/idempotency';
import { CheckRollForm } from '../check-roll-form/check-roll-form';
import { upFirst } from '../../../../core/format/text';

let nextId = 0;

/**
 * A contest that waits for the master (W7-X, boards W7-Xa 3 and W7-Xb 4 to 7), the entry of the CONTEST reaction window in his
 * queue. Two stages. The answer: who tries what, the initiator's total (his to read, RN-20), the skill of the defender (the
 * app suggests the one with the higher modifier; the master picks) and the roll, in the app or typed, for an NPC defender, for a
 * player who left the roll to him ("Deixar o mestre rolar por mim") or for one who has not answered ("Rolar por Brisa"); and
 * "Encerrar disputa", which ends it with no result. The shove's choice, when the winner is an NPC: "Derrubar" or "Empurrar 1,5 m"
 * (a blocked square says why and the push becomes "não sai do lugar"). It answers with `RespondContest` and `ResolveShove`,
 * never with `AnswerReaction`, and every call carries its own key, made once.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-contest-card',
  imports: [CheckRollForm, MatButtonModule, MatIconModule],
  styleUrl: './master-card.scss',
  template: `
    <section class="card" [attr.aria-labelledby]="uid + 't'" data-testid="contest-card">
      <h2 class="card__title" [id]="uid + 't'">{{ title() }}</h2>
      <p class="card__sub">Disputa</p>

      @if (initiatorLine(); as line) {
        <div class="total">
          <span class="total__head">{{ line.head }}</span>
          <span class="total__sum">{{ line.sum }}</span>
          @if (line.natural) {
            <span class="mr-tag" data-testid="natural-mark">{{ line.natural }}</span>
          }
        </div>
      }

      @if (stage() === 'shove') {
        <p class="card__text" role="status">
          {{ shoveLead() }}
        </p>
        <fieldset class="choices">
          <legend class="choices__cap">O que fazer com {{ targetName() }}</legend>
          <label class="choice" [class.choice--on]="outcome() === Prone">
            <input
              type="radio"
              class="mr-visually-hidden"
              [name]="uid + 'o'"
              [checked]="outcome() === Prone"
              (change)="outcome.set(Prone)"
            />
            <span class="choice__text">
              <span class="choice__name">Derrubar</span>
              <span class="choice__sub">{{ proneText() }}</span>
            </span>
          </label>
          <label class="choice" [class.choice--on]="outcome() === Push">
            <input
              type="radio"
              class="mr-visually-hidden"
              [name]="uid + 'o'"
              [checked]="outcome() === Push"
              (change)="outcome.set(Push)"
            />
            <span class="choice__text">
              <span class="choice__name">Empurrar 1,5 m</span>
              <span class="choice__sub">{{ pushText() }}</span>
            </span>
          </label>
        </fieldset>
      } @else {
        @if (!formShown()) {
          <div class="mr-notice mr-notice--neutral" role="status">
            <mat-icon aria-hidden="true">schedule</mat-icon>
            <p>{{ waitText() }}</p>
          </div>
          <div class="btns">
            <button
              mat-flat-button
              type="button"
              class="btn"
              data-initial
              [disabled]="busy()"
              (click)="forPlayer.set(true)"
            >
              Rolar por {{ defenderLabel() }}
            </button>
          </div>
        } @else {
          @if (c().deferred) {
            <p class="card__text">{{ waitText() }}</p>
          } @else {
            <fieldset class="choices">
              <legend class="choices__cap">
                {{ choiceCap() }}
              </legend>
              @for (o of options(); track o.skill) {
                <label class="choice" [class.choice--on]="chosenSkill() === o.skill">
                  <input
                    type="radio"
                    class="mr-visually-hidden"
                    [name]="uid + 's'"
                    [checked]="chosenSkill() === o.skill"
                    (change)="pick(o.skill)"
                  />
                  <span class="choice__text">
                    <span class="choice__name">{{ label(o) }}</span>
                    @if (o.suggested) {
                      <span class="choice__sub">Sugerida</span>
                    }
                  </span>
                </label>
              }
            </fieldset>
          }
          <app-check-roll-form
            [mode]="mode()"
            [modifier]="modifier()"
            [checkName]="checkName()"
            [notes]="notes()"
            [totalNote]="'Total ' + ofDefender()"
            [appLabel]="rollLabel()"
            [diceMode]="diceMode()"
            [preference]="preference()"
            [busy]="busy()"
            (roll)="answer($event)"
          />
        }
      }

      @if (error()) {
        <div class="mr-notice mr-notice--danger" role="alert">
          <mat-icon aria-hidden="true">error</mat-icon>
          <p>{{ error() }}</p>
        </div>
      }

      <div class="btns">
        @if (stage() === 'shove') {
          <button
            mat-flat-button
            type="button"
            class="btn"
            data-initial
            [disabled]="busy()"
            (click)="confirmShove()"
          >
            Confirmar
          </button>
        }
        <button mat-stroked-button type="button" class="btn" [disabled]="busy()" (click)="close()">
          Encerrar disputa
        </button>
      </div>
      <p class="small">Encerrar não devolve a ação que a disputa gastou.</p>
    </section>
  `,
})
export class ContestCard {
  private readonly api = inject(ContestClient);
  private readonly animator = inject(RollAnimator);
  private readonly keys = new ActionKey();

  readonly contest = input.required<ContestView>();
  readonly encounter = input.required<Encounter | null>();
  readonly campaignId = input.required<string>();
  readonly state = input.required<CombatState>();
  readonly contests = input.required<ContestState>();
  readonly diceMode = input.required<DiceMode>();
  readonly preference = input.required<DicePreference>();

  /** What the last call did, for the page's live region. */
  readonly said = output<string>();

  protected readonly uid = `cc-${nextId++}-`;
  protected readonly Prone = ShoveOutcome.PRONE;
  protected readonly Push = ShoveOutcome.PUSH;
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  /** "Rolar por Brisa": the master answers for a player who has not (the form opens). */
  protected readonly forPlayer = signal(false);
  private readonly picked = signal<ContestSkill | null>(null);
  protected readonly outcome = signal<ShoveOutcome>(ShoveOutcome.PRONE);

  protected readonly c = computed(() => this.contest());
  protected readonly title = computed(() => masterContestTitle(this.encounter(), this.c()));
  protected readonly stage = computed<'answer' | 'shove'>(() =>
    this.c().status === ContestStatus.AWAITING_OUTCOME ? 'shove' : 'answer',
  );
  protected readonly initiatorLabel = computed(
    () => this.encounter()?.combatants.find((x) => x.id === this.c().initiatorId)?.label ?? '',
  );
  protected readonly defender = computed(() =>
    this.encounter()?.combatants.find((x) => x.id === this.c().defenderId),
  );
  protected readonly defenderLabel = computed(() => this.defender()?.label ?? 'o jogador');
  /** "do Hobgoblin", "de Brisa". */
  protected readonly ofDefender = computed(() => ofNamed(whoIs(this.defender())));
  /** "pelo Hobgoblin", "por Brisa". */
  protected readonly byDefender = computed(() => byWho(whoIs(this.defender())));
  /** "Rolar pelo Hobgoblin", or "Rolar 2d20 pelo Hobgoblin" when the roll has advantage or disadvantage. */
  protected readonly rollLabel = computed(
    () =>
      `Rolar ${
        this.mode() === RollModeKind.NORMAL || this.mode() === RollModeKind.UNSPECIFIED
          ? ''
          : '2d20 '
      }${this.byDefender()}`,
  );
  protected readonly initiatorLine = computed(() => {
    const roll = this.c().initiatorRoll;
    return roll
      ? {
          head: rollHead(this.initiatorLabel(), roll.skill),
          sum: totalLine(roll),
          natural: checkNaturalMark(roll),
        }
      : null;
  });
  protected readonly options = computed(() => this.c().answerOptions);
  protected readonly chosenSkill = computed(() => this.picked() ?? suggestedSkill(this.options()));
  protected readonly chosen = computed(
    () => this.options().find((o) => o.skill === this.chosenSkill()) ?? this.options()[0],
  );
  protected readonly mode = computed(() => this.chosen()?.mode ?? RollModeKind.NORMAL);
  protected readonly modifier = computed(() => this.chosen()?.modifier ?? 0);
  protected readonly notes = computed(() => this.chosen()?.notes ?? []);
  protected readonly checkName = computed(() => skillLine(this.chosenSkill()));
  protected readonly formShown = computed(() => !waitingForPlayer(this.c()) || this.forPlayer());
  protected readonly waitText = computed(() => answerWait(this.encounter(), this.c()));
  protected readonly choiceCap = computed(() =>
    this.c().purpose === ContestPurpose.ESCAPE
      ? 'O teste de quem agarra'
      : 'O alvo escolhe a habilidade (SRD). Sugerido: a de maior modificador.',
  );
  protected readonly targetName = computed(() => named(whoIs(this.defender())));
  protected readonly shoveLead = computed(() => {
    const winner = winnerLine(this.encounter(), this.c());
    return `${winner ? `${winner} a disputa. ` : ''}Escolha o que fazer com ${this.targetName()}.`;
  });
  protected readonly proneText = computed(() => proneLine(whoIs(this.defender())));
  protected readonly pushText = computed(() => {
    const choice = this.c().shoveChoice;
    return masterPushLine(
      ofNamed(whoIs(this.encounter()?.combatants.find((x) => x.id === this.c().initiatorId))),
      choice?.pushAvailable === false ? choice.pushBlocked : 0,
    );
  });

  protected readonly label = optionLabel;

  protected pick(skill: ContestSkill): void {
    this.picked.set(skill);
    this.error.set('');
  }

  /** The defender's roll, for whoever the master rolls: the skill that is chosen and the d20 (app or typed). */
  protected async answer(die: CheckDie): Promise<void> {
    if (this.busy()) {
      return;
    }
    const skill = this.chosenSkill();
    await this.run('responder à disputa', async () => {
      const res = await this.api.respond(
        {
          campaignId: this.campaignId(),
          encounterId: this.encounter()?.id ?? '',
          contestId: this.c().id,
          skill,
          die,
        },
        this.keys.keyFor({ contest: this.c().id, skill, die }),
      );
      this.state().apply(res.encounter);
      this.contests().applyContest(res.contest);
      const roll = res.contest.defenderRoll;
      // The master's roll for the defender, as the line under the card tells it.
      const show = showOfCheck(`Teste de ${skillName(skill)}`, roll, { withTotal: true });
      if (show) {
        this.animator.play(show);
      }
      const natural = checkNaturalMark(roll);
      const mine = roll
        ? `${rollHead(this.defenderLabel(), roll.skill)} ${totalLine(roll)}.${natural ? ` ${natural}.` : ''} `
        : '';
      const verdict = winnerLine(this.encounter(), res.contest);
      this.said.emit(`${mine}${upFirst(verdict)}.`.trim());
    });
  }

  /** "Confirmar": the shove's choice; a push into a blocked square is "não sai do lugar" (STAYS), which the server takes then. */
  protected async confirmShove(): Promise<void> {
    if (this.busy()) {
      return;
    }
    const blocked = this.c().shoveChoice?.pushAvailable === false;
    const outcome =
      this.outcome() === ShoveOutcome.PUSH && blocked ? ShoveOutcome.STAYS : this.outcome();
    await this.run('resolver o empurrão', async () => {
      const res = await this.api.resolveShove(
        this.campaignId(),
        this.encounter()?.id ?? '',
        this.c().id,
        outcome,
        this.keys.keyFor({ shove: this.c().id, outcome }),
      );
      this.state().apply(res.encounter);
      this.contests().applyContest(res.contest);
      this.said.emit(
        outcome === ShoveOutcome.PRONE
          ? `${upFirst(this.targetName())} ficou ${proneWord(this.defenderLabel())}.`
          : outcome === ShoveOutcome.PUSH
            ? `${upFirst(this.targetName())} foi empurrado 1,5 m.`
            : `${upFirst(this.targetName())} não saiu do lugar.`,
      );
    });
  }

  /** "Encerrar disputa": no result; the action it spent stays spent. */
  protected async close(): Promise<void> {
    if (this.busy()) {
      return;
    }
    await this.run('encerrar a disputa', async () => {
      const encounter = await this.api.closeContest(
        this.campaignId(),
        this.encounter()?.id ?? '',
        this.c().id,
        this.keys.keyFor({ close: this.c().id }),
      );
      this.state().apply(encounter);
      this.said.emit('Disputa encerrada.');
    });
  }

  private async run(what: string, call: () => Promise<void>): Promise<void> {
    this.busy.set(true);
    this.error.set('');
    try {
      await call();
    } catch (err) {
      this.error.set(combatErrorMessage(err, what));
    } finally {
      this.busy.set(false);
    }
  }
}
