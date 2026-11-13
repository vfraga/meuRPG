import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import { type Combatant, SunlightTrait } from '../../../../../gen/meurpg/play/v1/combat_pb';

/**
 * The master's notice under the action chips for a creature with Sunlight Sensitivity or Weakness, while the combat has a sun
 * (RN-20, RN-29): in the sun it says what the sun spoils (warning, with the sun icon, so it is text and not only color); out of
 * it, that the creature rolls normally. Nothing without the trait or with the sun off. A player never gets the trait, so the
 * notice is the master's.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-sunlight-notice',
  imports: [MatIconModule],
  template: `
    @if (text(); as t) {
      <p
        class="mr-notice sun"
        [class.mr-notice--warning]="creature().inSunlight"
        [class.mr-notice--neutral]="!creature().inSunlight"
        role="status"
        data-testid="sunlight-notice"
      >
        <mat-icon aria-hidden="true">{{ creature().inSunlight ? 'wb_sunny' : 'wb_twilight' }}</mat-icon>
        <span>{{ t }}</span>
      </p>
    }
  `,
  styles: `
    :host {
      display: block;
    }
    .sun {
      margin: 0;
      padding: 8px 12px;
      font-size: var(--mr-font-14);
    }
  `,
})
export class SunlightNotice {
  readonly creature = input.required<Combatant>();
  /** The combat is in sunlight (`Encounter.sunlightOn`). */
  readonly sunlightOn = input(false);

  protected readonly text = computed(() => {
    const c = this.creature();
    if (!this.sunlightOn() || c.sunlightTrait === SunlightTrait.UNSPECIFIED) {
      return '';
    }
    if (!c.inSunlight) {
      return `${c.label} está fora da luz do sol. Rola normal.`;
    }
    return c.sunlightTrait === SunlightTrait.WEAKNESS
      ? `${c.label} está na luz do sol. Desvantagem nos ataques, nas salvaguardas e nos testes de atributo.`
      : `${c.label} está na luz do sol. Desvantagem nos ataques e nos testes de Percepção que dependem da visão.`;
  });
}
