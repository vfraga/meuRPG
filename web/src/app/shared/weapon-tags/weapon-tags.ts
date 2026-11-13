import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { MarksVm } from '../../core/characters/gear';

/** The marks of a weapon as tags: a crescent for "Prateada", a crystal for "Adamantina", a spark for "Mágica". The word
 * is always there; the icon only helps the eye. Nothing is drawn for a plain weapon. */
@Component({
  selector: 'app-weapon-tags',
  imports: [MatIconModule],
  templateUrl: './weapon-tags.html',
  styleUrl: './weapon-tags.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class WeaponTags {
  readonly marks = input<MarksVm | null | undefined>(null);

  protected readonly tags = computed(() => {
    const m = this.marks();
    return [
      m?.silvered && { word: 'Prateada', icon: 'dark_mode' },
      m?.adamantine && { word: 'Adamantina', icon: 'diamond' },
      m?.magical && { word: 'Mágica', icon: 'auto_awesome' },
    ].filter((t): t is { word: string; icon: string } => !!t);
  });
}
