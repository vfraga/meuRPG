import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

/**
 * "★ Inspiração": the read-only mark of a character that has the master's Inspiração (RN-43, board 581). One word and a star,
 * so the mark never rides on colour. As a `pill` it is a chip (the player's turn band, the master's "Grupo" row); as `inline` it
 * is the same filled chip, one line high, beside the name in the order row (like "Vez"; read "Tem Inspiração"). It is drawn only for a reader who may know (the master, or the
 * owner of the character): the caller leaves it out for everybody else (RN-10). While the owner's ask to pass it waits for the master
 * (`waiting`) it reads "Inspiração · esperando o mestre", on the warning surface (581-07, item 4): the words, not the colour, say it.
 */
@Component({
  selector: 'app-favor-chip',
  imports: [MatIconModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `<mat-icon class="favor__icon" aria-hidden="true">star</mat-icon>@if (variant() === 'inline') {<span class="mr-visually-hidden">Tem </span>}Inspiração@if (waiting()) { · esperando o mestre}`,
  styleUrl: './favor-chip.scss',
  host: { '[class]': "'favor favor--' + variant() + (waiting() ? ' favor--waiting' : '')" },
})
export class FavorChip {
  readonly variant = input<'pill' | 'inline'>('pill');
  /** The owner's ask to pass it waits for the master: "Inspiração · esperando o mestre". */
  readonly waiting = input(false);
}
