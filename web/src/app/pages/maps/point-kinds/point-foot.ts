import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  inject,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';

import { focusWithRing } from '../../../core/creatures/focus-ring';
import { PairFoot } from '../../../shared/pair-foot/pair-foot';
import { MapAsk } from '../../../shared/map-ask/map-ask';

/**
 * The foot of the new point panels (E9-02): "Salvar ponto", the screen's one filled button, and "Apagar ponto" beside it
 * (`app-pair-foot`: two buttons of the same size, stacked when the column is narrow). Deleting asks in place with the editor's
 * one question (`app-map-ask`: "Apagar Fosso escondido?", "Voltar" first, the filled "Apagar ponto"). A point that cannot be
 * deleted (a treasure that was found or turned into XP) shows the dashed "can't act" button and the reason in a line.
 * Presentational: the page runs the calls.
 */
@Component({
  selector: 'app-point-foot',
  imports: [MapAsk, PairFoot],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (confirming()) {
      <app-map-ask [title]="'Apagar ' + name() + '?'" confirmLabel="Apagar ponto" (cancel)="cancelAsk()" (confirm)="remove.emit()">
        <p class="pf__text">Não dá para desfazer.</p>
      </app-map-ask>
    } @else {
      <div #pair>
        <app-pair-foot
          confirmLabel="Salvar ponto"
          [ready]="!saving() && dirty()"
          [confirmFirst]="true"
          cancelLabel="Apagar ponto"
          cancelIcon="delete"
          [cancelDanger]="!blocked()"
          [cancelOff]="!!blocked()"
          [cancelDescribedBy]="blocked() ? id + '-why' : ''"
          (confirm)="save.emit()"
          (cancel)="!blocked() && ask()"
        />
      </div>
      @if (blocked(); as why) {
        <p class="pf__why" [id]="id + '-why'">{{ why }}</p>
      }
    }
  `,
  styles: `
    :host {
      display: block;
      padding-top: var(--mr-space-3);
      border-top: 1px solid var(--mr-rule);
    }

    .pf__text {
      margin: 0;
    }

    .pf__why {
      margin: var(--mr-space-2) 0 0;
      font-size: 14px;
      line-height: 19px;
      color: var(--mr-ink-muted);
    }
  `,
})
export class PointFoot {
  private readonly injector = inject(Injector);
  private static next = 0;
  protected readonly id = `pf-${PointFoot.next++}`;

  readonly name = input.required<string>();
  readonly saving = input(false);
  readonly dirty = input(false);
  /** Why the point cannot be deleted, or empty. */
  readonly blocked = input('');

  readonly save = output<void>();
  readonly remove = output<void>();

  protected readonly confirming = signal(false);
  private readonly pair = viewChild('pair', { read: ElementRef<HTMLElement> });

  protected ask(): void {
    this.confirming.set(true);
  }

  protected cancelAsk(): void {
    this.confirming.set(false);
    afterNextRender(
      () =>
        focusWithRing(
          this.pair()?.nativeElement.querySelector('.pf__cancel') as HTMLElement | null | undefined,
        ),
      { injector: this.injector },
    );
  }
}
