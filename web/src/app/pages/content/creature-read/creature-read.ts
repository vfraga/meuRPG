import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  input,
  signal,
  untracked,
} from '@angular/core';

import type { Creature } from '../../../../gen/meurpg/rules/v1/rules_pb';
import type { TableEntry } from '../../../../gen/meurpg/rules/v1/table_content_pb';
import type { CatalogVm } from '../../../core/content/catalog';
import { carriersOf } from '../../../core/content/creature-carriers';
import { readCreatureForMaster } from '../../../core/content/creature-phone-read';
import { creatureSlug } from '../../../core/creatures/bestiary-format';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { EntryRead } from '../entry-read/entry-read';

/**
 * A table creature on a phone, for the master (#578): the numbers, the damage lines, the traits and the attacks, and what
 * brings it. Read only (the editor is on a laptop). It reads the base's stat block for the lines the base gives, as the
 * editor's side panel does; until it comes, the lines the table wrote are already there.
 */
@Component({
  selector: 'app-creature-read',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [EntryRead],
  template: `@if (read(); as r) {
    <app-entry-read [read]="r" />
  }`,
  styles: `
    :host {
      display: block;
    }
  `,
})
export class CreatureRead {
  private readonly creatures = inject(CreaturesClient);

  readonly campaignId = input.required<string>();
  readonly entry = input.required<TableEntry>();
  readonly entries = input<readonly TableEntry[]>([]);
  readonly catalog = input.required<CatalogVm>();

  /** The base's stat block, once read; `null` before, and when it could not be read. */
  private readonly base = signal<Creature | null>(null);
  private seq = 0;

  protected readonly read = computed(() => {
    const body = this.entry().body;
    if (body.case !== 'tableCreature') {
      return null;
    }
    const baseName = this.base()?.summary?.namePt || creatureSlug(body.value.baseKey);
    return readCreatureForMaster(
      body.value,
      this.base(),
      baseName,
      (k) => this.catalog().nameOf(k),
      carriersOf(this.entries(), this.entry().key),
    );
  });

  /** The base's key: a computed, so an edit of anything else (archiving) does not read the stat block again. */
  private readonly baseKey = computed(() => {
    const body = this.entry().body;
    return body.case === 'tableCreature' ? body.value.baseKey : '';
  });

  constructor() {
    effect(() => {
      const key = this.baseKey();
      untracked(() => void this.readBase(key));
    });
  }

  private async readBase(key: string): Promise<void> {
    const mine = ++this.seq;
    try {
      const block = key === '' ? null : await this.creatures.statBlock(this.campaignId(), key);
      if (mine === this.seq) {
        this.base.set(block);
      }
    } catch {
      if (mine === this.seq) {
        this.base.set(null);
      }
    }
  }
}
