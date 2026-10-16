import {
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  inject,
  input,
  output,
  signal,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { article } from '../../../../core/combat/combat-log';
import { combatantInitial } from '../../../../core/combat/combat-view';
import { type ReactorRow } from '../../../../core/combat/theatre';
import { tieNumbers, upFirst } from '../../../../core/format/text';
import { CombatantToken } from '../../../../shared/combatant-token/combatant-token';

let nextId = 0;

/**
 * "Oferecer ataque de oportunidade" (RN-25, E10-04 state 3): in a combat without a map the server never finds out that someone left a reach, so
 * the master says it. The mover is the one on turn; here he picks whose reach it left ("Saiu do alcance de"), and the player of that
 * character gets the question on their phone (the master answers for an NPC). The two buttons are outlined and the same size, because
 * "Próximo turno" is the one filled button of the page. The server has the last word on who can react (`NO_OPPORTUNITY`, `REACTION_USED`); the
 * list only leaves out whoever is on the mover's side or already out.
 */
@Component({
  selector: 'app-offer-panel',
  imports: [CombatantToken, MatButtonModule, MatIconModule],
  template: `
    <section class="offer" [attr.aria-labelledby]="id + '-t'">
      <h3 class="offer__title" [id]="id + '-t'">Oferecer ataque de oportunidade</h3>
      <p class="offer__text">
        {{ mover() }} saiu do alcance de alguém? Escolha quem. O jogador recebe a pergunta no celular e decide; atacar gasta a reação dele.
      </p>
      @if (error()) {
        <p class="offer__error" role="alert"><mat-icon aria-hidden="true">error</mat-icon>{{ error() }}</p>
      }
      <p class="offer__sub" [id]="id + '-s'">Saiu do alcance de</p>
      <div class="rows" role="radiogroup" [attr.aria-labelledby]="id + '-s'">
        @for (r of rows(); track r.id) {
          <label class="row" [class.row--on]="picked() === r.id" [class.row--off]="r.spent || r.offered">
            <input
              type="radio"
              class="mr-visually-hidden"
              [name]="id"
              [value]="r.id"
              [checked]="picked() === r.id"
              [disabled]="r.spent || r.offered"
              (change)="picked.set(r.id)"
            />
            <span class="row__dot" aria-hidden="true"></span>
            <app-combatant-token [initial]="initial(r.label)" [npc]="!r.player" [size]="30" />
            <span class="row__name">{{ r.label }}</span>
            <span class="row__sub">{{ r.spent ? 'Reação usada' : r.offered ? 'Oferta feita' : r.sub }}</span>
          </label>
        } @empty {
          <p class="offer__text">Não há ninguém do outro lado que possa reagir.</p>
        }
      </div>
      <div class="actions">
        <button mat-stroked-button type="button" class="btn" [disabled]="busy()" (click)="cancel.emit()">Não oferecer</button>
        <button mat-stroked-button type="button" class="btn" [disabled]="!picked() || busy()" disabledInteractive (click)="send()">
          {{ picked() ? 'Oferecer ' + toName() : 'Oferecer' }}
        </button>
      </div>
    </section>
  `,
  styleUrl: './offer-panel.scss',
})
export class OfferPanel {
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);

  /** The mover's label ("Goblin 1"). */
  readonly moverLabel = input.required<string>();
  readonly rows = input.required<readonly ReactorRow[]>();
  readonly busy = input(false);
  readonly error = input('');
  readonly offer = output<string>();
  readonly cancel = output<void>();

  protected readonly id = `offer-${nextId++}`;
  protected readonly picked = signal('');
  protected readonly mover = computed(() =>
    tieNumbers(`${upFirst(article(this.moverLabel()))} ${this.moverLabel()}`),
  );
  protected readonly toName = computed(() => {
    const r = this.rows().find((x) => x.id === this.picked());
    return r ? tieNumbers(`a ${r.label}`) : '';
  });

  constructor() {
    // Opens on the first one that can react, so the keyboard lands on the list.
    afterNextRender(
      () => {
        const first = this.rows().find((r) => !r.spent && !r.offered);
        if (first) {
          this.picked.set(first.id);
        }
        queueMicrotask(() =>
          this.host.nativeElement.querySelector<HTMLInputElement>('input:checked')?.focus(),
        );
      },
      { injector: this.injector },
    );
  }

  protected initial(label: string): string {
    return combatantInitial(label);
  }

  protected send(): void {
    if (this.picked() && !this.busy()) {
      this.offer.emit(this.picked());
    }
  }
}
