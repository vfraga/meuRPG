import {
  Component,
  DestroyRef,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  inject,
  effect,
  input,
  model,
  output,
  untracked,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { combatantInitial } from '../../../../core/combat/combat-view';
import { type ReactorRow } from '../../../../core/combat/theatre';
import { tieNumbers } from '../../../../core/format/text';
import { CombatantToken } from '../../../../shared/combatant-token/combatant-token';

let nextId = 0;

/**
 * "Oferecer ataque de oportunidade" (RN-25, E10-04 state 3), in a combat without a map and on a grid: without a map the server never finds out
 * that someone left a reach, so the master says it; on a grid the server only sees what it measures, so the master adds the rest (the
 * distances here are the map's, only to help, and the candidates are ringed on it). The mover is the one on turn; here he picks whose reach it left ("Saiu do alcance de"), and the player of that
 * character gets the question on their phone (the master answers for an NPC). The two buttons are the same size; "Oferecer a ..." is the
 * filled one while the panel is open, and "Próximo turno" goes outlined. The server has the last word on who can react (`NO_OPPORTUNITY`, `REACTION_USED`); the
 * list only leaves out whoever is on the mover's side or already out.
 */
@Component({
  selector: 'app-offer-panel',
  imports: [CombatantToken, MatButtonModule, MatIconModule],
  template: `
    <section class="offer" [attr.aria-labelledby]="id + '-t'">
      <h3 class="offer__title" [id]="id + '-t'">Oferecer ataque de oportunidade</h3>
      <p class="offer__text">
        {{ line() }}
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
            <span class="row__text">
              <span class="row__name">{{ r.label }}</span>
              <span class="row__sub" [title]="subOf(r)">{{ subOf(r) }}</span>
            </span>
          </label>
        } @empty {
          <p class="offer__text">Não há ninguém do outro lado que possa reagir.</p>
        }
      </div>
      @if (grid()) {
        <p class="offer__note">A distância é do mapa, só para ajudar: quem decide é você.</p>
      }
      <div class="actions">
        <button matButton="outlined" type="button" class="btn" [disabled]="busy()" (click)="cancel.emit()">Não oferecer</button>
        <button matButton="filled" type="button" class="btn btn--main" [disabled]="!picked() || busy()" disabledInteractive (click)="send()">
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

  readonly rows = input.required<readonly ReactorRow[]>();
  readonly busy = input(false);
  readonly error = input('');
  /** The combat has a map: the candidates are ringed on it and each row says how far it is now (`hints`). */
  readonly grid = input(false);
  /** By candidate: "agora a 3 m", from the mover's own attacks; none for a theatre or an unknown distance. */
  readonly hints = input<ReadonlyMap<string, string>>(new Map());
  /** The map shows beside the panel (not a phone's own "Mapa" tab): the line says the candidates are ringed on it. */
  readonly mapBeside = input(false);
  /** Counts the times the master chose the offer again while the panel was open: the focus goes back to the list. */
  readonly focusAsk = input(0);
  readonly offer = output<string>();
  readonly cancel = output<void>();

  protected readonly id = `offer-${nextId++}`;
  /** The candidate chosen: the page reads it to ring the map and writes it when a ring is tapped. */
  readonly picked = model('');
  protected readonly toName = computed(() => {
    const r = this.rows().find((x) => x.id === this.picked());
    return r ? tieNumbers(`a ${r.label}`) : '';
  });

  protected readonly line = computed(() =>
    !this.grid()
      ? 'O jogador decide no celular; atacar gasta a reação dele.'
      : this.mapBeside()
        ? 'Os candidatos têm um anel no mapa.'
        : 'Escolha quem saiu do alcance.',
  );

  constructor() {
    // A pick that stopped being able to react (its reaction was spent, an offer was made): the first one that can takes its place.
    effect(() => {
      const rows = this.rows();
      const picked = this.picked();
      untracked(() => {
        if (picked && !rows.some((r) => r.id === picked && !r.spent && !r.offered)) {
          this.picked.set(rows.find((r) => !r.spent && !r.offered)?.id ?? '');
        }
      });
    });
    // The offer was chosen again while the panel is open: the focus lands on the checked radio, after the menu has given it back.
    let asked = this.focusAsk();
    let timer: ReturnType<typeof setTimeout> | undefined;
    inject(DestroyRef).onDestroy(() => clearTimeout(timer));
    effect(() => {
      const n = this.focusAsk();
      if (n !== asked) {
        asked = n;
        clearTimeout(timer);
        timer = setTimeout(() => this.focusChecked());
      }
    });
    // Opens on the first one that can react, so the keyboard lands on the list.
    afterNextRender(
      () => {
        const first = this.rows().find((r) => !r.spent && !r.offered);
        const kept = this.rows().some((r) => r.id === this.picked() && !r.spent && !r.offered);
        if (first && !kept) {
          this.picked.set(first.id);
        }
        queueMicrotask(() => this.focusChecked());
      },
      { injector: this.injector },
    );
  }

  private focusChecked(): void {
    this.host.nativeElement.querySelector<HTMLInputElement>('input:checked')?.focus();
  }

  /** The line under a name: why it cannot receive the offer, or what it is and how far it stands now. */
  protected subOf(r: ReactorRow): string {
    if (r.spent || r.offered) {
      return r.spent ? 'Reação usada' : 'Oferta feita';
    }
    return [r.sub, this.hints().get(r.id)].filter(Boolean).join(' · ');
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
