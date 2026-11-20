import {
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
  type Combatant,
  CreatureAttack,
  type GetTurnOptionsResponse,
  type PendingDamage,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { ActionOption, AttackOption } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { attackDetail, attackName, reasonText } from '../../../../core/combat/combat-options';
import { article } from '../../../../core/combat/combat-log';
import { withArticle } from '../../../../core/format/article';
import { stateWord } from '../../../../core/combat/combat-view';
import { CreaturesClient } from '../../../../core/creatures/creatures-client';
import { standUpRow } from '../../../../core/combat/stand-up';
import { tight } from '../../../../core/format/text';
import type { FallNote } from '../../../../core/traps/trap-log';
import { metersFixed } from '../../../../core/units';
import { CreatureArt } from '../../../../shared/creatures/creature-art';
import { ActionRow } from '../action-groups/action-row';
import { PartState } from '../joint-turn/part-state';

/**
 * One creature of the player on the page of its turn (MR-037, E9-12 states 3 and 6): the picture, the name, its
 * hit points (the owner's and the master's; RN-20) and armor class (the book's: the combat never sends an armor
 * class to a player), the state of its part in a joint turn, its action and its movement, then what it can do:
 *   - a creature that attacks: its attacks with "Atacar" and "Mover o Lobo atroz 1";
 *   - a familiar, which does not attack: "O familiar não ataca. Ele usa as outras ações:" and the standard actions;
 *   - the familiar of the Pacto da Corrente, which attacks with its reaction: a "Reação" row with "Atacar".
 * Each creature has its own action and its own movement: attacking with one never spends another's. Off its turn
 * every button is off with the reason in a line. The numbers and the lists are what the server sent (`GetTurnOptions`
 * on the creature); nothing is worked out here.
 */
@Component({
  selector: 'app-creature-block',
  imports: [ActionRow, CreatureArt, MatButtonModule, MatIconModule, PartState],
  templateUrl: './creature-block.html',
  styleUrl: './creature-block.scss',
})
export class CreatureBlock {
  private readonly client = inject(CreaturesClient);

  readonly campaignId = input.required<string>();
  readonly creature = input.required<Combatant>();
  /** Its options (`GetTurnOptions`); `null` until they are read. */
  readonly options = input<GetTurnOptionsResponse | null>(null);
  /** It may act now (its part of the turn is running). */
  readonly acting = input(false);
  /** Part of a joint turn of two or more: the state pill says whether its part ended. */
  readonly joint = input(false);
  readonly busy = input(false);
  /** Why nothing can be done now ("Esperando a reação do mestre"); empty when free. */
  readonly locked = input('');
  /** A hit of this creature whose damage is not rolled yet (the sheet was closed): it blocks ending the turn. */
  readonly pending = input<PendingDamage | null>(null);
  /** What a trap did to this creature this round (9.14's note, for the creature whose part is showing). */
  readonly trapNote = input<FallNote | null>(null);

  /** "Atacar": the attack's key. */
  readonly attack = output<string>();
  /** A reaction attack of the familiar of the Pacto da Corrente. */
  readonly reactionAttack = output<string>();
  /** A standard action, by key ("standard:dash"). */
  readonly action = output<string>();
  readonly move = output<void>();
  /** "Levantar-se" of a prone creature (half its speed). */
  readonly standUp = output<void>();
  /** "Rolar o dano" of the pending hit. */
  readonly rollDamage = output<void>();

  /** The armor class of its book; the combat does not send one to a player (RN-20). */
  protected readonly ac = signal<number | null>(null);

  protected readonly c = computed(() => this.creature());
  protected readonly mode = computed(() => this.creature().creatureAttack);
  protected readonly hp = computed(() => {
    const c = this.creature();
    const own =
      c.hitPointsCurrent !== undefined && c.hitPointsMax !== undefined
        ? `PV ${c.hitPointsCurrent} de ${c.hitPointsMax}`
        : stateWord(c.state);
    return tight(
      c.hitPointsCurrent !== undefined && this.ac() !== null ? `${own} · CA ${this.ac()}` : own,
    );
  });
  protected readonly ended = computed(() => this.creature().turnPartEnded);
  /** The reason every button is off: not its turn, its part ended, or the turn waits for an answer. */
  protected readonly why = computed(() => {
    if (this.locked()) {
      return this.locked();
    }
    if (!this.acting()) {
      return this.ended()
        ? 'Esta parte do turno já foi encerrada.'
        : `Ainda não é a vez ${withArticle(this.creature().label, 'de')}.`;
    }
    return '';
  });
  protected readonly off = computed(() => !!this.why());
  protected readonly actionWord = computed(() =>
    this.creature().actionUsed ? 'Usada' : 'Disponível',
  );
  protected readonly flies = computed(() => this.creature().speedFlyFt > 0);
  protected readonly moveLeft = computed(() =>
    tight(metersFixed(this.creature().movementLeftDft / 10)),
  );
  protected readonly moveTotal = computed(() =>
    tight(metersFixed(Math.max(1, this.creature().speedDft) / 10)),
  );
  protected readonly standRow = computed(() => standUpRow(this.creature()));
  protected readonly moveLabel = computed(() => `Mover o ${this.creature().label}`);

  /** What it attacks with as an action; a familiar of the chain attacks with its reaction instead. */
  protected readonly attacks = computed<readonly AttackOption[]>(() =>
    this.mode() === CreatureAttack.FULL ? (this.options()?.options?.attacks ?? []) : [],
  );
  protected readonly reactionAttacks = computed<readonly AttackOption[]>(() =>
    this.mode() === CreatureAttack.REACTION ? (this.options()?.options?.attacks ?? []) : [],
  );
  protected readonly cannotAttack = computed(() => this.mode() === CreatureAttack.NONE);
  /** The actions everyone has, without Atacar and Conjurar (a creature has the first only as its attacks). */
  protected readonly standard = computed<readonly ActionOption[]>(() =>
    (this.options()?.options?.standardActions ?? []).filter(
      (a) => !/^standard:(attack|cast-a-spell)$/.test(a.action?.key ?? ''),
    ),
  );

  /** "do Lobo atroz 1", "da Cobra 2". */
  protected readonly ofCreature = computed(
    () => `${article(this.creature().label) === 'a' ? 'da' : 'do'} ${this.creature().label}`,
  );
  protected readonly attackName = attackName;
  protected readonly reasonText = reasonText;
  protected readonly reactionWhy = computed(() =>
    this.creature().reactionUsed ? 'Reação já usada' : '',
  );

  constructor() {
    // The book's armor class, read once per kind (the client keeps it).
    effect(() => {
      const key = this.creature().monsterKey;
      untracked(() => {
        if (!key) {
          return;
        }
        void this.client.statBlock(this.campaignId(), key).then(
          (block) => this.ac.set(block.armorClass),
          () => this.ac.set(null),
        );
      });
    });
  }

  protected detail(o: AttackOption): string {
    return o.attack ? attackDetail(o.attack, true) : '';
  }
}
