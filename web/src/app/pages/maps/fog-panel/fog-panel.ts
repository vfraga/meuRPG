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
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { LightLevel, type Map as MapMessage } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { focusWithRing } from '../../../core/creatures/focus-ring';
import { editorErrorMessage } from '../../../core/maps/map-errors';
import { MapsClient } from '../../../core/maps/maps-client';
import { MapAsk } from '../../../shared/map-ask/map-ask';
import { RevealSwitch } from '../../../shared/reveal-switch/reveal-switch';

const LEVELS: readonly { level: LightLevel; label: string; icon: string }[] = [
  { level: LightLevel.BRIGHT, label: 'Claro', icon: 'light_mode' },
  { level: LightLevel.DIM, label: 'Penumbra', icon: 'contrast' },
  { level: LightLevel.DARK, label: 'Escuro', icon: 'dark_mode' },
];

/**
 * "Névoa de guerra" (E9-01 1, 4, 5, 6, 7; MR-036): the map's fog settings, saved at once (`SetMapFog`, each field
 * on its own) — "Ligar a névoa", "Visão do grupo", the "Luz de base" (Claro, Penumbra, Escuro) — and "Esquecer o que
 * foi visto", which asks in place (the players keep only what their character sees now; it cannot be undone).
 * The fog needs a grid: without one the switches stay off and say why. The same panel is on the master's phone,
 * where the settings are editable (only painting is not). Presentational plus the calls; the page keeps the map.
 */
@Component({
  selector: 'app-fog-panel',
  imports: [MapAsk, MatButtonModule, MatIconModule, RevealSwitch],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './fog-panel.html',
  styleUrl: './fog-panel.scss',
})
export class FogPanel {
  private readonly api = inject(MapsClient);
  private readonly injector = inject(Injector);

  readonly campaignId = input.required<string>();
  readonly map = input.required<MapMessage>();

  /** The map as the server has it after a change. */
  readonly changed = output<MapMessage>();
  /** "Esquecer o que foi visto" was done (the "Ver como" counts change). */
  readonly forgotten = output<void>();

  protected readonly levels = LEVELS;
  protected readonly busy = signal(false);
  protected readonly asking = signal(false);
  protected readonly error = signal('');
  protected readonly note = signal('');
  private readonly opener = viewChild('opener', { read: ElementRef<HTMLButtonElement> });

  protected readonly hasGrid = computed(() => this.map().gridColumns > 0);
  protected readonly on = computed(() => this.map().fogEnabled);
  protected readonly base = computed(() => this.map().baseLight);
  protected readonly bright = LightLevel.BRIGHT;

  protected async set(changes: {
    fogEnabled?: boolean;
    baseLight?: LightLevel;
    groupVision?: boolean;
  }): Promise<void> {
    if (this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    this.note.set('');
    try {
      this.changed.emit(await this.api.setFog(this.campaignId(), this.map().id, changes));
    } catch (err) {
      this.error.set(editorErrorMessage(err, 'fog', 'mudar a névoa'));
    } finally {
      this.busy.set(false);
    }
  }

  protected ask(): void {
    this.error.set('');
    this.asking.set(true);
  }

  protected cancel(): void {
    this.asking.set(false);
    afterNextRender(() => focusWithRing(this.opener()?.nativeElement), { injector: this.injector });
  }

  protected async forget(): Promise<void> {
    if (this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      await this.api.forgetVision(this.campaignId(), this.map().id);
      this.asking.set(false);
      this.note.set('Pronto: os jogadores voltaram a ver só o que o personagem deles vê agora.');
      this.forgotten.emit();
      afterNextRender(() => focusWithRing(this.opener()?.nativeElement), {
        injector: this.injector,
      });
    } catch (err) {
      this.error.set(editorErrorMessage(err, 'forget', 'esquecer o que foi visto'));
    } finally {
      this.busy.set(false);
    }
  }

  /** Arrow keys inside the light's choice (the radio pattern). */
  protected onKey(event: KeyboardEvent): void {
    const keys = ['ArrowLeft', 'ArrowUp', 'ArrowRight', 'ArrowDown'];
    if (!keys.includes(event.key) || !this.hasGrid()) {
      return;
    }
    event.preventDefault();
    const step = event.key === 'ArrowLeft' || event.key === 'ArrowUp' ? -1 : 1;
    const buttons = Array.from(
      (event.currentTarget as HTMLElement).querySelectorAll<HTMLElement>('[role="radio"]'),
    );
    const here = buttons.findIndex((b) => b === document.activeElement);
    const next = buttons[(Math.max(0, here) + step + buttons.length) % buttons.length];
    next?.focus();
    next?.click();
  }
}
