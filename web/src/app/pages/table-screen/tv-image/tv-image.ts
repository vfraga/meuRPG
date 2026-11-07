import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

/** The image the master shows to the whole table, as big as the screen allows, with its name under it. */
@Component({
  selector: 'app-tv-image',
  imports: [MatIconModule],
  templateUrl: './tv-image.html',
  styleUrl: './tv-image.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class TvImage {
  readonly name = input('');
  readonly url = input.required<string>();
  readonly width = input(0);
  readonly height = input(0);
}
