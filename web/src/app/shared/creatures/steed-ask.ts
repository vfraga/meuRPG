import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  inject,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { CharacterCreature } from '../../../gen/meurpg/characters/v1/characters_pb';
import { creatureErrorMessage } from '../../core/creatures/creature-errors';
import { CreaturesClient } from '../../core/creatures/creatures-client';
import { focusWithRing } from '../../core/creatures/focus-ring';
import { type SteedGoing, steedAskText, steedRefusal } from '../../core/creatures/steed-card';

let nextId = 0;

/**
 * The two questions a bonded steed's card asks in place (#588, boards 588-24 and 588-25), never in a dialog: "Dispensar
 * Estrela?" (the steed goes, the bond stays) and "Libertar Estrela?" (the bond ends). An `alertdialog` like any dismissal:
 * "Voltar" first and with the focus, so a stray Enter sends nothing away; the confirm carries `data-confirm`. Both call
 * `DismissCreature`, with `dismissal` DISMISS or RELEASE.
 *
 * In a combat that holds the steed the owner's player spends the action of the turn: the chip "● Gasta a sua ação" says so
 * (`spendsAction`), and a refusal for the turn or the action goes to the card (`refused`), which shows it under its
 * actions. Any other error stays here, in words, with the question open.
 */
@Component({
  selector: 'app-steed-ask',
  imports: [MatButtonModule, MatIconModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @let c = creature();
    <div class="ask" role="alertdialog" [attr.aria-labelledby]="id + '-t'" [attr.aria-describedby]="id + '-d'">
      <p class="ask__t" [id]="id + '-t'"><mat-icon aria-hidden="true">warning</mat-icon><span>{{ verb() }} {{ c.name }}?</span></p>
      <p class="ask__d" [id]="id + '-d'">{{ text() }}</p>
      @if (spendsAction()) {
        <p class="ask__chip"><span aria-hidden="true">●</span> Gasta a sua ação</p>
      }
      @if (error()) {
        <p class="err" role="alert"><mat-icon aria-hidden="true">error</mat-icon>{{ error() }}</p>
      }
      <button #first type="button" matButton="outlined" class="big" [disabled]="busy()" (click)="closed.emit(false)">Voltar</button>
      <button type="button" matButton="outlined" class="big big--danger" data-confirm [disabled]="busy()" (click)="confirm()"><span class="lbl">{{ verb() }}&nbsp;<span class="nm">{{ c.name }}</span></span></button>
    </div>
  `,
  styleUrl: './creature-edit.scss',
})
export class SteedAsk {
  private readonly client = inject(CreaturesClient);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);

  readonly campaignId = input.required<string>();
  readonly creature = input.required<CharacterCreature>();
  readonly mode = input.required<SteedGoing>();
  /** Whose steed it is, for the sentence ("da sua ficha" or "da ficha de Lia") and for a refusal ("Só na vez de Lia"). */
  readonly ownerView = input(true);
  readonly ownerName = input('');
  /** A combat holds the steed and the asker is its owner's player: the chip "Gasta a sua ação". */
  readonly spendsAction = input(false);
  /** Done: true when the steed went (the list is read again), false when the person backed out. */
  readonly closed = output<boolean>();
  /** The combat refused for the turn or the action: the card says why under its actions. */
  readonly refused = output<string>();

  protected readonly id = `steed-ask-${nextId++}`;
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  private readonly first = viewChild('first', { read: ElementRef<HTMLElement> });

  protected readonly verb = computed(() => (this.mode() === 'release' ? 'Libertar' : 'Dispensar'));
  protected readonly text = computed(() =>
    steedAskText(this.mode(), this.creature().name, this.ownerView() ? '' : this.ownerName()),
  );

  constructor() {
    afterNextRender(
      () => {
        // The whole question comes into view under the sticky bar, then the focus goes to "Voltar".
        this.host.nativeElement.scrollIntoView?.({ block: 'nearest' });
        focusWithRing(this.first()?.nativeElement);
      },
      { injector: inject(Injector) },
    );
  }

  protected async confirm(): Promise<void> {
    if (this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      await this.client.dismiss(this.campaignId(), this.creature().id, this.mode());
      this.closed.emit(true);
    } catch (err) {
      const refusal = steedRefusal(err, this.ownerName());
      if (refusal) {
        this.refused.emit(refusal);
      } else {
        this.error.set(creatureErrorMessage(err, 'dismiss'));
      }
    } finally {
      this.busy.set(false);
    }
  }
}
