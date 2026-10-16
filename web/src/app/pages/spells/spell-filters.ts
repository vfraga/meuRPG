import { Component, computed, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { SpellClass } from '../../core/spells/spells-client';
import { LEVEL_CHIPS, SCHOOLS, toggleLevel } from '../../core/spells/spells-filter';
import type { SpellsState } from '../../core/spells/spells-state';
import { RevealSwitch } from '../../shared/reveal-switch/reveal-switch';

/** The player's own character, for "Só as que posso aprender". */
export interface MineCharacter {
  readonly id: string;
  readonly name: string;
  /** "Pensantus, Mago 4". */
  readonly label: string;
}

/**
 * The controls of the "Magias" filters (E10-11): the name, the class, the circles (chips that toggle),
 * the school and "Só as que posso aprender". The page draws them in its left panel from 1100 px up, and
 * the "Filtros" sheet draws the same ones on a phone, so there is one set of controls and one meaning.
 * Every change goes to the state, which asks the server; nothing is filtered here.
 */
@Component({
  selector: 'app-spell-filters',
  imports: [MatIconModule, RevealSwitch],
  templateUrl: './spell-filters.html',
  styleUrl: './spell-filters.scss',
})
export class SpellFilters {
  readonly state = input.required<SpellsState>();
  readonly classes = input<readonly SpellClass[]>([]);
  readonly mine = input<MineCharacter | null>(null);
  /** The name field belongs to the panel; on a phone it sits on the page, above "Filtros". */
  readonly showSearch = input(false);

  protected readonly schools = SCHOOLS;
  protected readonly chips = LEVEL_CHIPS;
  protected readonly filter = computed(() => this.state().filter());
  protected readonly mineHint = computed(() => {
    const m = this.mine();
    return m ? `${this.filter().onlyMine ? 'Ligado' : 'Desligado'} · ${m.label}.` : '';
  });

  protected toggle(level: number): void {
    void this.state().change({ levels: toggleLevel(this.filter().levels, level) });
  }

  protected allLevels(): void {
    void this.state().change({ levels: [] });
  }
}
