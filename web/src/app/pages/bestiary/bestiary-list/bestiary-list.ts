import { Component, DestroyRef, ElementRef, computed, inject, signal } from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import {
  ActivatedRoute,
  NavigationCancel,
  NavigationError,
  NavigationStart,
  Router,
  RouterLink,
} from '@angular/router';

import type { CreatureSummary } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { type BestiaryAccess, BestiaryAccessCheck } from '../../../core/creatures/bestiary-access';
import { bestiaryErrorMessage } from '../../../core/creatures/bestiary-errors';
import { acAndHp, creatureSlug, typeAndSize } from '../../../core/creatures/bestiary-format';
import {
  CHALLENGE_RANGES,
  CHALLENGE_RATINGS,
  CREATURE_SIZES,
  CREATURE_TYPES,
  challengeBounds,
} from '../../../core/creatures/creature-types';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { formatInt, joinDots } from '../../../core/format/text';
import {
  PutMonstersSheet,
  type PutMonstersData,
  type PutMonstersResult,
} from '../../../shared/monsters/put-sheet/put-sheet';
import { openSheet } from '../../../shared/sheet/sheet-host';
import { BestiaryRow, type BestiaryRowData } from './bestiary-row';

type ListState = { status: 'loading' } | { status: 'error'; message: string } | { status: 'ready' };

/** The query parameters that keep the search when the master opens a stat block and comes back. */
const PARAMS = { query: 'q', type: 'type', size: 'size', cr: 'cr' } as const;

/**
 * "/campaigns/:id/bestiary" (MR-042, E10-08, states 1, 2 and 7): the master's list of the SRD's
 * 334 creatures. Four filters (name in Portuguese or the SRD's English, type, size and challenge
 * rating) ask the server, which answers the whole bestiary in one page (`page_size` 400): a row
 * says the Portuguese name, the SRD's name in small type, the type and size, the ND, and the
 * armor class and average hit points. A row opens the stat block. The filters live in the URL,
 * so "Voltar ao Bestiário" brings the same search back.
 *
 * Only the master gets this page: the SRD is public and the server answers any member, so the
 * page asks for the campaign's role and tells a player so (the campaign page has no entry for
 * them either).
 */
