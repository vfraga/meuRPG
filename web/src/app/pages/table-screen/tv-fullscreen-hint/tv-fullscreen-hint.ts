import { ChangeDetectionStrategy, Component, output } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

/** "Pressione F11 ou clique na tela para pôr em tela cheia": a button, so the click does it. */
@Component({
  selector: 'app-tv-fullscreen-hint',
  imports: [MatIconModule],
  template: `
    <button type="button" class="tv-hint" (click)="enter.emit()">
      <mat-icon aria-hidden="true">fullscreen</mat-icon>
      <span><strong>Pressione F11 ou clique na tela</strong> para pôr em tela cheia.</span>
    </button>
  `,
  styleUrl: './tv-fullscreen-hint.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class TvFullscreenHint {
  readonly enter = output<void>();
}
