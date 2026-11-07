import { ChangeDetectionStrategy, Component, computed, input, output } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatMenuModule } from '@angular/material/menu';
import { RouterLink } from '@angular/router';

import {
  type Combatant,
  type Encounter,
  EncounterStatus,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { isCreature } from '../../../../core/combat/creature-names';
import { creatureSlug } from '../../../../core/creatures/bestiary-format';
import { isPlayer } from '../../../../core/combat/combat-view';

/**
 * "Mais ações" of the NPC's card (E10-04, #643): the card's other doors in one menu, a flat list with one block per line, so a new
 * line is appended without touching the others. "Oferecer ataque de oportunidade" is the master's for an NPC on turn of a combat
 * under way, on a map or without one; "Ficha da criatura" is the bestiary's page of a monster. When nobody can react the offer
 * stays in the list, off, with its reason under it. Choosing the offer closes the menu and the page opens the panel in the card.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-npc-card-menu',
  imports: [MatButtonModule, MatIconModule, MatMenuModule, RouterLink],
  template: `
    @if (hasItems()) {
      <button
        matButton="outlined"
        type="button"
        class="trigger"
        [attr.aria-label]="'Mais ações de ' + subject().label"
        [matMenuTriggerFor]="menu"
      >
        <mat-icon aria-hidden="true">more_vert</mat-icon>Mais ações
      </button>
      <mat-menu #menu="matMenu" xPosition="before" (closed)="closed()">
        @if (offerOn()) {
          <button
            mat-menu-item
            type="button"
            [attr.aria-describedby]="offerWhy() ? 'offer-why' : null"
            [disabled]="!!offerWhy()"
            disabledInteractive
            (click)="choose()"
          >
            <mat-icon aria-hidden="true">swords</mat-icon>
            <span class="item">
              Oferecer ataque de oportunidade
              @if (offerWhy()) {
                <small class="item__why" id="offer-why">{{ offerWhy() }}</small>
              }
            </span>
          </button>
        }
        @if (slug(); as s) {
          <a mat-menu-item [routerLink]="['/campaigns', campaignId(), 'bestiary', s]">
            <mat-icon aria-hidden="true">open_in_new</mat-icon>Ficha da criatura
          </a>
        }
      </mat-menu>
    }
  `,
  styleUrl: './npc-card-menu.scss',
})
export class NpcCardMenu {
  readonly campaignId = input.required<string>();
  readonly encounter = input.required<Encounter>();
  /** The one on turn. */
  readonly subject = input.required<Combatant>();
  /** Why the offer cannot be made now ("Ninguém pode reagir agora."), or `''`. */
  readonly offerWhy = input('');
  /** The offer was chosen and the menu is closed: the page opens the panel, or keeps it open and gives it the focus. */
  readonly offer = output<void>();

  /** The offer row was chosen: the page is told once the menu has closed and given the focus back, so the panel is the one to take it. */
  private chose = false;
  protected readonly offerOn = computed(
    () =>
      !isPlayer(this.subject()) &&
      !isCreature(this.subject()) &&
      this.encounter().status === EncounterStatus.ACTIVE,
  );
  /** The creature's route segment when the subject is a monster of the bestiary ("bandit"); the key only reaches the master (RN-29). */
  protected readonly slug = computed(() => creatureSlug(this.subject().bestiaryCreatureKey ?? ''));

  /** Each line of the menu has its own condition; a new line adds its own here (`|| …`) and its own block in the template. */
  protected readonly hasItems = computed(() => this.offerOn() || !!this.slug());

  protected choose(): void {
    if (!this.offerWhy()) {
      this.chose = true;
    }
  }

  protected closed(): void {
    if (this.chose) {
      this.chose = false;
      this.offer.emit();
    }
  }
}
