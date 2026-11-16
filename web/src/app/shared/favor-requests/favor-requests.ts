import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  effect,
  inject,
  output,
  signal,
  untracked,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { FavorPassVm, FavorRefundVm } from '../../core/play/favor-client';
import {
  FAVOR_WORK_LABEL,
  favorPassBody,
  favorPassTitle,
  favorRefundBody,
  favorRefundNote,
} from '../../core/play/favor-pass';
import { FavorState } from '../../core/play/favor-state';
import { FavorLines } from '../favor-lines/favor-lines';

/**
 * "Para você responder" (RN-43, boards 581-05 and 581-06): what the Inspiração asks of the master, at the top of what waits for him.
 * Two cards, each in a 1 px `--mr-accent` frame on the surface:
 *
 * - "Brisa quer passar a Inspiração": "**Para Tor.** Se aceitar, ela sai de Brisa e vai para Tor." with "Recusar" and "Aceitar". When
 *   the receiver has one already (`receiver_has`) the card says "**Para Pensantus.** Pensantus já tem Inspiração: aceitar não é
 *   possível." and "Aceitar" is dashed with that line as its reason; "Recusar" works.
 * - "Inspiração sem efeito": the roll spent it with advantage it had already, for a reason the roller cannot read, so the card names the
 *   roller (a chip), the roll and the hidden sources (only the master reads them) and asks "Manter gasta" or "Devolver a Inspiração".
 *
 * The heading counts the cards in an accent badge and says "A mesa não espera". A card with its call on its way is "in flight": the
 * pressed button reads "Aceitando…", "Recusando…", "Devolvendo…" or "Mantendo…" and its pair is dashed (still focusable, `aria-disabled`),
 * so a second tap cannot answer twice. A card leaves when it is answered; if it closed before the answer the notices under the heading
 * say why (`FavorLines`: "Brisa cancelou o pedido antes da sua resposta. Nada mudou.", "Esse pedido já foi respondido em outra tela.",
 * "Jabuti já tem Inspiração de novo: não há o que devolver. O pedido fechou."), and an accept the server refuses with `CANNOT_RECEIVE`
 * says "Essa personagem não pode ficar com a Inspiração agora." and leaves the card.
 *
 * Nothing is drawn while nothing waits except the notices, so the host takes no room. The focus is never taken when a card arrives (an
 * alert says its title once); after an answer it goes to the next card's title, or, with none left, `answered` tells the page to put it
 * where it belongs. It reads the root `FavorState`, which the live session keeps current on every `vitals_changed` hint.
 */