@Component({
  selector: 'app-bestiary-list',
  imports: [BestiaryRow, MatButtonModule, MatIconModule, MatProgressSpinnerModule, RouterLink],
  templateUrl: './bestiary-list.html',
  styleUrl: './bestiary-list.scss',
})
export class BestiaryList {
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  private readonly client = inject(CreaturesClient);
  private readonly accessCheck = inject(BestiaryAccessCheck);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);

  protected readonly campaignId = this.route.snapshot.paramMap.get('id') ?? '';
  protected readonly access = signal<BestiaryAccess | { status: 'loading' }>({ status: 'loading' });
  protected readonly state = signal<ListState>({ status: 'loading' });
  /** The monsters just put in the combat, for the confirmation above the list. */
  protected readonly put = signal<PutMonstersResult | null>(null);

  protected readonly query = signal(this.route.snapshot.queryParamMap.get(PARAMS.query) ?? '');
  protected readonly type = signal(this.route.snapshot.queryParamMap.get(PARAMS.type) ?? '');
  protected readonly size = signal(this.route.snapshot.queryParamMap.get(PARAMS.size) ?? '');
  protected readonly cr = signal(this.route.snapshot.queryParamMap.get(PARAMS.cr) ?? '');

  protected readonly found = signal<readonly CreatureSummary[]>([]);
  /** How many creatures pass the filters (the server's `total`). */
  protected readonly total = signal(0);
  /** How many the book has in all, once known: from an unfiltered answer, or asked for once when the link arrives with a search. */
  protected readonly catalog = signal<number | null>(null);
  /** The book's size could not be asked: the count then says only how many matched. */
  private readonly catalogFailed = signal(false);

  protected readonly types = CREATURE_TYPES;
  protected readonly sizes = CREATURE_SIZES;
  protected readonly ranges = CHALLENGE_RANGES;
  protected readonly ratings = CHALLENGE_RATINGS;

  protected readonly filtered = computed(
    () =>
      this.query().trim() !== '' || this.type() !== '' || this.size() !== '' || this.cr() !== '',
  );
  /** "5 de 334 criaturas"; nothing until the book's size is known (never "5 de 5"), and just "5 criaturas" if it cannot be. */
  protected readonly count = computed(() => {
    const all = this.catalog();
    if (all !== null) {
      return `${formatInt(this.total())} de ${formatInt(all)} criaturas`;
    }
    return this.catalogFailed() ? `${formatInt(this.total())} criaturas` : '';
  });
  /** The search in the URL's words, for the rows' links: opening a row keeps what is typed, even before the pause ends. */
  protected readonly rowParams = computed<Record<string, string>>(() => {
    const out: Record<string, string> = {};
    const entries: [string, string][] = [
      [PARAMS.query, this.query().trim()],
      [PARAMS.type, this.type()],
      [PARAMS.size, this.size()],
      [PARAMS.cr, this.cr()],
    ];
    for (const [key, value] of entries) {
      if (value !== '') {
        out[key] = value;
      }
    }
    return out;
  });
  /** "Nenhuma criatura com “wyrm”." (or, with no text typed, the filters are what matched nothing). */
  protected readonly emptyTitle = computed(() => {
    const q = this.query().trim();
    return q ? `Nenhuma criatura com “${q}”.` : 'Nenhuma criatura passa nesses filtros.';
  });
  protected readonly formatCatalog = () => `${formatInt(this.catalog() ?? 0)} criaturas`;
  protected readonly rows = computed(() =>
    this.found().map((s) => ({
      key: s.key,
      type: s.type,
      slug: creatureSlug(s.key),
      namePt: s.namePt,
      name: s.name,
      kind: typeAndSize(s),
      nd: `ND ${s.challengeRating}`,
      stats: acAndHp(s),
      meta: joinDots([typeAndSize(s), `ND ${s.challengeRating}`, acAndHp(s)]),
    })),
  );

  private timer: ReturnType<typeof setTimeout> | null = null;
  private seq = 0;
  /** A navigation away from the list (a row opened) is under way: the list must not write its URL over it. */
  private leaving = false;

  constructor() {
    const destroyRef = inject(DestroyRef);
    destroyRef.onDestroy(() => {
      if (this.timer) {
        clearTimeout(this.timer);
      }
    });
    // The list stays on screen while the next page's chunk loads, so the pending typing pause could still fire
    // and its URL write would win over the row's navigation. Leaving the list cancels the pause and the write.
    const listPath = `/campaigns/${this.campaignId}/bestiary`;
    const sub = this.router.events.subscribe((event) => {
      if (event instanceof NavigationStart && event.url.split('?')[0] !== listPath) {
        this.leaving = true;
        if (this.timer) {
          clearTimeout(this.timer);
          this.timer = null;
        }
      } else if (event instanceof NavigationCancel || event instanceof NavigationError) {
        this.leaving = false; // the list is still the page: its URL follows the search again
      }
    });
    destroyRef.onDestroy(() => sub.unsubscribe());
    void this.start();
  }

  protected async start(): Promise<void> {
    this.access.set({ status: 'loading' });
    // The first search does not wait for the role: both go at once (a player's answer is never drawn).
    void this.search();
    this.access.set(await this.accessCheck.check(this.campaignId));
  }

  /** "Pôr no combate" on a row: the sheet opens, and what went in is announced above the list. */
  protected openPut(row: BestiaryRowData): void {
    const creature = this.found().find((c) => c.key === row.key);
    if (!creature) {
      return;
    }
    openSheet<PutMonstersSheet, PutMonstersData, PutMonstersResult>(
      this.dialog,
      this.bottomSheet,
      PutMonstersSheet,
      {
        data: { campaignId: this.campaignId, creature },
        ariaLabel: 'Pôr no combate',
        labelledBy: 'put-t',
        width: '600px',
        tall: true,
      },
    ).subscribe((result) => {
      if (result) {
        this.put.set(result);
      }
    });
  }

  protected setQuery(value: string): void {
    this.query.set(value);
    // Typing waits a moment, so a fast typist asks once.
    if (this.timer) {
      clearTimeout(this.timer);
    }
    this.timer = setTimeout(() => void this.search(), 250);
  }

  protected setType(value: string): void {
    this.type.set(value);
    void this.search();
  }

  protected setSize(value: string): void {
    this.size.set(value);
    void this.search();
  }

  protected setCr(value: string): void {
    this.cr.set(value);
    void this.search();
  }

  /** "Limpar filtros" and "Limpar a busca": back to the whole bestiary, or just without the typed name. */
  protected clear(onlyName = false): void {
    this.query.set('');
    if (!onlyName) {
      this.type.set('');
      this.size.set('');
      this.cr.set('');
    }
    void this.search();
    // The button that was pressed is gone now: the focus goes to the search field, where the person starts over.
    this.host.nativeElement.querySelector<HTMLInputElement>('input[type=search]')?.focus();
  }

  protected async search(): Promise<void> {
    if (this.timer) {
      clearTimeout(this.timer);
      this.timer = null;
    }
    const seq = ++this.seq;
    this.syncUrl();
    const { minCr, maxCr } = challengeBounds(this.cr());
    try {
      const res = await this.client.search(this.campaignId, {
        query: this.query().trim(),
        type: this.type(),
        size: CREATURE_SIZES.find((s) => s.value === this.size())?.size,
        minCr,
        maxCr,
        pageSize: 400,
      });
      if (seq !== this.seq) {
        return;
      }
      this.found.set(res.creatures);
      this.total.set(res.total);
      this.state.set({ status: 'ready' });
      if (!this.filtered()) {
        this.catalog.set(res.total);
        this.catalogFailed.set(false);
      } else if (this.catalog() === null) {
        // Arrived with a search in the link: the book's size is one more (cheap) question.
        void this.learnCatalog();
      }
    } catch (err) {
      if (seq === this.seq) {
        this.state.set({ status: 'error', message: bestiaryErrorMessage(err, 'list') });
      }
    }
  }

  private async learnCatalog(): Promise<void> {
    try {
      const all = await this.client.search(this.campaignId, { pageSize: 1 });
      this.catalog.set(all.total);
    } catch {
      // The count line then says only how many matched.
      this.catalogFailed.set(true);
    }
  }

  /** The search goes into the URL (replacing the entry, so Back leaves the bestiary, not the last letter typed). */
  private syncUrl(): void {
    if (this.leaving) {
      return;
    }
    void this.router.navigate([], {
      relativeTo: this.route,
      replaceUrl: true,
      queryParams: {
        [PARAMS.query]: this.query().trim() || null,
        [PARAMS.type]: this.type() || null,
        [PARAMS.size]: this.size() || null,
        [PARAMS.cr]: this.cr() || null,
      },
      queryParamsHandling: 'merge',
    });
  }
}
