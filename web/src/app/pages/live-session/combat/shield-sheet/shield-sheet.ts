import { Component, ElementRef, computed, effect, inject, signal, viewChild } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { ReactionOutcome, type ReactionPrompt } from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  type SlotRow,
  defaultSlot,
  freeText,
  lastSlotWarning,
  slotRows,
} from '../../../../core/combat/cast-flow';
import { circleLabel } from '../../../../core/combat/combat-grid';
import { joinDots } from '../../../../core/format/text';
import { article } from '../../../../core/combat/combat-log';
import { CombatClient } from '../../../../core/combat/combat-client';
import { ActionKey } from '../../../../core/connect/idempotency';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { SlotPicker } from '../cast-sheet/slot-picker';
import { SheetFrame } from '../sheet-frame/sheet-frame';
import { injectSheet } from '../sheet-host';

/** What the page hands the Escudo sheet. */
export interface ShieldSheetData {
  readonly campaignId: string;
  readonly encounterId: string;
  readonly prompt: ReactionPrompt;
  readonly round: number;
  readonly usage: readonly {
    readonly level: number;
    readonly total: number;
    readonly used: number;
  }[];
  readonly pact: {
    readonly slotLevel: number;
    readonly total: number;
    readonly used: number;
  } | null;
  readonly state: CombatState;
  /** The player's armor class from their own sheet, for "Sua CA é 18"; `null` when unknown. */
  readonly armorClass: number | null;
}

/**
 * "Você foi atingido: usar Escudo?" (MR-014, E6-28): a hit on the player's
 * character that is not critical waits for their reaction, and the combat page
 * opens this alert dialog by itself (`Encounter.reaction_prompts`). It says what
 * Escudo does (+5 on the armor class until the start of the next turn, the hit
 * may turn into a miss, the reaction and a slot are spent), lists the slots as
 * radios (the lowest free one is chosen) with the last-slot warning, and has two
 * buttons of the same size: "Não usar" (focus starts there, so a stray Enter
 * lets the hit go) and "Conjurar Escudo". The player decides without the total or
 * the armor class, as at a table; the answer says only whether it stopped the
 * hit. The master may answer for the player (their card): then the prompt is
 * gone and this says so. It cannot be closed without an answer. The keys are
 * made once, so a repeated tap never casts twice.
 */
@Component({
  selector: 'app-shield-sheet',
  imports: [MatButtonModule, MatIconModule, SheetFrame, SlotPicker],
  template: `
    <app-sheet-frame
      [title]="title()"
      [subtitle]="subtitle()"
      icon="shield"
      [phone]="inSheet"
      [closable]="false"
    >
      @if (error()) {
        <div class="mr-notice mr-notice--danger" role="alert">
          <mat-icon aria-hidden="true">error</mat-icon>
          <p>{{ error() }}</p>
        </div>
      }
      @switch (stage()) {
        @case ('ask') {
          <p class="what">
            Usar {{ name }}? A sua CA sobe 5 até o começo do seu próximo turno, e o ataque pode virar erro. Gasta a sua
            reação e um espaço de magia.
          </p>
          <app-slot-picker [rows]="rows()" [chosen]="slot()" (pick)="slot.set($event); error.set('')" />
          @if (warning()) {
            <div class="mr-notice mr-notice--warning" role="status">
              <mat-icon aria-hidden="true">info</mat-icon>
              <p><strong>{{ warning() }}</strong></p>
            </div>
          }
        }
        @case ('gone') {
          <p class="what" role="status">O mestre respondeu por você: este acerto já não espera a sua reação.</p>
        }
        @default {
          <div class="res" role="status" aria-live="polite">
            <span class="pill" [class.pill--bad]="outcome() === 'still'">
              <mat-icon aria-hidden="true">{{ outcome() === 'still' ? 'close' : 'check' }}</mat-icon>{{ outcome() === 'still' ? 'Acertou' : 'Errou' }}
            </span>
            @if (outcome() === 'still') {
              <p class="what">Mesmo com o {{ name }}, o ataque acertou. O dano segue para o mestre.</p>
            } @else {
              <p class="what"><strong>O {{ name }} segurou o ataque{{ byWhom() }}.</strong></p>
            }
            <p class="what">{{ ac() }}</p>
            <p class="small">{{ afterText() }}</p>
          </div>
        }
      }
      <div foot>
        @if (stage() === 'ask') {
          <div class="pair">
            <button #no mat-stroked-button type="button" class="pair__btn" data-initial-focus [disabled]="busy()" (click)="decline()">
              Não usar
            </button>
            <button
              mat-flat-button
              type="button"
              class="pair__btn"
              [disabled]="busy() || !slot()"
              disabledInteractive
              (click)="use()"
            >
              Conjurar {{ name }}
            </button>
          </div>
        } @else {
          <button #close mat-stroked-button type="button" class="pair__btn pair__btn--one" data-initial-focus (click)="done()">
            Fechar
          </button>
        }
      </div>
    </app-sheet-frame>
  `,
  styleUrl: './shield-sheet.scss',
})
export class ShieldSheet {
  private readonly api = inject(CombatClient);
  private readonly sheet = injectSheet<ShieldSheetData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;