@Component({
  selector: 'app-favor-requests',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatButtonModule, MatIconModule, FavorLines],
  template: `
    @if (count() > 0) {
      <section class="fr" aria-labelledby="fr-h">
        <header class="fr__head">
          <h2 class="fr__title" id="fr-h">
            <mat-icon aria-hidden="true">star_border</mat-icon>Para você responder
          </h2>
          <span class="fr__count" data-testid="favor-count"
            >{{ count()
            }}<span class="mr-visually-hidden">
              {{ count() === 1 ? 'pedido' : 'pedidos' }}</span
            ></span
          >
          <span class="fr__wait">A mesa não espera</span>
        </header>
        <ul class="fr__list">
          @for (p of passes(); track p.id) {
            <li>
              <article
                class="fr__card"
                data-testid="favor-pass-card"
                [attr.aria-labelledby]="'fr-t-' + p.id"
              >
                <mat-icon class="fr__icon" aria-hidden="true">keyboard_tab</mat-icon>
                <div class="fr__body">
                  <h3 class="fr__card-title" [id]="'fr-t-' + p.id" tabindex="-1">
                    {{ title(p) }}
                  </h3>
                  <p class="fr__text" [id]="'fr-b-' + p.id">
                    @for (piece of body(p); track $index) {
                      @if (piece.bold) {
                        <b>{{ piece.text }}</b>
                      } @else {
                        {{ piece.text }}
                      }
                    }
                  </p>
                  <div class="fr__pair">
                    <button
                      matButton="outlined"
                      type="button"
                      class="fr__btn"
                      data-answer="refuse"
                      [class.mr-button--off]="pairOff(p.id, 'refuse')"
                      [attr.aria-disabled]="busy(p.id) ? 'true' : null"
                      [disabled]="pairOff(p.id, 'refuse')"
                      disabledInteractive
                      [attr.aria-describedby]="'fr-t-' + p.id"
                      (click)="refuse(p)"
                    >
                      <mat-icon aria-hidden="true">close</mat-icon
                      >{{ label(p.id, 'refuse', 'Recusar') }}
                    </button>
                    <button
                      matButton="filled"
                      type="button"
                      class="fr__btn"
                      data-answer="accept"
                      [class.mr-button--off]="p.receiverHas || pairOff(p.id, 'accept')"
                      [attr.aria-disabled]="p.receiverHas || busy(p.id) ? 'true' : null"
                      [disabled]="p.receiverHas || pairOff(p.id, 'accept')"
                      disabledInteractive
                      [attr.aria-describedby]="p.receiverHas ? 'fr-b-' + p.id : 'fr-t-' + p.id"
                      (click)="accept(p)"
                    >
                      <mat-icon aria-hidden="true">{{
                        p.receiverHas ? 'block' : 'check'
                      }}</mat-icon
                      >{{ label(p.id, 'accept', 'Aceitar') }}
                    </button>
                  </div>
                </div>
              </article>
            </li>
          }
          @for (r of refunds(); track r.id) {
            <li>
              <article
                class="fr__card"
                data-testid="favor-refund-card"
                [attr.aria-labelledby]="'fr-t-' + r.id"
              >
                <mat-icon class="fr__icon" aria-hidden="true">star_border</mat-icon>
                <div class="fr__body">
                  <h3 class="fr__card-title" [id]="'fr-t-' + r.id" tabindex="-1">
                    Inspiração sem efeito
                  </h3>
                  <span class="mr-tag fr__who" data-testid="favor-who"
                    ><mat-icon aria-hidden="true">person</mat-icon>{{ r.characterName }}</span
                  >
                  <p class="fr__text">
                    @for (piece of refundBody(r); track $index) {
                      @if (piece.bold) {
                        <b>{{ piece.text }}</b>
                      } @else {
                        {{ piece.text }}
                      }
                    }
                  </p>
                  <p class="fr__note" [id]="'fr-n-' + r.id">{{ note(r) }}</p>
                  <div class="fr__pair">
                    <button
                      matButton="outlined"
                      type="button"
                      class="fr__btn"
                      data-answer="keep"
                      [class.mr-button--off]="pairOff(r.id, 'keep')"
                      [attr.aria-disabled]="busy(r.id) ? 'true' : null"
                      [disabled]="pairOff(r.id, 'keep')"
                      disabledInteractive
                      [attr.aria-describedby]="'fr-t-' + r.id"
                      (click)="keep(r)"
                    >
                      <mat-icon aria-hidden="true">money_off</mat-icon
                      >{{ label(r.id, 'keep', 'Manter gasta') }}
                    </button>
                    <button
                      matButton="filled"
                      type="button"
                      class="fr__btn"
                      data-answer="give"
                      [class.mr-button--off]="pairOff(r.id, 'give')"
                      [attr.aria-disabled]="busy(r.id) ? 'true' : null"
                      [disabled]="pairOff(r.id, 'give')"
                      disabledInteractive
                      [attr.aria-describedby]="'fr-t-' + r.id"
                      (click)="give(r)"
                    >
                      <mat-icon aria-hidden="true">undo</mat-icon
                      >{{ label(r.id, 'give', 'Devolver a Inspiração') }}
                    </button>
                  </div>
                </div>
              </article>
            </li>
          }
        </ul>
      </section>
    }
    <app-favor-lines />
    <p class="mr-visually-hidden" role="alert">{{ news() }}</p>
  `,
  styleUrl: './favor-requests.scss',
})
export class FavorRequests {
  protected readonly favor = inject(FavorState);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);

  /** The last card was answered while the focus was on a card: the page moves it on. */
  readonly answered = output<void>();

  protected readonly passes = this.favor.passes;
  protected readonly refunds = this.favor.refunds;
  protected readonly count = this.favor.asked;
  /** The title of a card that just arrived, said once in an alert. */
  protected readonly news = signal('');

  protected readonly title = (p: FavorPassVm) => favorPassTitle(p.fromName);
  protected readonly body = favorPassBody;
  protected readonly refundBody = favorRefundBody;
  protected note(r: FavorRefundVm): string {
    return favorRefundNote(r.characterName);
  }

  constructor() {
    let seen: readonly string[] = [];
    effect(() => {
      const cards = [
        ...this.passes().map((p) => ({ id: p.id, title: favorPassTitle(p.fromName) })),
        ...this.refunds().map((r) => ({ id: r.id, title: 'Inspiração sem efeito' })),
      ];
      untracked(() => {
        const ids = cards.map((c) => c.id);
        const fresh = cards.filter((c) => !seen.includes(c.id));
        const gone = seen.some((id) => !ids.includes(id));
        if (fresh.length > 0) {
          this.news.set(fresh.map((c) => c.title).join('. '));
        }
        if (gone) {
          this.moveFocus(ids.length > 0);
        }
        seen = ids;
      });
    });
  }

  /** What the call of a card is doing, as the card reads it. */
  protected busy(id: string): boolean {
    return this.favor.workOf(id) !== null;
  }

  /** A card with a call on its way has both buttons dashed, except the pressed one, which keeps its look and reads the gerund. */
  protected pairOff(id: string, own: 'refuse' | 'accept' | 'keep' | 'give'): boolean {
    const work = this.favor.workOf(id);
    return work !== null && work !== own;
  }

  protected label(id: string, own: 'refuse' | 'accept' | 'keep' | 'give', text: string): string {
    return this.favor.workOf(id) === own ? FAVOR_WORK_LABEL[own] : text;
  }

  protected accept(p: FavorPassVm): void {
    void this.favor.answerPass(p, true);
  }

  protected refuse(p: FavorPassVm): void {
    void this.favor.answerPass(p, false);
  }

  protected give(r: FavorRefundVm): void {
    void this.favor.answerRefund(r, true);
  }

  protected keep(r: FavorRefundVm): void {
    void this.favor.answerRefund(r, false);
  }

  /** After an answer: the next card's title, or, with none left, the page's next stop. Only when the focus was on a card (or fell to
   * the page with the card that held it): an answer from another screen never steals it. */
  private moveFocus(more: boolean): void {
    const active = document.activeElement;
    const ours = !active || active === document.body || this.host.nativeElement.contains(active);
    if (!ours) {
      return;
    }
    afterNextRender(
      () => {
        if (more) {
          this.host.nativeElement.querySelector<HTMLElement>('.fr__card-title')?.focus();
        } else {
          this.answered.emit();
        }
      },
      { injector: this.injector },
    );
  }
}
