import {
  ChangeDetectionStrategy,
  Component,
  OnInit,
  computed,
  inject,
  input,
  signal,
} from '@angular/core';
import { MatIconModule } from '@angular/material/icon';
import { RouterLink } from '@angular/router';

import type { TableEntry } from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { describeConnectError } from '../../../core/connect/connect-errors';
import { TableContentClient } from '../../../core/content/content-client';
import { CONTENT_NAV, countOfNav } from '../../../core/content/content-kinds';
import { PanelAction } from '../panel-action/panel-action';
import { PanelHead } from '../panel-head/panel-head';

type SummaryState =
  | { status: 'loading' }
  | { status: 'error'; message: string }
  | { status: 'ready'; entries: readonly TableEntry[]; leftOut: number };

/**
 * "2 classes, 1 raça, 1 antecedente e 5 magias da mesa": what the table made, by kind, leaving out
 * the kinds with none and the archived entries. Empty when there is nothing.
 */
export function contentCounts(entries: readonly TableEntry[]): string {
  const live = entries.filter((e) => !e.archived);
  const parts = CONTENT_NAV.map((nav) => ({ nav, n: countOfNav(live, nav) }))
    .filter((p) => p.n > 0)
    .map((p) => `${p.n} ${p.n === 1 ? p.nav.singular : p.nav.plural.toLowerCase()}`);
  if (parts.length === 0) {
    return '';
  }
  const list = new Intl.ListFormat('pt-BR', { type: 'conjunction' }).format(parts);
  return `${list} da mesa.`;
}

/** "3 entradas do conteúdo da mesa estão fora da campanha.": what the master reads on the campaign page when the rules refuse some. */
export function leftOutSentence(count: number): string {
  return count === 1
    ? '1 entrada do conteúdo da mesa está fora da campanha.'
    : `${count} entradas do conteúdo da mesa estão fora da campanha.`;
}

/**
 * "Conteúdo da mesa" on the master's "Regras" tab (board C6-03): how many classes, races,
 * backgrounds and spells the table made, and "Abrir o conteúdo". It reads the entries itself
 * (`ListTableEntries`).
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-content-summary',
  imports: [MatIconModule, PanelAction, PanelHead, RouterLink],
  templateUrl: './content-summary.html',
  styleUrl: './content-summary.scss',
})
export class ContentSummary implements OnInit {
  private readonly client = inject(TableContentClient);

  readonly campaignId = input.required<string>();

  protected readonly state = signal<SummaryState>({ status: 'loading' });
  /** The entries the rules refuse, left out of the campaign: the server tells the master only. */
  protected readonly leftOut = computed(() => {
    const s = this.state();
    return s.status === 'ready' ? leftOutSentence(s.leftOut) : '';
  });
  protected readonly counts = computed(() => {
    const s = this.state();
    return s.status === 'ready' ? contentCounts(s.entries) : '';
  });

  ngOnInit(): void {
    this.client.list(this.campaignId()).then(
      (list) =>
        this.state.set({ status: 'ready', entries: list.entries, leftOut: list.leftOut.length }),
      (err: unknown) => this.state.set({ status: 'error', message: describeConnectError(err, {}) }),
    );
  }
}
