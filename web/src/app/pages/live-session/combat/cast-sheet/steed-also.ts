import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';

/**
 * "Estrela também" on the cast sheet (#588, board 588-26 card 6; SRD 5.1, Find Steed): a spell that reaches only the caster
 * may reach the bonded steed the caster rides too. The server says when (`SpellOption.also_steed`); the check is on by
 * default and the caster may take it off. A real checkbox in a 44 px row, as "Esculpir Magias" has: Space marks.
 */
@Component({
  selector: 'app-steed-also',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <label class="also">
      <input type="checkbox" [checked]="checked()" (change)="flip.emit()" />
      <span class="also__text">
        <b class="also__t">{{ name() }} também</b>
        <span class="also__d">O corcel que você monta. Um efeito que dura cai nos dois.</span>
      </span>
    </label>
  `,
  styles: `
    :host {
      display: block;
    }

    .also {
      display: flex;
      align-items: flex-start;
      gap: var(--mr-space-2);
      min-height: 44px;
      padding-block: 6px;
      box-sizing: border-box;
    }

    input {
      flex: none;
      width: 20px;
      height: 20px;
      margin: 0;
    }

    .also__text {
      display: flex;
      flex-direction: column;
    }

    .also__t {
      font-size: var(--mr-font-15);
      line-height: 20px;
    }

    .also__d {
      font-size: var(--mr-font-14);
      line-height: 19px;
      color: var(--mr-ink-muted);
    }
  `,
})
export class SteedAlso {
  /** The steed's name, as the combat lists it. */
  readonly name = input.required<string>();
  readonly checked = input(true);
  readonly flip = output<void>();
}
