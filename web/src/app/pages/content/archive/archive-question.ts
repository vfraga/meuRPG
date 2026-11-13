import { Component, ElementRef, afterNextRender, input, output, viewChild } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import { PairFoot } from '../../../shared/pair-foot/pair-foot';
import { RelayEvents } from '../../../shared/relay-events/relay-events';

/**
 * "Arquivar Corujeiro?" in place (E10-01 state 8): the question opens in the editor's header, outlined in the accent, the focus
 * on its title and Esc closing it. The warning says what archiving does: nothing is deleted, and the sheets that use the entry
 * keep it. "Voltar" and "Arquivar Corujeiro" have the same width and the filled one is the only filled button on the page.
 */
@Component({
  selector: 'app-archive-question',
  imports: [RelayEvents, MatIconModule, PairFoot],
  template: `
    <section class="ask" aria-labelledby="ask-t" appRelayEvents (relayKeydown)="$event.key === 'Escape' && cancel.emit()">
      <h2 class="ask__title" id="ask-t" tabindex="-1" #title>{{ verb() }} {{ name() }}?</h2>
      <div class="mr-notice mr-notice--warning">
        <mat-icon aria-hidden="true">warning</mat-icon>
        @if (verb() === 'Remover') {
          <p>A entrada é arquivada, não apagada. As fichas que usam {{ name() }} continuam funcionando.</p>
        } @else {
          <p>As fichas que usam {{ name() }} continuam funcionando. A entrada só deixa de aparecer para fichas novas.</p>
        }
      </div>
      <p class="ask__using">{{ using() }}</p>
      <app-pair-foot cancelLabel="Voltar" [confirmLabel]="verb() + ' ' + name()" confirmIcon="archive" [busy]="busy()" (cancel)="cancel.emit()" (confirm)="confirm.emit()" />
    </section>
  `,
  styles: `
    :host {
      display: block;
    }

    .ask {
      display: flex;
      flex-direction: column;
      gap: var(--mr-space-3);
      box-sizing: border-box;
      max-width: 420px;
      padding: var(--mr-space-4);
      border: 2px solid var(--mr-accent);
      border-radius: var(--mr-radius-lg);
      background: var(--mr-surface);
      scroll-margin-top: 96px;
    }

    .ask__title {
      margin: 0;
      font-family: var(--mr-font-display);
      font-size: var(--mr-font-22);
      line-height: 28px;
      font-weight: 700;

      &:focus {
        outline: none;
      }
    }

    .ask__using {
      margin: 0;
      color: var(--mr-ink-muted);
    }
  `,
})
export class ArchiveQuestion {
  readonly name = input.required<string>();
  readonly using = input.required<string>();
  /** "Remover" on an entry the rules refuse (it is archived, nothing is deleted). */
  readonly verb = input<'Arquivar' | 'Remover'>('Arquivar');
  readonly busy = input(false);
  readonly cancel = output<void>();
  readonly confirm = output<void>();
  private readonly title = viewChild<ElementRef<HTMLElement>>('title');

  constructor() {
    afterNextRender(() => {
      const t = this.title()?.nativeElement;
      t?.scrollIntoView?.({ block: 'nearest' });
      t?.focus({ preventScroll: true });
    });
  }
}
