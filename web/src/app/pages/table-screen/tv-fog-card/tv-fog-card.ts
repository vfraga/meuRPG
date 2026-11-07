import { ChangeDetectionStrategy, Component } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

/** "O mapa fica na tela de cada um": the fog is on and the group's view is off, so the TV draws no map (RN-10). */
@Component({
  selector: 'app-tv-fog-card',
  imports: [MatIconModule],
  template: `
    <section class="mr-panel tv-fog" aria-labelledby="tv-fog-title">
      <mat-icon class="tv-fog__icon" aria-hidden="true">map</mat-icon>
      <h2 class="tv-fog__title" id="tv-fog-title">O mapa fica na tela de cada um.</h2>
      <p class="tv-fog__text">
        A névoa está ligada e cada jogador vê só o que o próprio personagem vê.
      </p>
    </section>
  `,
  styleUrl: './tv-fog-card.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class TvFogCard {}
