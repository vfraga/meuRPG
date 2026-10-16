import { Component, OnInit, computed, inject, input, signal } from '@angular/core';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { RouterLink } from '@angular/router';

import type { CampaignDocument } from '../../../../gen/meurpg/campaigns/v1/campaign_document_pb';
import { describeConnectError } from '../../../core/connect/connect-errors';
import { DocumentClient } from '../../../shared/campaign-document/document-clients';
import { whenText } from '../../../shared/campaign-document/document-format';

type PanelState =
  | { status: 'loading' }
  | { status: 'error'; message: string }
  | { status: 'ready'; doc: CampaignDocument };

/**
 * The master's "Documento da campanha" panel on `/campaigns/:id` (E5-09,
 * MR-018): "Mirathel — preparação", "Editado ontem às 22:10. Só você vê
 * este documento." and "Abrir documento". `CampaignDetail` renders it for
 * the master only: the server refuses the document to a player.
 */
@Component({
  selector: 'app-document-panel',
  imports: [MatButtonModule, MatIconModule, RouterLink],
  templateUrl: './document-panel.html',
  styleUrl: './document-panel.scss',
})
export class DocumentPanel implements OnInit {
  private readonly documents = inject(DocumentClient);

  readonly campaignId = input.required<string>();
  readonly campaignName = input('');

  protected readonly state = signal<PanelState>({ status: 'loading' });
  protected readonly title = computed(() =>
    this.campaignName() ? `${this.campaignName()} — preparação` : 'Documento da campanha',
  );
  protected readonly line = computed(() => {
    const s = this.state();
    if (s.status !== 'ready') {
      return '';
    }
    const when = s.doc.updatedAt
      ? `Editado ${whenText(timestampDate(s.doc.updatedAt))}.`
      : 'Ainda sem texto.';
    return `${when} Só você vê este documento.`;
  });

  ngOnInit(): void {
    this.documents.get(this.campaignId()).then(
      (doc) => this.state.set({ status: 'ready', doc }),
      (err: unknown) => this.state.set({ status: 'error', message: describeConnectError(err, {}) }),
    );
  }
}
