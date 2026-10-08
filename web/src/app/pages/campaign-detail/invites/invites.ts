import { Component, OnInit, inject, input, signal } from '@angular/core';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatCheckboxModule } from '@angular/material/checkbox';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { MatSelectModule } from '@angular/material/select';
import { Code } from '@connectrpc/connect';

import { Invite, InviteState } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CampaignsService } from '../../../core/campaigns/campaigns.service';
import { describeConnectError } from '../../../core/connect/connect-errors';
import {
  inviteApprovalLabel,
  inviteDateLabel,
  inviteStateTag,
  inviteUsesLabel,
} from './invite-copy';
import { CopyStatus, InviteReveal } from './invite-reveal';

type ListState =
  | { status: 'loading' }
  | { status: 'ready'; invites: Invite[] }
  | { status: 'error'; message: string };

type CreateState = { status: 'idle' } | { status: 'saving' } | { status: 'error'; message: string };

const MASTER_ONLY_MESSAGES = {
  [Code.PermissionDenied]: 'Só o mestre da campanha pode gerenciar convites.',
  [Code.Unavailable]: 'Não foi possível falar com o servidor agora. Tente de novo em instantes.',
};

/**
 * The master's "Convites" section on `/campaigns/:id` (MR-002): create an
 * invite, see its link exactly once, and list/revoke existing invites.
 * Only rendered by `CampaignDetail` when `my_role` is master.
 *
 * One panel: the invites first (uses, a date, approval and a state tag per
 * row, "Revogar" on an active one), then "Novo convite" at the end, where
 * the one-time link appears (`InviteReveal`) right above the form that made
 * it.
 *
 * "Exigir aprovação do mestre" (RN-15, MR-024) makes whoever accepts the
 * invite a pending member: they create their character right away, and
 * join the campaign only when the master approves it.
 */
@Component({
  selector: 'app-campaign-invites',
  imports: [
    InviteReveal,
    MatButtonModule,
    MatCheckboxModule,
    MatFormFieldModule,
    MatIconModule,
    MatInputModule,
    MatSelectModule,
    ReactiveFormsModule,
  ],
  templateUrl: './invites.html',
  styleUrl: './invites.scss',
})
export class CampaignInvites implements OnInit {
  private readonly campaigns = inject(CampaignsService);
  private readonly fb = inject(FormBuilder);

  readonly campaignId = input.required<string>();

  protected readonly InviteState = InviteState;
  protected readonly inviteUsesLabel = inviteUsesLabel;
  protected readonly inviteDateLabel = inviteDateLabel;
  protected readonly inviteApprovalLabel = inviteApprovalLabel;
  protected readonly inviteStateTag = inviteStateTag;

  protected readonly listState = signal<ListState>({ status: 'loading' });
  /** Why a "Revogar" failed; the list stays on screen. */
  protected readonly revokeError = signal('');
  /** The invite whose "Revogar" is running. */
  protected readonly revoking = signal<string | null>(null);
  protected readonly createState = signal<CreateState>({ status: 'idle' });

  /** The just-created invite's link, shown exactly once (CreateInvite's doc
   * comment: "this is the only time the server ever returns it"). Cleared
   * when the master dismisses the banner or creates another invite. */
  protected readonly revealedLink = signal<string | null>(null);
  /** Whether the revealed link's invite requires approval, so the banner
   * can tell the master what whoever uses it will see. */
  protected readonly revealedRequiresApproval = signal(false);
  protected readonly copyStatus = signal<CopyStatus>('idle');

  protected readonly form = this.fb.nonNullable.group({
    maxUses: [1, [Validators.required, Validators.min(1), Validators.max(20)]],
    validityDays: [7, Validators.required],
    requiresApproval: [false],
  });

  ngOnInit(): void {
    // Not the constructor: `TestBed.createComponent` + `setInput()` (the
    // standard way to drive a required signal input in a spec) only
    // applies the input value before the first change-detection pass, and
    // ngOnInit is guaranteed to run after that — reading `campaignId()`
    // any earlier would throw for a value that has not been set yet.
    this.load();
  }

  private load(): void {
    this.listState.set({ status: 'loading' });
    this.campaigns.listInvites(this.campaignId()).then(
      (res) => this.listState.set({ status: 'ready', invites: res.invites }),
      (err: unknown) => {
        this.listState.set({
          status: 'error',
          message: describeConnectError(err, MASTER_ONLY_MESSAGES),
        });
      },
    );
  }

  protected async createInvite(): Promise<void> {
    if (this.form.invalid) {
      this.form.markAllAsTouched();
      return;
    }
    const { maxUses, validityDays, requiresApproval } = this.form.getRawValue();
    this.createState.set({ status: 'saving' });
    this.revealedLink.set(null);
    this.copyStatus.set('idle');
    try {
      const res = await this.campaigns.createInvite(
        this.campaignId(),
        maxUses,
        validityDays,
        requiresApproval,
      );
      this.createState.set({ status: 'idle' });
      if (res.invite) {
        const current = this.listState();
        const invites = current.status === 'ready' ? current.invites : [];
        this.listState.set({ status: 'ready', invites: [res.invite, ...invites] });
      }
      this.revealedLink.set(`${window.location.origin}/invite#t=${res.token}`);
      this.revealedRequiresApproval.set(requiresApproval);
      this.form.reset({ maxUses: 1, validityDays: 7, requiresApproval: false });
    } catch (err) {
      this.createState.set({
        status: 'error',
        message: describeConnectError(err, {
          ...MASTER_ONLY_MESSAGES,
          [Code.InvalidArgument]: 'Confira o número de usos (1 a 20) e a validade escolhida.',
        }),
      });
    }
  }

  protected dismissReveal(): void {
    this.revealedLink.set(null);
    this.copyStatus.set('idle');
  }

  protected async copyLink(link: string): Promise<void> {
    try {
      await navigator.clipboard.writeText(link);
      this.copyStatus.set('copied');
    } catch {
      this.copyStatus.set('error');
    }
  }

  protected async revoke(invite: Invite): Promise<void> {
    if (this.revoking() !== null) {
      return;
    }
    this.revoking.set(invite.id);
    this.revokeError.set('');
    try {
      const res = await this.campaigns.revokeInvite(this.campaignId(), invite.id);
      const current = this.listState();
      if (current.status === 'ready' && res.invite) {
        this.listState.set({
          status: 'ready',
          invites: current.invites.map((i) => (i.id === res.invite!.id ? res.invite! : i)),
        });
      }
    } catch (err) {
      // The list stays: only this action failed.
      this.revokeError.set(describeConnectError(err, MASTER_ONLY_MESSAGES));
    } finally {
      this.revoking.set(null);
    }
  }
}
