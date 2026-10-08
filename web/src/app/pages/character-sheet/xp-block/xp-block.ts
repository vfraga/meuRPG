import { Component, computed, input } from '@angular/core';

import { formatInt, tight } from '../../../core/format/text';
import { progress } from '../../../core/progression/xp-labels';
import { LevelUpTag } from '../../../shared/xp/level-up-tag';

/**
 * The XP on the character sheet (E7-10, MR-016): a block to read, not a field
 * to type in. The number in the display face, a bar towards the next level, the
 * sentence that says it in words ("2.366 de 2.700 XP para o nível 4. Faltam 334
 * XP.") and, when the character can level up, "Pode subir de nível" with what to
 * do. Only the master's awards change it, so the player does not type it any
 * more. The block is a polite live region: when the master gives XP and the
 * page reads the character again, the new number is announced.
 *
 * In a campaign that levels by milestones there is no block (there is no XP);
 * the sheet's header shows only the tag.
 */
@Component({
  selector: 'app-xp-block',
  imports: [LevelUpTag],
  template: `
    <section class="xp" role="status" aria-live="polite" aria-atomic="true">
      <span class="xp__label">Experiência</span>
      <div class="xp__top">
        <span class="xp__n">{{ xpText() }}</span>
        @if (canLevelUp() && !selfLevelUp()) {
          <app-level-up-tag />
        }
      </div>
      <div class="xp__bar" aria-hidden="true">
        <span class="xp__fill" [style.width.%]="bar().percent"></span>
      </div>
      @if (sentence()) {
        <p class="xp__line">{{ sentence() }}</p>
      }
    </section>
  `,
  styleUrl: './xp-block.scss',
})
export class XpBlock {
  readonly xp = input.required<number>();
  /** The XP of the next level; 0 at level 20, or while the sheet has no class. */
  readonly nextLevelXp = input.required<number>();
  /** The total level; 0 while the sheet has no class. */
  readonly level = input.required<number>();
  readonly canLevelUp = input(false);
  /** The master reads the same block: the instruction is for him. */
  readonly isMaster = input(false);
  /** The owner of a locked sheet levels up by the block under the header: the tag and the "O mestre sobe" line step aside. */
  readonly selfLevelUp = input(false);

  protected readonly xpText = computed(() => tight(`${formatInt(this.xp())} XP`));
  protected readonly bar = computed(() =>
    progress(this.xp(), this.nextLevelXp(), this.canLevelUp()),
  );
  protected readonly sentence = computed(() => {
    const next = this.nextLevelXp();
    const xp = this.xp();
    if (next <= 0) {
      return this.level() >= 20 ? 'Nível máximo.' : '';
    }
    const target = `o nível ${this.level() + 1}`;
    if (this.canLevelUp()) {
      return tight(
        `Chegou aos ${formatInt(next)} XP d${target}. ${this.isMaster() ? 'Suba o nível na ficha.' : this.selfLevelUp() ? 'Suba o nível pelo botão abaixo.' : 'O mestre sobe o seu nível na ficha.'}`,
      );
    }
    // XP past the threshold without the level offered (a dead character): nothing is missing, so nothing is said to be.
    const missing = xp < next ? ` Faltam ${formatInt(next - xp)} XP.` : '';
    return tight(`${formatInt(xp)} de ${formatInt(next)} XP para ${target}.${missing}`);
  });
}
