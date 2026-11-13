import { ChangeDetectionStrategy, Component, computed, input, output } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import { type Encounter, SunlightScope } from '../../../../../gen/meurpg/play/v1/combat_pb';

/**
 * The master's sun switch (#704): one `role="switch"` named "Luz do sol: desligada / todo o mapa / onde eu pintar". It sits in the
 * combat bar (from 768 px, and in the setup) and, on a phone, in the combat header; whoever renders it renders it once.
 */
@Component({
  selector: 'app-sun-switch',
  imports: [MatIconModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './sun-switch.html',
  styleUrl: './sun-switch.scss',
})
export class SunSwitch {
  readonly encounter = input.required<Encounter>();
  readonly busy = input(false);
  /** `true` asks for the sun, `false` takes it out (the scope stays). */
  readonly flip = output<boolean>();

  protected readonly on = computed(() => this.encounter().sunlightOn);
  /** "Luz do sol: todo o mapa" / "Luz do sol: onde eu pintar" while on, "Luz do sol: desligada" off: the switch's name. */
  protected readonly label = computed(() => {
    const e = this.encounter();
    if (!e.sunlightOn) {
      return 'Luz do sol: desligada';
    }
    return e.sunlightScope === SunlightScope.SQUARES
      ? 'Luz do sol: onde eu pintar'
      : 'Luz do sol: todo o mapa';
  });
}
