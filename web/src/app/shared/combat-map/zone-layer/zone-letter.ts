import { ChangeDetectionStrategy, Component, input } from '@angular/core';

import {
  ZONE_BADGE_BORDER_PX,
  ZONE_BADGE_INSET_PX,
  ZONE_BADGE_PX,
} from '../../../core/combat/zone-badge';

/**
 * A zone's letter in a white diamond, for the legend and the zones list: the same mark as the map's corner badge, so a letter
 * on the map finds its name beside it. It is a diamond, never a square: a square reads as an NPC token on the map.
 * The picture says "Zona A" to a screen reader.
 */
@Component({
  selector: 'app-zone-letter',
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: {
    '[style.--zl-badge]': "badgePx + 'px'",
    '[style.--zl-badge-inset]': "badgeInsetPx + 'px'",
    '[style.--zl-badge-border]': "badgeBorderPx + 'px'",
  },
  template: `<span class="zlt" role="img" [attr.aria-label]="'Zona ' + letter()"><span class="zlt__l" aria-hidden="true">{{ letter() }}</span></span>`,
  styles: `
    :host {
      display: inline-flex;
      flex: none;
    }
    .zlt {
      position: relative;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: var(--zl-badge);
      height: var(--zl-badge);
    }
    .zlt::before {
      content: '';
      position: absolute;
      inset: var(--zl-badge-inset);
      border: var(--zl-badge-border) solid var(--mr-map-token-ink);
      background: var(--mr-map-token-surface);
      transform: rotate(45deg);
    }
    .zlt__l {
      position: relative;
      color: var(--mr-map-token-ink);
      font-size: var(--mr-font-12);
      font-weight: 700;
      line-height: 1;
    }
  `,
})
export class ZoneLetter {
  readonly letter = input.required<string>();
  protected readonly badgePx = ZONE_BADGE_PX;
  protected readonly badgeInsetPx = ZONE_BADGE_INSET_PX;
  protected readonly badgeBorderPx = ZONE_BADGE_BORDER_PX;
}
