import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  input,
  output,
  signal,
  untracked,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import {
  HitRiderChoice,
  HitRiderKind,
  type Combatant,
  type HitRiderOffer,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { Ability } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { abilityName } from '../../../../core/content/feat-text';
import { CreaturesClient } from '../../../../core/creatures/creatures-client';
import { CREATURE_SIZES } from '../../../../core/creatures/creature-types';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import { dcLabel } from '../../../../core/format/save-dc';

/** What the monk picks on an offer. */
export interface RiderPick {
  readonly id: string;
  readonly choice: HitRiderChoice;
}

/**
 * The riders after a hit. The monk's (SRD 5.1, Monk): the Open Hand technique on a Flurry of
 * Blows hit, and Stunning Strike (1 ki point) on a melee hit. Each offer is optional and
 * answered once. The target's save is rolled by the app; the DC is the ki save DC.
 * A creature's rider (kind CREATURE, the master's alone) is the attack's save or grapple from the stat block: "Pedir
 * teste de resistência de Brisa (CD 10, Constituição)" or "Agarrar (CD de fuga 12)", with "Aplicar" and "Dispensar". Under it, the SRD's text of the action as a reminder ("Texto do SRD": the creature-type
 * and size exceptions the plan does not hold are the master's call) and, when the stat block sets one, the size limit.
 */
@Component({
  selector: 'app-hit-rider-card',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatButtonModule, MatIconModule],
  template: `
    @if (error()) {
      <div class="mr-notice mr-notice--danger" role="alert" data-testid="rider-error">
        <mat-icon aria-hidden="true">error</mat-icon>
        <p>{{ error() }}</p>
      </div>
    }
    @for (o of offers(); track o.id) {
      @if (isCreature(o)) {
        <div class="mr-notice rider" role="group" [attr.aria-label]="creatureTitle(o)" data-testid="hit-rider">
          <mat-icon aria-hidden="true">pets</mat-icon>
          <div class="rider__body">
            <p class="rider__text">
              <strong>{{ creatureTitle(o) }}</strong>
              em {{ label(o.targetId) }}
            </p>
            <p class="rider__hint" [id]="hintId(o, 'creature')">{{ creatureLine(o) }}</p>
            @if (sizeLimit(o); as limit) {
              <p class="rider__hint" data-testid="rider-size">{{ limit }}</p>
            }
            @if (srdText(o); as text) {
              <div class="rider__srd" data-testid="rider-srd">
                <strong>Texto do SRD</strong>
                <p class="rider__hint" lang="en">{{ text }}</p>
              </div>
            }
            <div class="rider__btns rider__btns--row">
              <button
                matButton="outlined"
                type="button"
                [disabled]="busy()"
                [attr.aria-label]="'Aplicar ' + creatureTitle(o)"
                [attr.aria-describedby]="hintId(o, 'creature')"
                (click)="pick(o, choices.APPLY)"
              >
                Aplicar
              </button>
              <button matButton type="button" [disabled]="busy()" [attr.aria-label]="'Dispensar ' + creatureTitle(o)" (click)="pick(o, choices.DECLINE)">Dispensar</button>
            </div>
          </div>
        </div>
      } @else {
      <div class="mr-notice rider" role="group" [attr.aria-label]="title(o)" data-testid="hit-rider">
        <mat-icon aria-hidden="true">sports_martial_arts</mat-icon>
        <div class="rider__body">
          <p class="rider__text">
            <strong>{{ title(o) }}</strong>
            em {{ label(o.targetId) }}
          </p>
          <div class="rider__btns">
            @if (isOpenHand(o)) {
              <div class="rider__opt">
                <button
                  matButton="outlined"
                  type="button"
                  [disabled]="busy()"
                  [attr.aria-describedby]="hintId(o, 'prone')"
                  (click)="pick(o, choices.PRONE)"
                >
                  Técnica da Mão Aberta: Derrubar (teste de Destreza)
                </button>
                <p class="rider__hint" [id]="hintId(o, 'prone')">{{ proneHint(o) }}</p>
              </div>
              <div class="rider__opt">
                <button
                  matButton="outlined"
                  type="button"
                  [disabled]="busy()"
                  [attr.aria-describedby]="hintId(o, 'push')"
                  (click)="pick(o, choices.PUSH)"
                >
                  Empurrar até 4,5 m (teste de Força)
                </button>
                <p class="rider__hint" [id]="hintId(o, 'push')">{{ pushHint(o) }}</p>
              </div>
              <div class="rider__opt">
                <button
                  matButton="outlined"
                  type="button"
                  [disabled]="busy()"
                  [attr.aria-describedby]="hintId(o, 'none')"
                  (click)="pick(o, choices.NO_REACTIONS)"
                >
                  Sem reações até o fim do seu próximo turno
                </button>
                <p class="rider__hint" [id]="hintId(o, 'none')">{{ noReactionsHint }}</p>
              </div>
            } @else {
              <div class="rider__opt">
                <button
                  matButton="outlined"
                  type="button"
                  [disabled]="busy() || o.kiLeft < 1"
                  [attr.aria-describedby]="hintId(o, 'stun')"
                  (click)="pick(o, choices.STUN)"
                >
                  Golpe Atordoante (1 ki)
                </button>
                <p class="rider__hint" [id]="hintId(o, 'stun')">{{ stunHint(o) }}</p>
              </div>
            }
            <button matButton type="button" [disabled]="busy()" (click)="pick(o, choices.DECLINE)">Dispensar</button>
          </div>
        </div>
      </div>
      }
    }
  `,
  styles: `
    :host {
      display: contents;
    }

    .rider {
      align-items: flex-start;
    }

    .rider__body {
      display: flex;
      flex: 1;
      flex-direction: column;
      gap: 12px;
      min-width: 0;
    }

    .rider__text {
      margin: 0;
    }

    .rider__srd {
      display: flex;
      flex-direction: column;
      gap: 2px;
    }

    .rider__opt {
      display: flex;
      flex-direction: column;
      gap: 4px;
    }

    .rider__hint {
      color: var(--mr-ink-muted);
      font-size: 0.875rem;
      margin: 0;
    }

    .rider__btns {
      display: flex;
      flex-direction: column;
      gap: 10px;
    }

    .rider__btns--row {
      flex-direction: row;
      flex-wrap: wrap;
    }
  `,
})
export class HitRiderCard {
  readonly offers = input<readonly HitRiderOffer[]>([]);
  readonly combatants = input<readonly Combatant[]>([]);
  readonly busy = input(false);
  /** The campaign, to read the attacker's stat block for a creature's rider (the save's ability); the master's alone. */
  readonly campaignId = input('');

