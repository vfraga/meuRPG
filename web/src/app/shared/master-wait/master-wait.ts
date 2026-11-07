import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

/**
 * "Esperando o mestre" (RN-10): the card a player reads while something that is the master's holds the table or
 * the player's own attack, whatever it is: an NPC's reaction, a check he rolls, an ask-mode Parry, Legendary
 * Resistance. It says only what the server wrote for the caller (`Encounter.reaction_wait`, "O resultado do seu ataque
 * sai quando ele responder."): never the reaction, the creature that has it or a number, so the same words come for a
 * creature with nothing to react with. A live region, so the wait is read when it arrives.
 */
@Component({
  selector: 'app-master-wait',
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { role: 'status' },
  imports: [MatIconModule],
  template: `
    <div class="wait" data-testid="master-wait">
      <mat-icon aria-hidden="true">hourglass_top</mat-icon>
      <p class="wait__text">
        <strong class="wait__title">{{ title() }}<span class="mr-visually-hidden">{{ '. ' }}</span></strong>
        @if (detail()) {
          <span class="wait__detail">{{ detail() }}</span>
        }
      </p>
    </div>
  `,
  styles: `
    :host {
      display: block;
    }

    .wait {
      display: flex;
      align-items: flex-start;
      gap: var(--mr-space-3);
      box-sizing: border-box;
      padding: 12px 16px;
      border: 1px solid var(--mr-line);
      border-radius: var(--mr-radius-md);
      background: var(--mr-ground);
      color: var(--mr-ink);
    }

    .mat-icon {
      flex: none;
      width: 20px;
      height: 20px;
      margin-top: 1px;
      font-size: var(--mr-font-20);
    }

    .wait__text {
      display: flex;
      flex-direction: column;
      gap: 2px;
      min-width: 0;
      margin: 0;
      font-size: var(--mr-font-16);
      line-height: 22px;
    }

    .wait__title {
      font-weight: 700;
    }

    .wait__detail {
      color: var(--mr-ink-muted);
    }
  `,
})
export class MasterWait {
  /** "Esperando o mestre". */
  readonly title = input.required<string>();
  /** "O resultado do seu ataque sai quando ele responder.", or `''`. */
  readonly detail = input('');
}