  protected readonly rows = computed(() =>
    slotRows(1, this.data.prompt.slots, this.data.usage, this.data.pact),
  );
  protected readonly slot = signal<SlotRow | null>(defaultSlot(this.rows()));
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');
  protected readonly outcome = signal<'stopped' | 'still' | null>(null);
  protected readonly decided = signal(false);
  protected readonly warning = computed(() => lastSlotWarning(this.slot(), null));
  protected readonly name = this.data.prompt.spellNamePt || 'Escudo Arcano';
  /** "Capitão Goblin · Cimitarra · Rodada 2"; just "Rodada 2" when the attacker is hidden. */
  protected readonly subtitle = computed(() => {
    if (this.stage() === 'result') {
      return '';
    }
    const p = this.data.prompt;
    return p.attackerLabel
      ? joinDots([
          p.attackerLabel,
          ...(p.attackNamePt ? [p.attackNamePt] : []),
          `Rodada ${this.data.round}`,
        ])
      : `Rodada ${this.data.round}`;
  });
  /** " do Capitão Goblin" after "segurou o ataque"; empty when the attacker is hidden. */
  protected readonly byWhom = computed(() => {
    const who = this.data.prompt.attackerLabel;
    return who ? ` ${article(who) === 'a' ? 'da' : 'do'} ${who}` : '';
  });
  /** "Sua CA é 18 até o começo do seu próximo turno." (their own armor class and the +5). */
  protected readonly ac = computed(() =>
    this.data.armorClass === null
      ? 'Sua CA sobe 5 até o começo do seu próximo turno.'
      : `Sua CA é ${this.data.armorClass + 5} até o começo do seu próximo turno.`,
  );
  protected readonly afterText = computed(() => `Sua reação foi usada · ${this.after()}`);
  /** What the sheet shows: the question, the answer, or that the master answered first. */
  protected readonly stage = computed<'ask' | 'result' | 'gone'>(() => {
    if (this.outcome() !== null) {
      return 'result';
    }
    const waiting = this.data.state
      .encounter()
      ?.reactionPrompts.some((p) => p.pendingDamageId === this.data.prompt.pendingDamageId);
    return waiting || this.busy() ? 'ask' : 'gone';
  });
  protected readonly title = computed(() =>
    this.stage() === 'result' ? `${this.name} conjurado` : 'Você foi atingido',
  );
  protected readonly after = computed(() => {
    const s = this.slot();
    return s
      ? `Espaços de ${circleLabel(s.level)}: ${freeText(Math.max(0, s.free - 1), s.total)}`
      : '';
  });
  /** One key per answer ("Conjurar" with its slot, or "Não usar"): a repeated tap is a retry, the other answer is a new request. */
  private readonly keys = new ActionKey();
  private readonly focus = viewChild('close', { read: ElementRef<HTMLButtonElement> });

  constructor() {
    effect(() => this.focus()?.nativeElement.focus());
  }

  protected async use(): Promise<void> {
    const s = this.slot();
    if (!s || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.useReaction(
        this.data.campaignId,
        this.data.encounterId,
        this.data.prompt.pendingDamageId,
        { level: s.level, pact: s.pact },
        this.keys.keyFor({ use: [s.level, s.pact] }),
      );
      this.data.state.apply(res.encounter);
      this.outcome.set(res.outcome === ReactionOutcome.STOPPED ? 'stopped' : 'still');
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'conjurar o Escudo'));
    } finally {
      this.busy.set(false);
    }
  }

  protected async decline(): Promise<void> {
    if (this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      this.data.state.apply(
        await this.api.declineReaction(
          this.data.campaignId,
          this.data.encounterId,
          this.data.prompt.pendingDamageId,
          this.keys.keyFor('decline'),
        ),
      );
      this.sheet.close(false);
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'deixar o ataque passar'));
    } finally {
      this.busy.set(false);
    }
  }

  protected done(): void {
    this.sheet.close(this.outcome() !== null);
  }
}