  readonly answer = output<RiderPick>();

  protected readonly choices = HitRiderChoice;
  private readonly creatures = inject(CreaturesClient);
  /** What the attacker's stat block says of each creature rider (by the offer's id): the save's ability, the SRD text and the size limit. */
  private readonly abilities = signal<ReadonlyMap<string, string>>(new Map());
  private readonly texts = signal<ReadonlyMap<string, { text: string; maxSize: string }>>(
    new Map(),
  );
  /** Why the stat block could not be read (the offer still reads, without the save's ability and the SRD text). */
  protected readonly error = signal('');
  private readonly asked = new Set<string>();
  private readonly labels = computed(() => new Map(this.combatants().map((c) => [c.id, c.label])));

  constructor() {
    // A creature's rider names the save's ability, which the offer does not carry: the attacker's stat block (kept in memory) has it.
    effect(() => {
      const campaignId = this.campaignId();
      const combatants = this.combatants();
      const offers = this.offers() ?? [];
      if (offers.length === 0) {
        untracked(() => this.error.set('')); // no offer, no failure to tell
      }
      for (const o of offers) {
        const key = combatants.find((c) => c.id === o.attackerId)?.bestiaryCreatureKey ?? '';
        if (!campaignId || !this.isCreature(o) || !key || this.asked.has(o.id)) {
          continue;
        }
        this.asked.add(o.id);
        this.creatures.statBlock(campaignId, key).then(
          (block) => {
            this.error.set('');
            const action = block?.actions.find((a) => a.key === o.actionKey);
            const ability = action?.save?.ability;
            if (action && (action.text || action.hitCondition?.maxSize)) {
              this.texts.update((m) =>
                new Map(m).set(o.id, {
                  text: action.text,
                  maxSize: action.hitCondition?.maxSize ?? '',
                }),
              );
            }
            if (ability !== undefined && ability !== Ability.UNSPECIFIED) {
              this.abilities.update((m) => new Map(m).set(o.id, abilityName(ability)));
            }
          },
          (err: unknown) => {
            // The offer reads without the ability; the next change of the offers or the combatants asks again.
            this.asked.delete(o.id);
            this.error.set(combatErrorMessage(err, 'ler a ficha da criatura'));
          },
        );
      }
    });
  }

