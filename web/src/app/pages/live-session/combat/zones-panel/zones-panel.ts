import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  effect,
  inject,
  input,
  output,
  signal,
  untracked,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatMenuModule } from '@angular/material/menu';

import { zonePattern } from '../../../../core/combat/zone-pattern';
import type { ZoneView } from '../../../../core/combat/zones';
import { ZoneLetter } from '../../../../shared/combat-map/zone-layer/zone-letter';
import { ZoneSwatch } from '../../../../shared/combat-map/zone-layer/zone-swatch';
import { type ZoneNote, zoneNotice } from './zones-announce';

/**
 * "Zonas ativas": one card per spell zone on the map (RN-37), the accessible twin of the map's drawing. A card reads the spell
 * and caster, the area in meters, the save and damage, the move per turn, the notes, the time left, the concentration and who
 * stands inside; a zone the server sent with hidden details reads name, area and caster, nothing more. The master ends a zone
 * here, after a question in place, and asks to move one (`move`; the page owns what opens). A list with no zone draws nothing,
 * but the live region stays so the first zone is announced.
 */
@Component({
  selector: 'app-zones-panel',
  imports: [MatButtonModule, MatIconModule, MatMenuModule, ZoneLetter, ZoneSwatch],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './zones-panel.html',
  styleUrl: './zones-panel.scss',
})
export class ZonesPanel {
  readonly zones = input.required<readonly ZoneView[]>();
  /** The master reads the panel: the footnote says how a zone ends. */
  readonly master = input(false);
  /** Another call is in the air. */
  readonly busy = input(false);
  /** A refusal of the end, said under its question. */
  readonly error = input('');
  /** "Encerrar zona" was confirmed: the zone's id. */
  readonly end = output<string>();
  /** "Mover": the zone's id. */
  readonly move = output<string>();

  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);
  private readonly asking = signal<string | null>(null);
  /** The zone whose end is being asked: none once the zone is gone from the list. */
  protected readonly confirming = computed(() => {
    const id = this.asking();
    return id !== null && this.zones().some((z) => z.id === id) ? id : null;
  });
  protected readonly notice = signal('');
  protected readonly pattern = zonePattern;
  private known: readonly ZoneNote[] | null = null;

  constructor() {
    effect(() => {
      const now = this.zones().map((z) => ({ id: z.id, label: z.label, letter: z.letter }));
      untracked(() => {
        const text = zoneNotice(this.known, now);
        this.known = now;
        if (text !== '') {
          this.notice.set(text);
        }
      });
    });
  }

  protected ask(id: string): void {
    this.asking.set(id);
    this.focusLater('.zp__no');
  }

  protected cancel(id: string): void {
    this.asking.set(null);
    this.focusLater(`[data-zone="${id}"] .zp__more`);
  }

  private focusLater(selector: string): void {
    afterNextRender(() => this.host.nativeElement.querySelector<HTMLElement>(selector)?.focus(), {
      injector: this.injector,
    });
  }
}
