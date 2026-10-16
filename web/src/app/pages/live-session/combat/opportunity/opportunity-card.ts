import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  effect,
  inject,
  input,
  output,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import {
  type Encounter,
  JumpKind,
  type OpportunityOffer,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { metersFixed } from '../../../../core/units';
import {
  type ReactorAttack,
  attackLabel,
  masterAsk,
  masterNews,
  offersToAnswer,
  reactorIsMasters,
} from '../../../../core/combat/opportunity';
import { autoPassText } from '../../../../core/combat/reaction-autopass';
import { tieNumbers, upFirst } from '../../../../core/format/text';
import { article } from '../../../../core/combat/combat-log';
import { ofThe } from '../../../../core/combat/move-plan';
import type { CombatantInfo } from '../combat-info';

/** The master's answer to an NPC's offer: "Atacar com X", or "Não atacar". */
export interface MasterAnswer {
  readonly offer: OpportunityOffer;
  readonly attack: ReactorAttack | null;
}

/**
 * The master's opportunity prompts, on the combat screen (E9-13). Two kinds:
 *
 * - **An NPC's reactor** (the master answers): "O Toren saiu do alcance do Goblin
 *   2. Goblin 2 ataca o Toren?", the attack with its numbers and "gasta a
 *   reação dele", and "Não atacar" (the focus lands there: the safe answer) with
 *   "Atacar com <arma>", the same width. The turn of the mover waits.
 * - **A player's reactor**: "Esperando a reação do Caio (Toren)", with "Seguir
 *   sem esperar", which takes that reaction away from the player when they do not
 *   answer (offline). The prompt stays on the player's screen until they answer
 *   or the master skips it.
 *
 * It lists what the server sent the master (`Encounter.opportunity_offers`):
 * the browser works out neither who provokes whom nor who may answer.
 */
@Component({
  selector: 'app-opportunity-card',
  imports: [MatButtonModule, MatIconModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @for (o of offers(); track o.id) {
      <section class="op" role="group" [attr.aria-label]="'Ataque de oportunidade de ' + o.reactorLabel">
        <h2 class="op__title"><mat-icon aria-hidden="true">swords</mat-icon>Ataque de oportunidade</h2>
        @if (mine(o) || !o.byHand) {
          <p class="op__news">{{ mine(o) ? news(o) : leaving(o) }}</p>
        }
        @if (mine(o)) {
          <p class="op__ask"><b>{{ ask(o) }}</b></p>
          @if (attacks(o)[0]; as first) {
            <p class="op__sub">{{ first.detail }} · gasta a reação dele</p>
          }
          @if (!attacks(o).length) {
            <p class="op__sub" role="status">
              {{ failed().has(o.id) ? 'Não deu para ler os ataques dele.' : 'Lendo os ataques dele…' }}
            </p>
          }
          <div class="op__pair">
            <button mat-stroked-button type="button" class="op__btn" data-safe [disabled]="busy()" (click)="answer.emit({ offer: o, attack: null })">
              Não atacar
            </button>
            @for (a of attacks(o); track a.key) {
              <button mat-flat-button type="button" class="op__btn" [disabled]="busy() || !a.attack" disabledInteractive (click)="answer.emit({ offer: o, attack: a })">
                {{ label(a) }}
              </button>
            }
            @if (!attacks(o).length && failed().has(o.id)) {
              <button mat-stroked-button type="button" class="op__btn" (click)="retry.emit(o)">Tentar de novo</button>
            }
          </div>
          <p class="op__note">
            <mat-icon aria-hidden="true">info</mat-icon>
            O turno {{ ofMover(o) }} espera a sua resposta. {{ walked(o) }}
          </p>
          @if (o.byHand) {
            <!-- A combat without a map: nobody has a square, so nothing goes back anywhere. -->
            <div class="op__pair">
              <button mat-stroked-button type="button" class="op__btn" [disabled]="busy()" (click)="withdraw.emit(o)">Retirar a oferta</button>
            </div>
          } @else {
            <p class="op__note op__note--plain">
              O ataque conta como feito logo antes de {{ theMover(o) }} sair do alcance: se levar a 0 PV, o token volta ao último
              quadrado que ainda estava no alcance.
            </p>
          }
        } @else if (o.byHand) {
          <!-- The master offered it by hand: the wait is a status, and he can send it on or take it back. -->
          <div class="op__wait" role="status">
            <mat-icon aria-hidden="true">schedule</mat-icon>
            <p><b>Esperando a resposta {{ waitingFor(o) }}.</b> {{ leavingBy(o) }} O turno continua depois da resposta.</p>
          </div>
          <div class="op__pair">
            <button mat-stroked-button type="button" class="op__btn" [disabled]="busy()" (click)="skip.emit(o)">Seguir sem esperar</button>
            <button mat-stroked-button type="button" class="op__btn" [disabled]="busy()" (click)="withdraw.emit(o)">Retirar a oferta</button>
          </div>
          <p class="op__note op__note--plain">
            “Seguir sem esperar” é para quando o jogador não responde: o turno continua. “Retirar a oferta” é para quando você ofereceu sem querer.
            Nos dois, {{ theReactor(o) }} não ataca e continua com a reação.
          </p>
        } @else {
          <p class="op__ask"><b>Esperando a reação {{ waitingFor(o) }}</b>@if (autoText(o); as t) { · {{ t }}}</p>
          <div class="op__pair">
            <button mat-stroked-button type="button" class="op__btn" [disabled]="busy()" (click)="skip.emit(o)">
              Seguir sem esperar
            </button>
            @if (canUndo()) {
              <button mat-stroked-button type="button" class="op__btn" [disabled]="busy()" (click)="undo.emit()">
                Desfazer o movimento
              </button>
            }
          </div>
          <p class="op__note op__note--plain">
            Se você seguir sem esperar, {{ theReactor(o) }} não ataca e continua com a reação. O aviso fica na tela do jogador até a
            resposta, ou até você seguir sem esperar.
          </p>
        }
      </section>
    }
  `,
  styleUrl: './opportunity-card.scss',
})
export class OpportunityCard {
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);
  readonly encounter = input.required<Encounter>();
  /** Each NPC reactor's attacks with their numbers, by offer id. */
  readonly attacksByOffer = input<ReadonlyMap<string, readonly ReactorAttack[]>>(new Map());
  readonly info = input<ReadonlyMap<string, CombatantInfo>>(new Map());
  readonly busy = input(false);
  /** The seconds left before the master's screen passes each offer by itself, by offer id. */
  readonly autoLeft = input<Readonly<Record<string, number>>>({});
  /** The offers whose reactor's attacks could not be read (the card offers to try again). */
  readonly failed = input<ReadonlySet<string>>(new Set());
  /** The master's last action is the move that made the offers: "Desfazer o movimento" is possible. */
  readonly canUndo = input(false);

  readonly answer = output<MasterAnswer>();
  readonly skip = output<OpportunityOffer>();
  /** "Retirar a oferta": only the master's own offers (a combat without a map). */
  readonly withdraw = output<OpportunityOffer>();
  readonly retry = output<OpportunityOffer>();
  readonly undo = output<void>();

  protected readonly offers = computed(() => offersToAnswer(this.encounter()));

  constructor() {
    // A prompt that appears puts the focus on the safe answer ("Não atacar").
    let seen = '';
    effect(() => {
      const ids = this.offers()
        .map((o) => o.id)
        .join(',');
      if (ids !== '' && ids !== seen) {
        afterNextRender(
          () =>
            this.host.nativeElement
              .querySelector<HTMLButtonElement>('[data-safe]')
              ?.focus({ focusVisible: true } as FocusOptions),
          {
            injector: this.injector,
          },
        );
      }
      seen = ids;
    });
  }

  protected mine(o: OpportunityOffer): boolean {
    return reactorIsMasters(this.encounter(), o);
  }

  protected news(o: OpportunityOffer): string {
    return tieNumbers(masterNews(o));
  }

  /** "O Goblin 2 sai do alcance do Toren: espera o Caio". */
  protected leaving(o: OpportunityOffer): string {
    const reactor = this.encounter().combatants.find((c) => c.id === o.reactorId);
    const name = reactor ? (this.info().get(reactor.characterId)?.playerName ?? '') : '';
    const who = name ? `${article(name)} ${name}` : 'o jogador';
    return tieNumbers(
      `${upFirst(`${article(o.moverLabel)} ${o.moverLabel}`)} sai do alcance ${ofThe([o.reactorLabel])}: espera ${who}.`,
    );
  }

  protected ask(o: OpportunityOffer): string {
    return tieNumbers(masterAsk(o));
  }

  protected attacks(o: OpportunityOffer): readonly ReactorAttack[] {
    return this.attacksByOffer().get(o.id) ?? [];
  }

  protected label(a: ReactorAttack): string {
    return attackLabel(a);
  }

  protected ofMover(o: OpportunityOffer): string {
    return ofThe([o.moverLabel]);
  }

  protected theMover(o: OpportunityOffer): string {
    return `${article(o.moverLabel)} ${o.moverLabel}`;
  }

  protected theReactor(o: OpportunityOffer): string {
    return `${article(o.reactorLabel)} ${o.reactorLabel}`;
  }

  /** "O Goblin 1 saiu do alcance dele.": the master's offer, with the name of the one whose reach it was. */
  protected leavingBy(o: OpportunityOffer): string {
    const of = article(o.reactorLabel) === 'a' ? 'dela' : 'dele';
    const left = o.jump === JumpKind.LONG ? 'saltou para fora do' : 'saiu do';
    return tieNumbers(
      `${upFirst(`${article(o.moverLabel)} ${o.moverLabel}`)} ${left} alcance ${of}.`,
    );
  }

  /** "Ele já andou 4,5 m: o movimento valeu." */
  protected walked(o: OpportunityOffer): string {
    const mover = this.encounter().combatants.find((c) => c.id === o.moverId);
    const used = mover?.movementUsedDft ?? 0;
    // A jump is said as one: "Já saltou 4,5 m: o salto valeu."
    if (o.jump === JumpKind.LONG) {
      return used > 0 ? `Já saltou ${metersFixed(used / 10)}: o salto valeu.` : 'O salto valeu.';
    }
    return used > 0
      ? `Já andou ${metersFixed(used / 10)}: o movimento valeu.`
      : 'O movimento valeu.';
  }

  /** "do Caio (Toren)": the player's name when the roster has it. */
  /** "Passa sozinho em 24 s", for the offer a player's character answers; empty when this screen has no clock for it. */
  protected autoText(o: OpportunityOffer): string {
    const left = this.autoLeft()[o.id];
    return left === undefined ? '' : autoPassText(left);
  }

  protected waitingFor(o: OpportunityOffer): string {
    const reactor = this.encounter().combatants.find((c) => c.id === o.reactorId);
    const name = reactor ? (this.info().get(reactor.characterId)?.playerName ?? '') : '';
    return tieNumbers(
      name ? `${ofThe([name])} (${o.reactorLabel})` : `do jogador de ${o.reactorLabel}`,
    );
  }
}
