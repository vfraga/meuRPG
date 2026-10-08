import {
  afterNextRender,
  Component,
  computed,
  effect,
  ElementRef,
  inject,
  Injector,
  signal,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type {
  GetTurnOptionsResponse,
  OpportunityOffer,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { ownCombatant } from '../../../../core/combat/combat-view';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import {
  type ReactorAttack,
  attackLabel,
  playerQuestion,
  spendText,
} from '../../../../core/combat/opportunity';
import { tieNumbers } from '../../../../core/format/text';
import { SheetFrame } from '../sheet-frame/sheet-frame';
import { injectSheet } from '../sheet-host';

/** What the page hands the opportunity prompt. */
export interface OpportunitySheetData {
  readonly campaignId: string;
  readonly encounterId: string;
  readonly offer: OpportunityOffer;
  readonly round: number;
  /** Reads the reactor's melee attacks with their numbers (its own `GetTurnOptions`) and the options they came from. */
  readonly load: () => Promise<{
    readonly attacks: readonly ReactorAttack[];
    readonly options: GetTurnOptionsResponse;
  }>;
  readonly state: CombatState;
}

/** What the prompt closes with: the attack the player chose (with the options it comes from), or nothing for "Não atacar". */
export type OpportunityAnswer = {
  readonly attackKey: string;
  readonly options: GetTurnOptionsResponse;
} | null;

/**
 * "O Goblin 2 está saindo do seu alcance. Ataque de oportunidade?" (E9-13): the
 * player's `alertdialog`, like Escudo's. The server detected the exit and made
 * the offer (`Encounter.opportunity_offers`); the page opens this by itself and it
 * cannot be closed without an answer. Focus starts on the safe "Não atacar" (with
 * its ring: it is a keyboard-style focus); the answers are stacked at the same width,
 * the first weapon's "Atacar com <arma>" the one filled button, with a line of numbers per
 * weapon above them ("Espada longa +5 · 1d8 + 3 cortante"). The weapons are read when the
 * prompt opens: until they are in the attack buttons are off and say why, and if the read fails
 * the prompt says so and offers "Tentar de novo" ("Não atacar" always works, so the offer is
 * never stuck). A chosen attack closes the prompt and hands over to the attack sheet, with
 * the mover as its target. It says what is spent (the reaction), never the target's armor
 * class. If the master answered or skipped meanwhile, it says so and only closes.
 */
@Component({
  selector: 'app-opportunity-sheet',
  imports: [MatButtonModule, MatIconModule, SheetFrame],
  template: `
    <app-sheet-frame title="Ataque de oportunidade" [subtitle]="subtitle()" icon="swords" [phone]="inSheet" [closable]="false">
      @if (error()) {
        <div class="mr-notice mr-notice--danger" role="alert">
          <mat-icon aria-hidden="true">error</mat-icon>
          <p>{{ error() }}</p>
        </div>
      }
      @if (gone()) {
        <p class="what" role="status">O mestre respondeu por você ou retirou a oferta: esse ataque de oportunidade não espera mais a sua resposta.</p>
      } @else {
        <p class="what">{{ question() }}</p>
        <p class="small">{{ spend() }}</p>
        @if (attacks(); as list) {
          <ul class="weapons" aria-label="Seus ataques corpo a corpo">
            @for (a of list; track a.key) {
              <li><b>{{ a.name }}</b> {{ a.numbers }}</li>
            }
          </ul>
        } @else if (readFailed()) {
          <p class="small" role="status">Não deu para ler os seus ataques. Dá para tentar de novo, ou não atacar.</p>
        } @else {
          <p class="small" role="status">Lendo os seus ataques…</p>
        }
      }
      <div foot>
        @if (gone()) {
          <button mat-stroked-button type="button" class="btn" data-initial-focus (click)="sheet.close(null)">Fechar</button>
        } @else {
          <div class="stack">
            <button #safe mat-stroked-button type="button" class="btn" data-initial-focus [disabled]="busy()" (click)="decline()">
              Não atacar
            </button>
            @if (attacks(); as list) {
              @for (a of list; track a.key; let first = $first) {
                @if (first) {
                  <button mat-flat-button type="button" class="btn" [disabled]="busy()" (click)="attack(a)">{{ label(a) }}</button>
                } @else {
                  <button mat-stroked-button type="button" class="btn" [disabled]="busy()" (click)="attack(a)">{{ label(a) }}</button>
                }
              }
            } @else if (readFailed()) {
              <button mat-stroked-button type="button" class="btn" (click)="read()">Tentar de novo</button>
            } @else {
              <button mat-flat-button type="button" class="btn btn--off" disabled disabledInteractive>Atacar</button>
            }
          </div>
        }
      </div>
    </app-sheet-frame>
  `,
  styleUrl: './opportunity-sheet.scss',
})
export class OpportunitySheet {
  private readonly api = inject(CombatClient);
  private readonly injector = inject(Injector);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  protected readonly sheet = injectSheet<OpportunitySheetData, OpportunityAnswer>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');
  /** The attacks, once read; `null` while they come or when the read failed. */
  protected readonly attacks = signal<readonly (ReactorAttack & { numbers: string })[] | null>(
    null,
  );
  protected readonly readFailed = signal(false);
  private options: GetTurnOptionsResponse | null = null;

  protected readonly subtitle = computed(() =>
    tieNumbers(`${this.data.offer.moverLabel} · Rodada ${this.data.round}`),
  );
  /** The reactor is the player's character (not one of their creatures): the page's own combatant is the reactor. */
  private readonly charReacts = computed(() => {
    const e = this.data.state.encounter();
    const own = e ? ownCombatant(e) : null;
    return !own || own.id === this.data.offer.reactorId;
  });
  protected readonly question = computed(() => playerQuestion(this.data.offer, this.charReacts()));
  protected readonly spend = computed(() => spendText(this.data.offer, this.charReacts()));
  /** The offer is no longer in the combat: the master (or the turn) answered it. */
  protected readonly gone = computed(
    () =>
      !this.busy() &&
      !(this.data.state.encounter()?.opportunityOffers ?? []).some(
        (o) => o.id === this.data.offer.id,
      ),
  );

  constructor() {
    void this.read();
    // The safe answer has the focus, and its ring: the sheet opens for a keyboard-style answer.
    afterNextRender(
      () =>
        (
          this.host.nativeElement.querySelector('[data-initial-focus]') as HTMLButtonElement | null
        )?.focus({
          focusVisible: true,
        } as FocusOptions),
      { injector: this.injector },
    );
  }

  protected async read(): Promise<void> {
    this.readFailed.set(false);
    try {
      const { attacks, options } = await this.data.load();
      this.options = options;
      // The numbers after the name: "+5 · 1d8 + 3 cortante".
      this.attacks.set(attacks.map((a) => ({ ...a, numbers: a.detail.replace(`${a.name} `, '') })));
    } catch {
      this.readFailed.set(true);
    }
  }

  protected label(a: ReactorAttack): string {
    return attackLabel(a);
  }

  protected async decline(): Promise<void> {
    if (this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      this.data.state.apply(
        await this.api.declineOpportunity(
          this.data.campaignId,
          this.data.encounterId,
          this.data.offer.id,
        ),
      );
      this.sheet.close(null);
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'responder'));
    } finally {
      this.busy.set(false);
    }
  }

  protected attack(a: ReactorAttack): void {
    if (this.options) {
      this.sheet.close({ attackKey: a.key, options: this.options });
    }
  }
}