  protected isCreature(o: HitRiderOffer): boolean {
    return o.kind === HitRiderKind.CREATURE;
  }

  /** The attack's name ("Garras"), or the rider's kind when the offer has none. */
  protected creatureTitle(o: HitRiderOffer): string {
    return o.actionNamePt || 'Efeito do ataque';
  }

  /** "Pedir teste de resistência de Brisa (CD 10, Constituição)" or "Agarrar (CD de fuga 12)", and what a failure gives. */
  protected creatureLine(o: HitRiderOffer): string {
    const target = this.label(o.targetId) || 'o alvo';
    if (o.escapeDc > 0) {
      const condition = o.conditionNamePt
        ? `: ${target} fica ${o.conditionNamePt.toLowerCase()}`
        : '';
      return `Agarrar (CD de fuga ${o.escapeDc})${condition}`;
    }
    const detail = [dcLabel(o.saveDc), this.abilities().get(o.id) ?? ''].filter(Boolean).join(', ');
    const fail = o.conditionNamePt
      ? `. Se falhar: ${o.conditionNamePt}`
      : '. Se falhar: dano extra';
    return `Pedir teste de resistência de ${target}${detail ? ` (${detail})` : ''}${fail}`;
  }

  /** The SRD's text of the action, the master's reminder of the exceptions the plan does not hold (the creature types, the sizes). */
  protected srdText(o: HitRiderOffer): string {
    return this.texts().get(o.id)?.text ?? '';
  }

  /** "Só criaturas até Grande" when the stat block sets the biggest size the effect holds. */
  protected sizeLimit(o: HitRiderOffer): string {
    const max = (this.texts().get(o.id)?.maxSize ?? '').toLowerCase();
    const size = CREATURE_SIZES.find((z) => z.value === max);
    return size ? `Só criaturas até ${size.label.toLowerCase()}` : '';
  }

  protected isOpenHand(o: HitRiderOffer): boolean {
    return o.kind === HitRiderKind.OPEN_HAND;
  }

  protected title(o: HitRiderOffer): string {
    return this.isOpenHand(o) ? 'Técnica da Mão Aberta' : 'Golpe Atordoante';
  }

  protected readonly noReactionsHint =
    'Sem teste de resistência: o alvo não pode usar reações até o fim do seu próximo turno.';

  /** A unique id for the hint a button is described by. */
  protected hintId(o: HitRiderOffer, what: string): string {
    return `rider-${o.id}-${what}`;
  }

  /** "Teste de Destreza CD 14." The DC is the monk's own ki save DC (never an NPC's number, RN-20); left out when the server sends none. */
  private saveText(ability: string, o: HitRiderOffer): string {
    return o.saveDc > 0 ? `Teste de ${ability} CD ${o.saveDc}.` : `Teste de ${ability}.`;
  }

  protected proneHint(o: HitRiderOffer): string {
    return `${this.saveText('Destreza', o)} Se falhar, fica Derrubado.`;
  }

  protected pushHint(o: HitRiderOffer): string {
    return `${this.saveText('Força', o)} Se falhar, é empurrado até 4,5 m.`;
  }

  /** What Stunning Strike costs and does; with no ki left, the reason it is off. */
  protected stunHint(o: HitRiderOffer): string {
    const cost =
      o.kiLeft < 1
        ? 'Sem ki: você não tem pontos de ki para gastar.'
        : `Gasta 1 ponto de ki (restam ${o.kiLeft}, ficam ${o.kiLeft - 1}).`;
    return `${cost} ${this.saveText('Constituição', o)} Se falhar, fica atordoado até o fim do seu próximo turno.`;
  }

  protected label(id: string): string {
    return this.labels().get(id) ?? '';
  }

  protected pick(o: HitRiderOffer, choice: HitRiderChoice): void {
    this.answer.emit({ id: o.id, choice });
  }
}
