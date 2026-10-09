import { Component, computed, input, output } from '@angular/core';
import { MatCheckboxModule } from '@angular/material/checkbox';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';

import { RouterLink } from '@angular/router';

import { TableMark } from '../../../shared/table-mark/table-mark';
import { spellLevelLabel } from '../../../core/characters/character-labels';
import type { OutsideSpell } from '../class-blocks';
import { SpellOptionVm } from '../character-editor.types';
import { countLabel } from '../editor-labels';

/**
 * One catalog-backed spell list on the "Magias" step (Truques, Magias
 * conhecidas or Magias preparadas): a title, the names already chosen, a
 * search box and a scrolling list of checkboxes. Picks are content keys
 * from the catalog, never typed text. The selection and the filter stay in
 * `CharacterEditor`; this only shows them and reports changes.
 */
@Component({
  selector: 'app-spell-picker',
  imports: [
    MatCheckboxModule,
    MatFormFieldModule,
    MatIconModule,
    MatInputModule,
    RouterLink,
    TableMark,
  ],
  templateUrl: './spell-picker.html',
  styleUrl: './spell-picker.scss',
})
export class SpellPicker {
  /** The id of the title, which also names the checkbox group. */
  readonly titleId = input.required<string>();
  readonly title = input.required<string>();
  readonly searchLabel = input.required<string>();
  /** Every option of the chosen class's list, for the "chosen" line. */
  readonly options = input.required<readonly SpellOptionVm[]>();
  /** `options` narrowed by `filter`. */
  readonly filtered = input.required<readonly SpellOptionVm[]>();
  readonly selected = input.required<ReadonlySet<string>>();
  readonly filter = input.required<string>();
  /** "truque" / "magia", for the count and the empty states. */
  readonly noun = input.required<'truque' | 'magia'>();
  readonly showLevel = input(false);
  /** The highest circle the character reaches, or `null` for no limit. A
   * listed spell above it is one the player had already picked (the level
   * was lowered): it stays so it can be unchecked, and is marked. */
  readonly maxSpellLevel = input<number | null>(null);

  /** The most the class takes in this list, or `null` for no limit to show. The count reads "2 de 3". */
  readonly limit = input<number | null>(null);
  /** Warn "Prepare até N" while fewer than the limit are chosen (the prepared list of a preparing class). */
  readonly warnBelowLimit = input(false);

  /** What the search finds that no list of the sheet has: greyed, with the reason. */
  readonly outside = input<readonly OutsideSpell[]>([]);
  /** Where "Ver em Magias" goes. */
  readonly magiasLink = input<readonly string[]>([]);

  readonly toggle = output<string>();
  readonly filterChange = output<string>();
  /** The "?" next to a spell was pressed: the page opens its description. */
  readonly describe = output<SpellOptionVm>();

  protected readonly spellLevelLabel = spellLevelLabel;

  /** "3º nível", or "3º nível, acima do nível" past the limit. */
  protected levelText(level: number): string {
    const max = this.maxSpellLevel();
    return max !== null && level > max
      ? `${spellLevelLabel(level)}, acima do nível`
      : spellLevelLabel(level);
  }

  protected readonly chosen = computed(() =>
    this.options()
      .filter((spell) => this.selected().has(spell.key))
      .map((spell) => spell.namePt),
  );

  protected readonly count = computed(() => {
    const n = this.chosen().length;
    const limit = this.limit();
    if (limit !== null) {
      return this.noun() === 'truque'
        ? `${n} de ${limit} ${limit === 1 ? 'truque escolhido' : 'truques escolhidos'}`
        : `${n} de ${limit} ${limit === 1 ? 'magia escolhida' : 'magias escolhidas'}`;
    }
    return this.noun() === 'truque'
      ? countLabel(n, 'truque escolhido', 'truques escolhidos', 'Nenhum truque escolhido')
      : countLabel(n, 'magia escolhida', 'magias escolhidas', 'Nenhuma magia escolhida');
  });

  /** "Prepare até 4": fewer are chosen than the class prepares. */
  protected readonly prepareWarning = computed(() => {
    const limit = this.limit();
    return this.warnBelowLimit() && limit !== null && this.chosen().length < limit
      ? `Prepare até ${limit}`
      : '';
  });

  /** "2 truques escolhidos: Mãos Mágicas, Raio de Fogo". */
  protected readonly chosenLine = computed(() => {
    const names = this.chosen();
    return names.length > 0 ? `${this.count()}: ${names.join(', ')}` : this.count();
  });

  protected readonly emptyText = computed(() => {
    const truque = this.noun() === 'truque';
    if (this.filter().trim()) {
      return truque
        ? 'Nenhum truque com esse nome. Apague a busca para ver a lista toda.'
        : 'Nenhuma magia com esse nome. Apague a busca para ver a lista toda.';
    }
    return truque
      ? 'Nenhum truque encontrado para a classe escolhida.'
      : 'Nenhuma magia encontrada para a classe escolhida.';
  });
}
