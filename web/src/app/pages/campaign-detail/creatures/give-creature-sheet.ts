import { Component, computed, DestroyRef, effect, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';

import type { Creature, CreatureSummary } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { creatureErrorMessage } from '../../../core/creatures/creature-errors';
import { CREATURE_TYPES } from '../../../core/creatures/creature-types';
import {
  CREATURE_NAME_MAX,
  challengeText,
  nameCounter,
  summarySubtitle,
} from '../../../core/creatures/creature-format';
import { ActionKey } from '../../../core/connect/idempotency';
import { formatInt, joinDots } from '../../../core/format/text';
import { SheetFrame } from '../../live-session/combat/sheet-frame/sheet-frame';
import { injectSheet } from '../../live-session/combat/sheet-host';
import { type ChoiceRow, CreatureChoiceList } from '../../../shared/creatures/creature-choice-list';

/** What the campaign's list hands the dialog. */
export interface GiveCreatureData {
  readonly campaignId: string;
  readonly characterId: string;
  readonly characterName: string;
}

/** The creature given, for the list's live notice. */
export interface GiveCreatureResult {
  readonly name: string;
}

/** The type filter: any, then the SRD's creature types (the bestiary's filter uses the same list). */
const TYPES: readonly { value: string; label: string }[] = [
  { value: '', label: 'Qualquer' },
  ...CREATURE_TYPES,
];

/** The challenge ratings the filter offers: "Até 1/8" means that rating or lower. */
const CRS = ['0', '1/8', '1/4', '1/2', '1', '2', '3', '4', '5', '6', '8', '10', '15', '20', '30'];

/**
 * "Dar uma criatura a Toren" (E9-10, quadro 5): the master searches the SRD's
 * 334 creatures by name (Portuguese or English, while typing), type and highest
 * challenge rating, picks one, names it (the book's name to start) and gives
 * it. Three fields of 44 px side by side, the count of what passes the filters,
 * a list that scrolls inside the dialog, the name, and the footer's two
 * buttons: the one filled button says what happens ("Dar o Mastim a Toren").
 * Desktop only: on a phone the master reads the creatures and gives none (the
 * list hides the button, as painting a map is also a computer's task).
 *
 * The catalog row has no armor class or hit points (`CreatureSummary`), so the
 * line under the picked creature says them from its stat block, read when it
 * is picked. The server decides everything else (`GiveCreature`: 40 creatures
 * at most, a living player's character).
 */
@Component({
  selector: 'app-give-creature-sheet',
  imports: [
    CreatureChoiceList,
    FormsModule,
    MatButtonModule,
    MatFormFieldModule,
    MatIconModule,
    MatInputModule,
    SheetFrame,
  ],
  templateUrl: './give-creature-sheet.html',
  styleUrl: './give-creature-sheet.scss',
})
export class GiveCreatureSheet {
  private readonly client = inject(CreaturesClient);
  private readonly giveKey = new ActionKey();
  private readonly sheet = injectSheet<GiveCreatureData, GiveCreatureResult>();
  private readonly destroyRef = inject(DestroyRef);
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly types = TYPES;
  protected readonly crs = CRS;
  protected readonly max = CREATURE_NAME_MAX;
  protected readonly nameCounter = nameCounter;

  protected readonly query = signal('');
  protected readonly type = signal('');
  protected readonly maxCr = signal('');
  protected readonly found = signal<readonly CreatureSummary[] | null>(null);
  protected readonly total = signal(0);
  /** How many creatures the book has in all (the first, unfiltered search). */
  protected readonly catalog = signal(0);
  protected readonly picked = signal<CreatureSummary | null>(null);
  protected readonly block = signal<Creature | null>(null);
  protected readonly name = signal('');
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');
  private timer: ReturnType<typeof setTimeout> | null = null;
  private seq = 0;

  protected readonly rows = computed<readonly ChoiceRow[]>(() =>
    (this.found() ?? []).map((s) => ({
      key: s.key,
      title: s.namePt,
      alias: s.name !== s.namePt ? s.name : undefined,
      // The picked row also says its armor class and hit points, from its stat block (the catalog row has neither).
      subtitle:
        this.picked()?.key === s.key && this.block()
          ? summarySubtitle(s, this.block() ?? undefined)
          : joinDots([s.sizePt, s.typePt, challengeText(s.challengeRating)]),
      art: true,
    })),
  );
  /** "2 de 334 · em ordem de nome": how many of the book's creatures pass the filters. */
  protected readonly count = computed(() => {
    const shown = this.found()?.length ?? 0;
    const text = `${formatInt(this.total())} de ${formatInt(this.catalog())} · em ordem de nome`;
    return shown < this.total() ? `${text} · mostrando as ${formatInt(shown)} primeiras` : text;
  });
  protected readonly goText = computed(() => {
    const p = this.picked();
    return p ? `Dar ${p.namePt} a ${this.data.characterName}` : 'Dar a criatura';
  });
  protected readonly note = computed(() => {
    const b = this.block();
    const hp = b ? ` com os PV do livro (${b.hitPoints})` : '';
    return `Vai para a ficha de ${this.data.characterName} e entra nos combates${hp}. Você corrige PV e condições depois.`;
  });
  /** Why the filled button cannot act yet, in a line near it. */
  protected readonly missing = computed(() => {
    if (!this.picked()) {
      return 'Escolha uma criatura da lista.';
    }
    return this.name().trim() === '' ? 'Dê um nome à criatura.' : '';
  });
  protected readonly ready = computed(
    () => !!this.picked() && this.name().trim() !== '' && !this.busy(),
  );

  constructor() {
    void this.search();
    // A typing pause pending when the dialog closes must not fire into a closed dialog.
    this.destroyRef.onDestroy(() => {
      if (this.timer) {
        clearTimeout(this.timer);
      }
    });
  }

  protected setQuery(v: string): void {
    this.query.set(v);
    this.later();
  }

  protected setType(v: string): void {
    this.type.set(v);
    void this.search();
  }

  protected setCr(v: string): void {
    this.maxCr.set(v);
    void this.search();
  }

  /** Typing waits a moment, so a fast typist asks once. */
  private later(): void {
    if (this.timer) {
      clearTimeout(this.timer);
    }
    this.timer = setTimeout(() => void this.search(), 250);
  }

  private async search(): Promise<void> {
    const seq = ++this.seq;
    try {
      const res = await this.client.search(this.data.campaignId, {
        query: this.query().trim(),
        type: this.type(),
        maxCr: this.maxCr(),
        pageSize: 100,
      });
      if (seq !== this.seq) {
        return;
      }
      this.found.set(res.creatures);
      this.total.set(res.total);
      this.catalog.update((n) => Math.max(n, res.total));
      // The pick stays only while it still passes the filters.
      const p = this.picked();
      if (p && !res.creatures.some((c) => c.key === p.key)) {
        this.picked.set(null);
        this.block.set(null);
      }
    } catch (err) {
      if (seq === this.seq) {
        this.found.set([]);
        this.error.set(creatureErrorMessage(err, 'read'));
      }
    }
  }

  protected pick(key: string): void {
    const s = this.found()?.find((c) => c.key === key) ?? null;
    this.picked.set(s);
    this.block.set(null);
    this.error.set('');
    if (!s) {
      return;
    }
    this.name.set(s.namePt);
    this.client.statBlock(this.data.campaignId, key).then(
      (b) => {
        if (this.picked()?.key === key) {
          this.block.set(b);
        }
      },
      () => undefined,
    );
  }

  protected async give(): Promise<void> {
    const p = this.picked();
    if (!p || !this.ready()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      // A retry of the same gift (a lost answer, a second tap) sends the same key and gives one creature.
      const name = this.name().trim();
      const made = await this.client.give(
        this.data.campaignId,
        this.data.characterId,
        p.key,
        name,
        this.giveKey.keyFor([p.key, name]),
      );
      this.sheet.close({ name: made?.name ?? this.name().trim() });
    } catch (err) {
      this.error.set(creatureErrorMessage(err, 'give'));
    } finally {
      this.busy.set(false);
    }
  }

  protected close(): void {
    this.sheet.close();
  }
}
