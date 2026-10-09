import { Component, computed, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import { LevelUpSpellsKind } from '../../../../gen/meurpg/characters/v1/characters_pb';
import type { PickItem } from '../../../core/levelup/levelup-flow';
import { ChangeRowsList } from '../change-rows/change-rows';
import { LevelUpSession } from '../level-up-session';
import { PickList } from '../pick-list/pick-list';

const LIST = new Intl.ListFormat('pt-BR', { type: 'conjunction' });

/** "de 1º nível", "de 1º ou 2º nível", "de 1º a 3º nível". */
function circles(max: number): string {
  return max <= 1 ? 'de 1º nível' : max === 2 ? 'de 1º ou 2º nível' : `de 1º a ${max}º nível`;
}

/**
 * Step "Magias" of the guided level-up (MR-040, E8-15): the new cantrips, the spells for the
 * spellbook or the spells known (with a search and "Ver os outros N"), the spells to prepare
 * in the new slots (up to the new maximum), and the slots that arrive by themselves. Each
 * spell has the "?" with its description. The lists and the counts come from the server's
 * options and the campaign's content; the maximum of prepared spells follows the preview, since
 * an ability increase moves it.
 */
@Component({
  selector: 'app-spells-step',
  imports: [ChangeRowsList, MatIconModule, PickList],
  templateUrl: './spells-step.html',
  styleUrl: './spells-step.scss',
})
export class SpellsStep {
  readonly s = input.required<LevelUpSession>();

  protected readonly cantrips = computed(() => {
    const d = this.s().draft;
    const n = d.cantripsAsked();
    return {
      n,
      title: n === 1 ? 'Truque novo' : 'Truques novos',
      lead: `Escolha ${n} dos ${d.cantripItems().length} truques de ${d.listName()} que você ainda não conhece.`,
    };
  });

  protected readonly spells = computed(() => {
    const s = this.s();
    const n = s.draft.spellsAsked();
    const o = s.draft.effective();
    const book = o.spellsKind === LevelUpSpellsKind.SPELLBOOK;
    const where = book ? 'para copiar no livro' : 'para aprender';
    const secrets = o.anyClassSpells;
    const lead = `Escolha ${n} ${n === 1 ? 'magia' : 'magias'} ${circles(o.maxSpellLevel)} ${where}.`;
    return {
      n,
      title: book ? 'Livro de magias' : 'Magias conhecidas',
      // Magical Secrets: some of them may come from any class's list.
      lead:
        secrets > 0
          ? `${lead} Até ${secrets} de qualquer classe; as outras, da lista de ${s.draft.listName()}.`
          : lead,
    };
  });

  protected readonly prepared = computed(() => {
    const s = this.s();
    const d = s.draft;
    const names = s.draft.have.prepared.map(
      (k) => d.names().get(k) ?? 'uma magia que saiu da lista',
    );
    const book = d.effective().spellsKind === LevelUpSpellsKind.SPELLBOOK;
    const n = d.preparedAsked();
    // The line follows the picks: it counts what is left to prepare, and goes once none is.
    const left = Math.max(0, n - d.prepared().size);
    return {
      n,
      base: d.have.prepared.length,
      strong: left > 0 ? `Prepare mais ${left}.` : '',
      note:
        `Você prepara até ${d.preparedMaxAfter()} magias${book ? ' do livro' : ''} (eram ${s.options.preparedMax}).` +
        (names.length > 0 ? ` Já preparadas: ${LIST.format(names)}.` : ''),
      max: d.preparedMaxAfter(),
      swap: book,
    };
  });

  protected readonly slots = computed(() =>
    this.s()
      .rows()
      .filter((r) => r.key.startsWith('slots-') || r.key === 'pact'),
  );

  protected describe(item: PickItem): void {
    this.s().describeSpell(item.key, item.name);
  }
}
