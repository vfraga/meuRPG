import {
  ChangeDetectionStrategy,
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
import { MatIconModule } from '@angular/material/icon';
import { MatMenuModule } from '@angular/material/menu';

import type { CharacterCreature } from '../../../../gen/meurpg/characters/v1/characters_pb';
import { focusWithRing } from '../../../core/creatures/focus-ring';
import { STEED_RELEASE_LINE, steedAwaySubtitle } from '../../../core/creatures/steed-card';
import { CreatureArt } from '../../../shared/creatures/creature-art';
import { SteedAsk } from '../../../shared/creatures/steed-ask';

/**
 * A bonded steed that is away (#588, board 588-26 card 1): dismissed, or gone at 0 hit points. `ListCharacterCreatures` does
 * not list it, so the panel draws it from `SummonSpellOptions.bonded_steed`: a dashed row with the muted silhouette, the name, the
 * form and the type ("… · fora da mesa") and the dashed tag "Fora". It has no numbers and no page; how it comes back is
 * said under the cast button. Its one action is "⋮" › "Libertar Estrela", asked
 * in the row (`SteedAsk`); out of a combat it spends nothing.
 */
@Component({
  selector: 'app-steed-away',
  imports: [CreatureArt, MatIconModule, MatMenuModule, SteedAsk],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @let s = steed();
    <article class="away" [attr.aria-labelledby]="'steed-away-' + s.id">
      <div class="away__head">
        <app-creature-art class="away__art" [monsterKey]="s.monsterKey" />
        <div class="away__text">
          <h3 class="away__name" tabindex="-1" [id]="'steed-away-' + s.id">{{ s.name }}</h3>
          <p class="away__sub">{{ subtitle() }}</p>
        </div>
        <span class="mr-tag away__tag">Fora</span>
        @if (!asking()) {
          <button type="button" class="away__more js-more" [matMenuTriggerFor]="more" [attr.aria-label]="'Mais ações de ' + s.name">
            <mat-icon aria-hidden="true">more_vert</mat-icon>
          </button>
          <mat-menu #more="matMenu">
            <button type="button" mat-menu-item class="release" (click)="asking.set(true)">
              <mat-icon aria-hidden="true">link_off</mat-icon>
              <span class="release__t">Libertar {{ s.name }}</span>
              <span class="release__line">{{ releaseLine }}</span>
            </button>
          </mat-menu>
        }
      </div>
      @if (asking()) {
        <app-steed-ask
          [campaignId]="campaignId()"
          [creature]="s"
          mode="release"
          [ownerView]="!isMaster()"
          [ownerName]="ownerName()"
          (closed)="asked($event)"
        />
      }
    </article>
  `,
  styleUrls: ['./steed-away.scss', '../../../shared/creatures/steed-release.scss'],
})
export class SteedAway {
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);

  readonly campaignId = input.required<string>();
  /** The bonded steed, as `SummonSpellOptions.bonded_steed` sends it. */
  readonly steed = input.required<CharacterCreature>();
  readonly ownerName = input('');
  readonly isMaster = input(false);
  /** The bond ended: the panel reads again, and the row goes. */
  readonly released = output<void>();

  protected readonly asking = signal(false);
  protected readonly releaseLine = STEED_RELEASE_LINE;
  protected readonly subtitle = computed(() =>
    steedAwaySubtitle(this.steed().monsterNamePt, this.steed().creatureType),
  );

  protected asked(changed: boolean): void {
    this.asking.set(false);
    if (changed) {
      this.released.emit();
      return;
    }
    afterNextRender(
      () => focusWithRing(this.host.nativeElement.querySelector<HTMLElement>('.js-more')),
      { injector: this.injector },
    );
  }
}
