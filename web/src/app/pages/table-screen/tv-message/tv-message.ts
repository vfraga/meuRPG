import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

/** A whole-screen card: the loading, the error, the lost connection, the end, the refusal, the sign-in. */
@Component({
  selector: 'app-tv-message',
  imports: [MatIconModule],
  templateUrl: './tv-message.html',
  styleUrl: './tv-message.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class TvMessage {
  readonly icon = input.required<string>();
  readonly title = input.required<string>();
  readonly detail = input('');
  readonly hint = input('');
  /** The page's own title is `h1`; under a header the card's is `h2`. */
  readonly level = input<1 | 2>(1);
}
