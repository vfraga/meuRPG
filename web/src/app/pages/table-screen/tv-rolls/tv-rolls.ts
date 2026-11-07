import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { TvRollVm } from '../../../core/play/table-screen-view';
import { TvFit } from '../tv-fit/tv-fit';

/** "4 e 4", "2, 5 e 6": the faces that came up, as a sentence. */
export function facesText(faces: readonly number[]): string {
  if (faces.length < 2) {
    return faces.join('');
  }
  return `${faces.slice(0, -1).join(', ')} e ${faces[faces.length - 1]}`;
}

/** "Rolagens da mesa": the free rolls the whole table sees, newest first; the newest is the big one. */
@Component({
  selector: 'app-tv-rolls',
  imports: [MatIconModule, TvFit],
  templateUrl: './tv-rolls.html',
  styleUrl: './tv-rolls.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class TvRolls {
  readonly rolls = input<readonly TvRollVm[]>([]);

  protected readonly newest = computed(() => this.rolls()[0] ?? null);
  protected readonly older = computed(() => this.rolls().slice(1));
  protected readonly faces = facesText;
}
