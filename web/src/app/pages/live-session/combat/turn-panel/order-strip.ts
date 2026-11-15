import { NgTemplateOutlet } from '@angular/common';
import { Component, computed, input } from '@angular/core';

import type { Combatant, Encounter } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { conditionTags } from '../../../../core/combat/conditions';
import { labelTags } from '../../../../core/effects/effects';
import { coverMarkTags, sideTags } from '../../../../core/combat/cover';
import {
  combatantInitial,
  isPlayer,
  playerWord,
  stateWord,
} from '../../../../core/combat/combat-view';
import { isCreature } from '../../../../core/combat/creature-names';
import { CombatantToken } from '../../../../shared/combatant-token/combatant-token';
import {
  type OrderItem,
  jointTurn,
  orderItems,
  groupAriaLabel,
} from '../../../../core/combat/joint-turn';
import { OrderGroup } from '../joint-turn/order-group';
import { CombatantTags } from '../combatant-tags/combatant-tags';
import { FormTag } from '../combatant-tags/form-tag';

/**
 * The order as a strip of chips (E6-05): a token, the name and one word. An
 * NPC says its state (Ileso, Ferido, Muito ferido, Derrotado), another
 * player only "Jogador" (their name is not sent to the other players), and
 * the one on turn has the 2px accent frame and the word "Vez"; the player's
 * own chip says "Você". A character at 0 hit points reads "Caída" (never
 * "Morrendo": that word is the master's), and a combatant with conditions has
 * its first tag and "+N" for the rest (E6-29). Hidden combatants are not in the
 * list a player gets.
 */
@Component({
  selector: 'app-order-strip',
  imports: [CombatantTags, FormTag, CombatantToken, NgTemplateOutlet, OrderGroup],
  template: `
  <ol class="strip" aria-label="Ordem de iniciativa" tabindex="0">
    @for (item of items(); track key(item)) {
      @if (item.kind === 'group') {
        <li class="group-item">
          <app-order-group class="dense" [total]="item.total" [onTurn]="item.onTurn" [mounted]="item.mounted" [label]="groupLabel(item)">
            <ol class="group__chips">
              @for (c of item.members; track c.id) {
                <ng-container *ngTemplateOutlet="chipTpl; context: { $implicit: c }" />
              }
            </ol>
          </app-order-group>
        </li>
      } @else {
        <ng-container *ngTemplateOutlet="chipTpl; context: { $implicit: item.combatant }" />
      }
    }
  </ol>
  <ng-template #chipTpl let-c>
      <li class="chip" [class.chip--turn]="current(c)">
        <span class="chip__top">
          <app-combatant-token [initial]="initial(c)" [npc]="npc(c)" [creature]="creature(c)" [defeated]="c.defeated" [mine]="c.mine" [size]="24" />
          @if (current(c)) {
            <span class="chip__word chip__word--turn">Vez</span>
          } @else if (c.mine) {
            <span class="chip__word">Você</span>
          }
        </span>
        <span class="chip__name" [class.chip__name--out]="c.defeated">{{ c.label }}</span>
        <span class="chip__sub">{{ word(c) }}</span>
        @if (c.wildShapeBeastKey) {
          <app-form-tag [beast]="c.wildShapeBeastNamePt" />
        }
        <app-combatant-tags [names]="tags(c)" [label]="c.label" [states]="c.states" [sources]="c.conditionSources" [people]="encounter().combatants" [compact]="true" />
      </li>
  </ng-template>
  `,
  styles: `
:host {
  display: block;
}

.strip {
  // The containing block of the chips' absolutely placed screen-reader text (combatant-tags' .sr): without it that text
  // anchors to the page and, at the end of a long order, widens the whole page past the window (rehearsal 5, 1024 px).
  position: relative;
  display: flex;
  gap: 8px;
  margin: 0;
  padding: 0 0 2px;
  list-style: none;
  overflow-x: auto;
  scroll-snap-type: x proximity;
}

.chip {
  display: flex;
  // 104px, or as wide as its one tag needs ("Concentração"), up to 150px.
  flex: 0 0 auto;
  min-width: 104px;
  max-width: 150px;
  flex-direction: column;
  gap: 3px;
  box-sizing: border-box;
  min-height: 68px;
  padding: 8px 10px;
  border: 1px solid var(--mr-line);
  border-radius: var(--mr-radius-md);
  background: var(--mr-surface);
  scroll-snap-align: start;
}

.chip--turn {
  padding: 7px 9px;
  border: 2px solid var(--mr-accent);
  background: var(--mr-accent-soft);
}

.group__chips {
  display: flex;
  padding: 0 0 2px;
  gap: 6px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.group-item {
  flex: none;
  scroll-snap-align: start;
}

.group__chips {
  padding: 4px 6px 6px;
}

.group-item .chip--turn {
  padding: 8px 10px;
  border: 1px solid var(--mr-line);
  background: var(--mr-surface);
}

.chip__top {
  display: flex;
  align-items: center;
  gap: 6px;
}

.chip__word {
  font-size: var(--mr-font-13);
  font-weight: 700;
  color: var(--mr-ink-muted);
}

.chip__word--turn {
  color: var(--mr-accent-text);
}

.chip__name {
  font-size: var(--mr-font-15);
  font-weight: 700;
  line-height: 18px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.chip__name--out {
  color: var(--mr-ink-muted);
  text-decoration: line-through;
}

.chip__sub {
  font-size: var(--mr-font-14);
  line-height: 17px;
  color: var(--mr-ink-muted);
}

  `,
})
export class OrderStrip {
  readonly encounter = input.required<Encounter>();

  protected initial(c: Combatant): string {
    return combatantInitial(c.label);
  }

  protected npc(c: Combatant): boolean {
    return !isPlayer(c) && !isCreature(c);
  }

  protected creature(c: Combatant): boolean {
    return isCreature(c);
  }

  protected word(c: Combatant): string {
    const word = isPlayer(c) ? playerWord(c) : stateWord(c.state);
    return sideTags(c).length > 0 ? `${word} · Aliado` : word;
  }

  protected tags(c: Combatant): string[] {
    // The chip is 104 px: the cover says its degree only ("Meia cobertura"); the full words ("marcada pelo mestre") are in the lists and the cast.
    return [
      ...(c.concentrationSpell ? ['Concentração'] : []),
      ...labelTags(conditionTags(c), c.effectLabels),
      ...coverMarkTags(c).map((t) => t.replace(' · marcada pelo mestre', '')),
    ];
  }

  protected readonly items = computed(() => orderItems(this.encounter(), false));
  private readonly joint = computed(() => jointTurn(this.encounter()));

  /** In a joint turn the box is on turn, not one chip. */
  protected current(c: Combatant): boolean {
    return !this.joint() && c.id === this.encounter().currentCombatantId;
  }

  protected key(item: OrderItem): string {
    return item.kind === 'group' ? item.members.map((m) => m.id).join('+') : item.combatant.id;
  }

  protected readonly groupLabel = groupAriaLabel;
}
