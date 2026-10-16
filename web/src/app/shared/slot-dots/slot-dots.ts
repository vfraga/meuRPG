import { Component, computed, input } from '@angular/core';

/**
 * A row of spell slots as the paper sheet draws them (README-A): a free
 * slot is a hollow ink ring; a used one is a filled ink disc crossed out in
 * the surface colour. Decorative only: the row around it carries the words
 * ("2 de 4 usados") and a `role="img"` name.
 */
@Component({
  selector: 'app-slot-dots',
  template: `
    @for (used of dots(); track $index) {
      <svg
        [attr.width]="px()"
        [attr.height]="px()"
        viewBox="0 0 16 16"
        aria-hidden="true"
        [class.dot--used]="used"
      >
        <circle cx="8" cy="8" r="7.25" />
        @if (used) {
          <path d="M4.96 4.96l6.08 6.08M11.04 4.96l-6.08 6.08" />
        }
      </svg>
    }
  `,
  styles: `
    :host {
      display: inline-flex;
      flex-wrap: wrap;
      gap: 6px;
    }
    :host(.slot-dots--small) {
      gap: 4px;
    }
    svg {
      display: block;
      flex-shrink: 0;
    }
    circle {
      fill: none;
      stroke: var(--mr-ink);
      stroke-width: 1.5;
    }
    .dot--used circle {
      fill: var(--mr-ink);
    }
    path {
      stroke: var(--mr-surface);
      stroke-width: 1.6;
      stroke-linecap: round;
    }
  `,
  host: { '[class.slot-dots--small]': "size() === 'small'" },
})
export class SlotDots {
  readonly total = input.required<number>();
  readonly used = input.required<number>();
  /** `small` (13px) in the master's party rows; 16px otherwise. */
  readonly size = input<'regular' | 'small'>('regular');

  protected readonly px = computed(() => (this.size() === 'small' ? 13 : 16));
  protected readonly dots = computed(() =>
    Array.from({ length: Math.max(0, this.total()) }, (_, i) => i < this.used()),
  );
}
