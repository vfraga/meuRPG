import { ChangeDetectionStrategy, Component, ElementRef, input, viewChild } from '@angular/core';

/**
 * The heading of a step of the area picker ("1 Onde ela explode", "2 Quem está na área"): the number in a ring and the
 * title, which takes the focus when its step opens (`tabindex="-1"`, no ring drawn for it). With `all` (the wall's board:
 * "1 Onde fica", "2 Quem está nela") every step is shown in a row, the current one filled; the current title is still the heading.
 */
@Component({
  selector: 'app-area-step',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (all().length) {
      <div class="row">
        @for (t of all(); track $index) {
          @if ($index + 1 === n()) {
            <h3 #heading class="step step--now" tabindex="-1" [id]="titleId() || null">
              <span class="step__n" aria-hidden="true">{{ n() }}</span>
              <span>{{ title() }}</span>
            </h3>
          } @else {
            <p class="step step--other">
              <span class="step__n" aria-hidden="true">{{ $index + 1 }}</span>
              <span>{{ t }}</span>
            </p>
          }
        }
      </div>
    } @else {
      <h3 #heading class="step" tabindex="-1" [id]="titleId() || null">
        <span class="step__n" aria-hidden="true">{{ n() }}</span>
        <span>{{ title() }}</span>
      </h3>
    }
  `,
  styles: `
    :host {
      display: block;
    }

    .step {
      display: flex;
      align-items: center;
      gap: var(--mr-space-2);
      margin: 0;
      font-size: var(--mr-font-16);
      font-weight: 700;
      line-height: 22px;

      &:focus {
        outline: none;
      }
    }

    .row {
      display: flex;
      flex-wrap: wrap;
      gap: var(--mr-space-2);
    }

    .step--now,
    .step--other {
      box-sizing: border-box;
      min-height: 36px;
      padding: var(--mr-space-1) var(--mr-space-3) var(--mr-space-1) var(--mr-space-2);
      border-radius: var(--mr-radius-pill);
      font-size: var(--mr-font-14);
    }

    .step--now {
      flex: 1 1 0;
      background: var(--mr-accent-soft);
      color: var(--mr-accent-text);
    }

    .step--other {
      flex: 0 1 auto;
      color: var(--mr-ink-muted);
      font-weight: 600;

      .step__n {
        border-color: var(--mr-control-line);
        color: var(--mr-ink-muted);
      }
    }

    .step__n {
      display: inline-flex;
      flex: none;
      align-items: center;
      justify-content: center;
      box-sizing: border-box;
      width: 24px;
      height: 24px;
      border: 1.5px solid var(--mr-accent);
      border-radius: 50%;
      color: var(--mr-accent-text);
      font-size: var(--mr-font-13);
    }
  `,
})
export class AreaStep {
  readonly n = input.required<number>();
  readonly title = input.required<string>();
  readonly titleId = input('');
  /** The titles of all the steps, in order, for a picker that shows them side by side. */
  readonly all = input<readonly string[]>([]);

  private readonly heading = viewChild.required<ElementRef<HTMLElement>>('heading');

  focus(): void {
    this.heading().nativeElement.focus();
  }
}
