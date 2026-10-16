import { Component, inject, signal } from '@angular/core';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { MatSelectModule } from '@angular/material/select';
import { Router, RouterLink } from '@angular/router';
import { Code } from '@connectrpc/connect';

import { Campaign, XpMode } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CampaignsService } from '../../core/campaigns/campaigns.service';
import { describeConnectError } from '../../core/connect/connect-errors';
import { ActionKey } from '../../core/connect/idempotency';
import { LivePill } from '../../shared/live-pill/live-pill';
import { OpenSessions } from '../../core/play/open-sessions';
import { creationRefusalText, roleTag, xpModeSentence } from '../../core/campaigns/campaign-copy';

type ListState =
  | { status: 'loading' }
  | { status: 'ready'; campaigns: Campaign[] }
  | { status: 'error'; message: string };

type CreateState = { status: 'idle' } | { status: 'saving' } | { status: 'error'; message: string };

/**
 * "Minhas campanhas" (guarded by authGuard — see app.routes.ts): MR-001's
 * "Criar campanha" form, then the list, one row per campaign (name, XP
 * mode, and the caller's role as a tag; "Pendente" while the master has
 * not approved them, MR-024), with an empty state that points to both ways
 * in: creating one, or an invite link. A campaign with an open session also
 * gets the "Sessão ao vivo" tag (RN-06), from the same light poll as the
 * app bar's "Ao vivo" link (`OpenSessions`).
 */
@Component({
  selector: 'app-campaigns',
  imports: [
    LivePill,
    MatButtonModule,
    MatFormFieldModule,
    MatIconModule,
    MatInputModule,
    MatProgressSpinnerModule,
    MatSelectModule,
    ReactiveFormsModule,
    RouterLink,
  ],
  templateUrl: './campaigns.html',
  styleUrl: './campaigns.scss',
})
export class Campaigns {
  private readonly campaigns = inject(CampaignsService);
  private readonly router = inject(Router);
  private readonly fb = inject(FormBuilder);
  private readonly createKey = new ActionKey();

  /** Campaigns with an open session, for the "Sessão ao vivo" tag. */
  protected readonly liveCampaignIds = inject(OpenSessions).liveCampaignIds;

  protected readonly XpMode = XpMode;
  protected readonly roleTag = roleTag;
  protected readonly xpModeSentence = xpModeSentence;

  protected readonly state = signal<ListState>({ status: 'loading' });
  protected readonly createState = signal<CreateState>({ status: 'idle' });

  protected readonly form = this.fb.nonNullable.group({
    name: ['', [Validators.required, Validators.maxLength(80)]],
    xpMode: this.fb.control<XpMode | null>(null, Validators.required),
  });

  constructor() {
    this.load();
  }

  private load(): void {
    this.campaigns.listMyCampaigns().then(
      (res) => this.state.set({ status: 'ready', campaigns: res.campaigns }),
      (err: unknown) => {
        this.state.set({
          status: 'error',
          message: describeConnectError(err, {}),
        });
      },
    );
  }

  protected async submit(): Promise<void> {
    if (this.form.invalid) {
      this.form.markAllAsTouched();
      return;
    }
    const { name, xpMode } = this.form.getRawValue();
    this.createState.set({ status: 'saving' });
    try {
      // One key per create: a retry of the same form sends the same key, so a lost answer or a second tap
      // makes one campaign.
      const res = await this.campaigns.createCampaign(
        name.trim(),
        xpMode!,
        this.createKey.keyFor([name.trim(), xpMode]),
      );
      this.createKey.renew();
      this.createState.set({ status: 'idle' });
      const id = res.campaign?.id;
      if (id) {
        await this.router.navigate(['/campaigns', id]);
      }
    } catch (err) {
      this.createState.set({
        status: 'error',
        message:
          creationRefusalText(err) ??
          describeConnectError(err, {
            [Code.InvalidArgument]: 'O nome da campanha precisa ter de 1 a 80 caracteres.',
          }),
      });
    }
  }
}
