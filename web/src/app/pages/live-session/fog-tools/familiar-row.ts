import { Component, computed, effect, inject, input, signal, untracked } from '@angular/core';

import {
  type CharacterCreature,
  CreatureSource,
} from '../../../../gen/meurpg/characters/v1/characters_pb';
import { sourcePhrase } from '../../../core/creatures/creature-format';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { joinDots } from '../../../core/format/text';
import { CreatureArt } from '../../../shared/creatures/creature-art';
import { FamiliarEyesButton } from '../../../shared/familiar-eyes/familiar-eyes-button';

/**
 * The familiar's row under the session map (E9-04): its picture, its name, "Corvo · familiar de Pensantus"
 * and "Ver pelos olhos". It is for the player whose character has a familiar (the list is the owner's and the
 * master's: RN-20), and goes away while the band says the player is already looking through its eyes. It
 * shows no distance: the server decides whether the familiar is within 30 m and says so when it is not.
 * The list is read again when the stream says the creatures changed (`reload`).
 */
@Component({
  selector: 'app-familiar-row',
  imports: [CreatureArt, FamiliarEyesButton],
  template: `
    @if (familiar(); as f) {
      <section class="fr" [attr.aria-label]="'Seu familiar ' + f.name">
        <app-creature-art class="fr__art" [monsterKey]="f.monsterKey" />
        <div class="fr__text">
          <span class="fr__name">{{ f.name }}</span>
          <span class="fr__sub">{{ sub() }}</span>
        </div>
        <app-familiar-eyes-button
          class="fr__btn"
          [block]="true"
          [campaignId]="campaignId()"
          [characterId]="characterId()"
          [characterName]="characterName()"
          [familiarName]="f.name"
        />
      </section>
    }
  `,
  styles: `
    :host {
      display: block;
    }

    .fr {
      display: flex;
      flex-wrap: wrap;
      align-items: center;
      gap: var(--mr-space-3);
      padding: 10px 14px;
      border: 1px solid var(--mr-control-line);
      border-radius: var(--mr-radius-md);
      background: var(--mr-surface);
    }

    .fr__art {
      --art-size: 48px;
      flex: none;
    }

    .fr__text {
      display: flex;
      flex: 1 1 10rem;
      min-width: 0;
      flex-direction: column;
    }

    .fr__name {
      font-family: var(--mr-font-display);
      font-size: 18px;
      font-weight: 700;
      line-height: 24px;
    }

    .fr__sub {
      color: var(--mr-ink-muted);
      font-size: 15px;
      line-height: 20px;
    }

    @media (max-width: 479.98px) {
      .fr__btn {
        flex: 1 1 100%;
      }
    }
  `,
})
export class FamiliarRow {
  private readonly api = inject(CreaturesClient);

  readonly campaignId = input.required<string>();
  readonly characterId = input.required<string>();
  readonly characterName = input.required<string>();
  /** Bumped when the stream says the creatures changed. */
  readonly reload = input(0);
  /** The player already looks through its eyes: no row. */
  readonly seeing = input(false);

  private readonly creatures = signal<readonly CharacterCreature[]>([]);
  private seq = 0;
  private shownFor = '';

  protected readonly familiar = computed(() =>
    this.seeing()
      ? null
      : (this.creatures().find((c) => c.source === CreatureSource.FAMILIAR) ?? null),
  );
  protected readonly sub = computed(() => {
    const f = this.familiar();
    if (!f) {
      return '';
    }
    const owner = sourcePhrase(f.source, this.characterName());
    return joinDots([f.monsterNamePt, owner.charAt(0).toLocaleLowerCase('pt-BR') + owner.slice(1)]);
  });

  constructor() {
    effect(() => {
      const campaignId = this.campaignId();
      const characterId = this.characterId();
      this.reload();
      // Another character's row never shows the last one's familiar while its own list is on the way.
      const owner = `${campaignId}/${characterId}`;
      if (owner !== this.shownFor) {
        this.shownFor = owner;
        this.creatures.set([]);
      }
      const seq = ++this.seq;
      void untracked(() =>
        this.api.list(campaignId, characterId).then(
          (list) => seq === this.seq && this.creatures.set(list),
          // The owner's list is all this row needs: a refusal or a failure means no row, never an error on the map.
          () => seq === this.seq && this.creatures.set([]),
        ),
      );
    });
  }
}
