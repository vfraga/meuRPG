import {
  Component,
  computed,
  effect,
  inject,
  input,
  output,
  signal,
  untracked,
} from '@angular/core';

import type { CharacterHighlights } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import {
  type HighlightTile,
  type OwnNumber,
  highlightTiles,
  ownNumbers,
} from '../../../../core/combat/combat-highlights';
import { HighlightsFrame } from '../../../../shared/highlights/highlights-frame';

/**
 * The players' "O combate acabou" card (MR-032, E8-11 state 3): it reads the
 * combat's highlights and draws them in the shared card (`HighlightsFrame`,
 * which the session's "A sessão acabou" uses too). A failed read shows nothing:
 * it is a bonus, not a screen the player needs.
 */
@Component({
  selector: 'app-highlights-card',
  imports: [HighlightsFrame],
  template: `
    @if (loaded()) {
      <app-highlights-frame
        ariaLabel="Destaques do combate"
        tag="Combate encerrado"
        title="O combate acabou"
        [subtitle]="subtitle()"
        [tiles]="tiles()"
        none="Ninguém causou, curou ou sofreu dano neste combate."
        [characterId]="characterId()"
        [ownTitle]="own().length > 0 ? 'Seu resultado, ' + characterName() : ''"
        [own]="own()"
        (closed)="closed.emit()"
      />
    }
  `,
  styles: ':host { display: block; margin-top: var(--mr-space-5); }',
})
export class HighlightsCard {
  private readonly api = inject(CombatClient);

  readonly campaignId = input.required<string>();
  readonly encounterId = input.required<string>();
  /** "Emboscada na estrada · 4 rodadas". */
  readonly subtitle = input('');
  /** The reader's own character: marks "Você" and gives "Seu resultado". */
  readonly characterId = input('');
  readonly characterName = input('');

  readonly closed = output<void>();

  protected readonly tiles = signal<readonly HighlightTile[]>([]);
  protected readonly loaded = signal(false);
  private readonly rows = signal<readonly CharacterHighlights[]>([]);
  protected readonly own = computed<OwnNumber[]>(() => ownNumbers(this.rows(), this.characterId()));

  constructor() {
    effect(() => {
      const campaignId = this.campaignId();
      const encounterId = this.encounterId();
      untracked(() => void this.load(campaignId, encounterId));
    });
  }

  private async load(campaignId: string, encounterId: string): Promise<void> {
    this.loaded.set(false);
    try {
      const res = await this.api.turn.highlights(campaignId, encounterId);
      this.tiles.set(highlightTiles(res));
      this.rows.set(res.characters);
      this.loaded.set(true);
    } catch {
      // Best effort: with no highlights there is no card.
      this.tiles.set([]);
      this.loaded.set(false);
    }
  }
}
