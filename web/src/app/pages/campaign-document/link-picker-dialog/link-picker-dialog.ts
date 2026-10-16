import { Component, OnInit, computed, inject, input, output, signal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { describeConnectError } from '../../../core/connect/connect-errors';
import { DocDialog } from '../doc-dialog/doc-dialog';
import { DocumentLinks } from '../../../shared/campaign-document/document-clients';

/** One row of the picker. */
interface Row {
  readonly id: string;
  readonly name: string;
  readonly sub: string;
}

export interface PickedLink {
  readonly id: string;
  readonly name: string;
}

type State =
  | { status: 'loading' }
  | { status: 'ready'; rows: readonly Row[] }
  | { status: 'error'; message: string };

/**
 * "Link para mapa" and "Link para ficha" (the editor toolbar): lists the
 * campaign's maps (`ListMaps`) or characters (`ListCharacters`); choosing a
 * row hands its id and name to the editor, which writes
 * `[nome](map:<id>)` or `[nome](character:<id>)` at the cursor, so the master
 * never types an id.
 */
@Component({
  selector: 'app-link-picker-dialog',
  imports: [DocDialog, MatButtonModule, MatIconModule],
  templateUrl: './link-picker-dialog.html',
  styleUrl: './link-picker-dialog.scss',
})
export class LinkPickerDialog implements OnInit {
  private readonly links = inject(DocumentLinks);

  readonly kind = input.required<'map' | 'character'>();
  readonly campaignId = input.required<string>();
  readonly picked = output<PickedLink>();
  readonly closed = output<void>();

  protected readonly state = signal<State>({ status: 'loading' });
  protected readonly isMap = computed(() => this.kind() === 'map');

  ngOnInit(): void {
    this.load();
  }

  protected load(): void {
    this.state.set({ status: 'loading' });
    const rows =
      this.kind() === 'map'
        ? this.links.listMaps(this.campaignId()).then((maps) =>
            maps.map<Row>((m) => ({
              id: m.id,
              name: m.name,
              sub: m.revealed ? 'Revelado aos jogadores' : 'Escondido dos jogadores',
            })),
          )
        : this.links.listCharacters(this.campaignId()).then((list) =>
            list.map<Row>((c) => ({
              id: c.id,
              name: c.name,
              sub:
                [c.classSummary, c.raceName].filter((p) => p !== '').join(', ') || 'Ficha básica',
            })),
          );
    rows.then(
      (list) => this.state.set({ status: 'ready', rows: list }),
      (err: unknown) => this.state.set({ status: 'error', message: describeConnectError(err, {}) }),
    );
  }
}
