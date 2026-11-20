import { computed, effect, signal, untracked } from '@angular/core';
import type { Observable } from 'rxjs';

import { type AttunementEntryVm, magicEntryOf } from '../../../core/characters/attunement';
import { combatItemRows, type TurnBlock } from '../../../core/characters/consumables';
import type { ChargesClient, TableDice } from '../../../core/play/charges-client';
import { NewestRead } from '../../../core/system/newest-read';
import { ConsumableUse } from '../../../shared/consumable-use/consumable-use';
import type { UseConsumableData } from '../../../shared/consumable-use/use-consumable-sheet';

/** What the combat's "Itens" reads from the screen it serves. */
export interface CombatItemsContext {
  readonly client: () => ChargesClient;
  readonly campaignId: () => string;
  /** The character whose items the screen shows: the player's own, or the one the master acts for; empty when there is none. */
  readonly characterId: () => string;
  readonly name: () => string;
  /** The master acts for the character: the sheet reads "Usar por Tor". */
  readonly byMaster: () => boolean;
  /** It is the turn the items are for (the tab is drawn only on the character's own turn): nothing is read otherwise. */
  readonly enabled: () => boolean;
  /** What of the combat changes what the items are or may cost (the turn, the Action): they are read again only when it changes. */
  readonly signature: () => string;
  /** What the combat knows that keeps a potion from the Action (`turnBlockOf`): the sheet then says so and offers nothing. */
  readonly turnBlock: () => TurnBlock | null;
  readonly dice: () => Promise<TableDice>;
  readonly openUse: (data: UseConsumableData) => Observable<boolean | undefined>;
  /** Puts the focus back on the item's "Usar" once the page has drawn it again (or on the tab, when the item is gone or folded). */
  readonly focusItem: (key: string) => void;
}

/**
 * The potions and scrolls of the character on turn, for the combat's "Itens" tab: the character's magic items
 * read from its sheet, the consumables among them grouped in rows, and "Usar", which opens the same sheet as the character
 * sheet's with a combat running (`ConsumableUse`). The items are read when the screen's character is known and on the character's
 * turn, and again whenever the turn's economy changes and after a use; a read that fails keeps what is shown.
 */
export class CombatItems {
  private readonly entries = signal<readonly AttunementEntryVm[]>([]);
  readonly rows = computed(() => combatItemRows(this.entries()));
  /** The use of an item: the sheet, the calls and the notice. */
  readonly use: ConsumableUse;
  /** What the last use said, for the card at the top of the tab. */
  readonly notice = computed(() => this.use.result());
  private loadedFor = '';

  /** Built where Angular can run an effect (a field of the screen). */
  constructor(private readonly ctx: CombatItemsContext) {
    this.use = new ConsumableUse({
      client: ctx.client,
      campaignId: ctx.campaignId,
      owner: () => {
        const id = ctx.characterId();
        return id
          ? { id, name: ctx.name(), byMaster: ctx.byMaster(), entries: this.entries() }
          : null;
      },
      inCombat: () => true,
      turnBlock: ctx.turnBlock,
      reload: () => this.read(),
      dice: ctx.dice,
      focusConsumable: ctx.focusItem,
      openUse: ctx.openUse,
    });
    effect(() => {
      const id = ctx.characterId();
      ctx.signature();
      if (!ctx.enabled() || !id) {
        return;
      }
      untracked(() => {
        void this.read();
      });
    });
  }

  /** "Usar" on a row: `entry` is the unit the row sends. */
  open(entry: AttunementEntryVm): void {
    this.use.openUse(entry);
  }

  /** A turn that passes does not carry the last use's words into the next one. */
  forgetNotice(): void {
    this.use.result.set(null);
  }

  private readonly reads = new NewestRead();

  /** Reads the items of the screen's character; `true` when the newest read of them is on screen. */
  private read(): Promise<boolean> {
    const characterId = this.ctx.characterId();
    return characterId
      ? this.reads.start(characterId, (seq) => this.fetch(characterId, seq))
      : Promise.resolve(false);
  }

  private async fetch(characterId: string, seq: number): Promise<boolean> {
    if (this.loadedFor !== characterId) {
      this.loadedFor = characterId;
      this.entries.set([]);
    }
    try {
      const sheet = await this.ctx.client().sheetOf(this.ctx.campaignId(), characterId);
      if (!this.reads.holds(characterId, seq) || characterId !== this.ctx.characterId()) {
        // A newer read of this character owns the screen: the answer is its own, not a refusal to say "on screen".
        return characterId === this.ctx.characterId()
          ? this.reads.overtaken(characterId, seq)
          : false;
      }
      this.entries.set(sheet.magicItems.map(magicEntryOf));
      return true;
    } catch {
      return false;
    }
  }
}
