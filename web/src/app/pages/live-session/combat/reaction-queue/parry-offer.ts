import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  inject,
  input,
  output,
  signal,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { Encounter } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import type { CombatState } from '../../../../core/combat/combat-state';
import type { ParryCard } from '../../../../core/combat/parry-card';
import { ActionKey } from '../../../../core/connect/idempotency';
import { type OfferFailure, type Sending, focusInitial, offerFailure } from './offer-answer';

/**
 * The master's Parry window (#557, boards 01, 08, 10, 12, 13): "Aparar o golpe de Tor?". It exists only when the master
 * set the creature to ask; the server decides Parry otherwise. "Deixar passar" comes first. The answer carries its own
 * key, kept across a retry, so a lost answer sent again never answers twice. Only the master's queue draws it.
 */
@Component({
  selector: 'app-parry-offer',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatButtonModule, MatIconModule],
  templateUrl: './parry-offer.html',
  styleUrl: './offer.scss',
})
export class ParryOffer {
  private readonly api = inject(CombatClient);
  private readonly keys = new ActionKey();
  private readonly injector = inject(Injector);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);

  readonly card = input.required<ParryCard>();
  readonly encounter = input.required<Encounter>();
  readonly campaignId = input.required<string>();
  readonly state = input.required<CombatState>();
  /** "Agora" for the window to answer, "Depois" for the ones that wait. */
  /** `null` draws no pill and no buttons (a window that is not the master's; the queue never gives one to this card). */
  readonly badge = input<'Agora' | 'Depois' | null>('Agora');
  /** "Depois da janela acima.", or `''`. */
  readonly waiting = input('');

  protected readonly sending = signal<Sending>(null);
  protected readonly failure = signal<OfferFailure | null>(null);
  /** What the answer did, for the queue's live region: this card is gone from the screen right after it. */
  readonly announce = output<string>();
  /** The answer a retry sends again: the same value, so the same key. */
  private last: boolean | null = null;

  protected answer(use: boolean): Promise<void> {
    this.last = use;
    return this.send(use);
  }

  protected retry(): Promise<void> {
    return this.last === null ? Promise.resolve() : this.send(this.last);
  }

  private async send(use: boolean): Promise<void> {
    if (this.sending()) {
      return;
    }
    this.sending.set(use ? 'use' : 'pass');
    // Retrying swaps the retry button for the answer buttons: focus goes to the first of them.
    if (this.failure()) {
      focusInitial(this.host.nativeElement, this.injector);
    }
    this.failure.set(null);
    try {
      const res = await this.api.attacks.answerParry(
        this.campaignId(),
        this.encounter().id,
        this.card().id,
        use,
        this.keys.keyFor({ window: this.card().id, use }),
      );
      this.state().apply(res.encounter);
      this.announce.emit(use ? 'Golpe aparado.' : 'O golpe passou.');
    } catch (err) {
      this.failure.set(offerFailure(err, 'responder a reação'));
      // The pressed button is swapped for the retry button (a lost connection): focus follows it.
      focusInitial(this.host.nativeElement, this.injector);
    } finally {
      this.sending.set(null);
    }
  }
}
