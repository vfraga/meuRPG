import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  computed,
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
import {
  type LegendaryResistanceCard,
  canConfirmResistance,
  resistanceIds,
} from '../../../../core/combat/legendary-resistance-card';
import { ActionKey } from '../../../../core/connect/idempotency';
import { type OfferFailure, type Sending, focusInitial, offerFailure } from './offer-answer';

let nextId = 0;

/**
 * The master's Legendary Resistance window (#557, boards 03, 08): one row per creature whose saving throw failed, each a
 * checkbox that starts unticked ("a magia espera; os dados já foram rolados"). "Deixar como rolou" comes first and sends
 * no creature; "Usar nas marcadas" sends exactly the ticked ones and stays off while none is ticked (it would be the
 * same as the first button: `canConfirmResistance`). A single failing creature has no checkbox: "Usar Resistência Lendária"
 * sends that creature. The server spends one use per creature named. Only the master's
 * queue draws it; the players read "Esperando o mestre".
 */
@Component({
  selector: 'app-resistance-offer',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatButtonModule, MatIconModule],
  templateUrl: './resistance-offer.html',
  styleUrls: ['./offer.scss', './resistance-offer.scss'],
})
export class ResistanceOffer {
  private readonly api = inject(CombatClient);
  private readonly keys = new ActionKey();
  private readonly injector = inject(Injector);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);

  readonly card = input.required<LegendaryResistanceCard>();
  readonly encounter = input.required<Encounter>();
  readonly campaignId = input.required<string>();
  readonly state = input.required<CombatState>();
  /** `null` draws no pill and no buttons (a window that is not the master's; the queue never gives one to this card). */
  readonly badge = input<'Agora' | 'Depois' | null>('Agora');
  readonly waiting = input('');

  protected readonly uid = `lr-${nextId++}-`;
  protected readonly ticked = signal<ReadonlySet<string>>(new Set());
  protected readonly sending = signal<Sending>(null);
  protected readonly failure = signal<OfferFailure | null>(null);
  /** What the answer did, for the queue's live region: this card is gone from the screen right after it. */
  readonly announce = output<string>();
  /** The rows stay still while an answer is in the air and while "Tentar de novo" waits: the retry resends the same answer. */
  protected readonly locked = computed(() => !!this.sending() || !!this.failure()?.connection);
  protected readonly canConfirm = computed(() => canConfirmResistance(this.card(), this.ticked()));
  /** The creatures a retry sends again: the same list, so the same key. */
  private last: readonly string[] | null = null;

  protected toggle(id: string, on: boolean): void {
    this.ticked.update((t) => {
      const next = new Set(t);
      if (on) {
        next.add(id);
      } else {
        next.delete(id);
      }
      return next;
    });
  }

  /** "Deixar como rolou": no creature. */
  protected decline(): Promise<void> {
    return this.answer([]);
  }

  /** "Usar nas marcadas": the ticked creatures, in the window's order. */
  protected confirm(): Promise<void> {
    return this.canConfirm()
      ? this.answer(resistanceIds(this.card(), this.ticked()))
      : Promise.resolve();
  }

  protected retry(): Promise<void> {
    return this.last === null ? Promise.resolve() : this.send(this.last);
  }

  private answer(ids: readonly string[]): Promise<void> {
    this.last = ids;
    return this.send(ids);
  }

  private async send(ids: readonly string[]): Promise<void> {
    if (this.sending()) {
      return;
    }
    this.sending.set(ids.length > 0 ? 'use' : 'pass');
    // Retrying swaps the retry button for the answer buttons: focus goes to the first of them.
    if (this.failure()) {
      focusInitial(this.host.nativeElement, this.injector);
    }
    this.failure.set(null);
    try {
      const res = await this.api.attacks.answerLegendaryResistance(
        this.campaignId(),
        this.encounter().id,
        this.card().id,
        { creatureIds: ids },
        this.keys.keyFor({ window: this.card().id, ids }),
      );
      this.state().apply(res.encounter);
      this.announce.emit(
        ids.length > 0 ? 'Resistência Lendária usada.' : 'Os testes ficaram como rolados.',
      );
    } catch (err) {
      this.failure.set(offerFailure(err, 'responder à Resistência Lendária'));
      // The pressed button is swapped for the retry button (a lost connection): focus follows it.
      focusInitial(this.host.nativeElement, this.injector);
    } finally {
      this.sending.set(null);
    }
  }
}
