import { Component, input } from '@angular/core';

import { combatantInitial } from '../../core/combat/combat-view';

/** One kind of creature of the saved encounter, with what it becomes ("Bugbear 1 e 2"). */
export interface SavedMonsterRow {
  readonly key: string;
  readonly namePt: string;
  readonly count: number;
  readonly becomes: string;
}

/**
 * "Monstros do encontro" in "Iniciar combate" (E10-09 state 7): read-only lines, the square token (an NPC), the creature, "Vira
 * Bugbear 1 e 2" and how many. The encounter is the battle point's: the master changes how the monsters start, not which come.
 */
@Component({
  selector: 'app-saved-monsters',
  template: `
    <ul class="mon__list">
      @for (m of rows(); track m.key) {
        <li class="mon__row">
          <span class="mon__tk" aria-hidden="true">{{ initial(m.namePt) }}</span>
          <span class="mon__t">
            <span class="mon__n">{{ m.namePt }}</span>
            <span class="mon__s">Vira {{ m.becomes }}</span>
          </span>
          <span class="mon__c"><span aria-hidden="true">×</span> {{ m.count }}<span class="mr-visually-hidden"> monstros</span></span>
        </li>
      }
    </ul>
  `,
  styles: `
    :host {
      display: block;
    }

    .mon__list {
      margin: 0;
      padding: 0;
      list-style: none;
    }

    .mon__row {
      display: grid;
      grid-template-columns: 38px minmax(0, 1fr) auto;
      align-items: center;
      gap: var(--mr-space-3);
      min-height: 60px;
      padding: 8px 0;
      border-top: 1px solid var(--mr-rule);
    }

    .mon__tk {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: 38px;
      height: 38px;
      box-sizing: border-box;
      border: 2px solid var(--mr-ink);
      border-radius: 8px;
      font-family: var(--mr-font-display);
      font-weight: 800;
      font-size: 18px;
    }

    .mon__t {
      display: flex;
      flex-direction: column;
      min-width: 0;
    }

    .mon__n {
      font-family: var(--mr-font-display);
      font-size: 20px;
      font-weight: 700;
      line-height: 24px;
      overflow-wrap: anywhere;
    }

    .mon__s {
      font-size: 14px;
      line-height: 18px;
      color: var(--mr-ink-muted);
    }

    .mon__c {
      font-family: var(--mr-font-display);
      font-size: 20px;
      font-weight: 800;
    }
  `,
})
export class SavedMonstersList {
  readonly rows = input.required<readonly SavedMonsterRow[]>();
  protected readonly initial = combatantInitial;
}
