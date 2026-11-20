import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

/**
 * The line of what the sheet is about to use, drawn once the choice is made (E6-07): an icon, a small caption over what was chosen,
 * and an optional action at the end ("Trocar"). The attack sheet's target and the window of a readied action's "Ataque preparado"
 * (#938) draw it. What was chosen is the content; the action is projected with the attribute `action`.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-chosen-row',
  imports: [MatIconModule],
  template: `
    <div class="chosen">
      <mat-icon aria-hidden="true">{{ icon() }}</mat-icon>
      <span class="chosen__text">
        <span class="chosen__cap">{{ caption() }}</span>
        <span><ng-content /></span>
      </span>
      <ng-content select="[action]" />
    </div>
  `,
  styleUrl: './chosen-row.scss',
})
export class ChosenRow {
  readonly icon = input.required<string>();
  readonly caption = input.required<string>();
}
