import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  effect,
  inject,
  input,
  output,
  untracked,
} from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import { MapAsk } from '../../../shared/map-ask/map-ask';

/**
 * "Porta" (E10-05 7, RN-26): the side panel while the door tool is the chosen one. It says what a tap does (puts the chosen kind where
 * a wall has floor on both sides, changes the kind of a door, and "Tirar a porta" closes the gap again), shows why a tap did nothing
 * (`refusal`, `role="alert"`), and what players see (a locked door as a closed one, a secret door as a wall). When the tap would put a
 * closed, locked or secret door where someone stands (`standing`), the panel asks first, in place: the person there would be out of the
 * players' sight until the door opens. Presentational: the editor owns the painting.
 */
@Component({
  selector: 'app-door-panel',
  imports: [MapAsk, MatIconModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <section class="mr-panel dp" aria-labelledby="dp-title">
      @if (standing()) {
        <app-map-ask [title]="erasing() ? 'Tirar a porta onde há alguém?' : 'Pôr a porta onde há alguém?'" [confirmLabel]="erasing() ? 'Tirar a porta' : 'Pôr a porta'" (cancel)="cancel.emit()" (confirm)="confirm.emit()">
          <div class="mr-notice mr-notice--warning" role="alert">
            <mat-icon aria-hidden="true">warning</mat-icon>
            @if (erasing()) {
              <p>{{ who() }} nesse quadrado. Com a parede de volta, quem está nele some da vista dos jogadores.</p>
            } @else {
              <p>{{ who() }} nesse quadrado. Com a porta fechada, quem está nele some da vista dos jogadores até a porta abrir.</p>
            }
          </div>
        </app-map-ask>
      } @else {
        <h2 class="mr-panel__title" id="dp-title">Porta</h2>
        <p class="dp__lead">Toque num quadrado para pôr a porta do tipo escolhido.</p>
        <ul class="dp__list">
          <li>Num quadrado de parede com chão dos dois lados, abre o vão e põe a porta.</li>
          <li>Numa porta, troca o tipo.</li>
          <li>“Tirar a porta” fecha o vão de volta, como parede.</li>
        </ul>
        @if (refusal()) {
          <div class="mr-notice mr-notice--danger" role="alert">
            <mat-icon aria-hidden="true">warning</mat-icon>
            <p>{{ refusal() }}</p>
          </div>
        }
        <p class="dp__note">
          Os jogadores veem uma porta trancada como porta fechada (a tranca nunca aparece no mapa deles) e uma porta secreta como parede, até
          você revelá-la na sessão.
        </p>
      }
    </section>
  `,
  styles: `
    :host {
      display: block;
    }

    .dp {
      display: flex;
      flex-direction: column;
      gap: var(--mr-space-3);
    }

    .dp .mr-panel__title {
      margin: 0;
    }

    .dp__lead,
    .dp__note {
      margin: 0;
      font-size: 16px;
      line-height: 22px;
    }

    .dp__note {
      padding-top: var(--mr-space-3);
      border-top: 1px solid var(--mr-line);
      font-size: 14px;
      line-height: 19px;
      color: var(--mr-ink-muted);
    }

    .dp__list {
      display: flex;
      flex-direction: column;
      gap: var(--mr-space-2);
      margin: 0;
      padding-left: 20px;
      font-size: 15px;
      line-height: 21px;
    }
  `,
})
export class DoorPanel {
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);

  /** Why the last tap did nothing; empty when it did. */
  readonly refusal = input('');
  /** The names of those standing where the tap would put a door; `null` when nobody does. */
  readonly standing = input<readonly string[] | null>(null);
  /** The question is about "Tirar a porta": the wall comes back over them. */
  readonly erasing = input(false);

  readonly confirm = output<void>();
  readonly cancel = output<void>();

  constructor() {
    // The question opens wholly in view, at any size (below the map at 1024, the panel may be off the screen).
    effect(() => {
      if (this.standing()) {
        untracked(() =>
          afterNextRender(
            () => this.host.nativeElement.scrollIntoView({ block: 'center', behavior: 'smooth' }),
            { injector: this.injector },
          ),
        );
      }
    });
  }

  /** "Toren está" / "Toren e Goblin 1 estão". */
  protected readonly who = computed(() => {
    const names = this.standing() ?? [];
    const last = names[names.length - 1] ?? '';
    return names.length > 1 ? `${names.slice(0, -1).join(', ')} e ${last} estão` : `${last} está`;
  });
}
