import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  input,
  model,
  output,
  signal,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type {
  LuckyOffer,
  OutsideInspirationOffer,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { effectivePreference } from '../../../../core/campaigns/dice-labels';
import { treatedSentence } from '../../../../core/combat/combat-dice';
import { luckyText } from '../../../../core/combat/lucky';
import type { ExtraDieField } from '../../../../core/effects/effects';
import type { HintTry } from '../../../../core/puzzles/puzzle-play';
import type { HintDie } from '../../../../core/puzzles/puzzles-client';
import { checkFaces, hasModeInfo } from '../../../../core/play/check-roll';
import type { InspirationRoll, LuckyRoll } from '../../../../core/resources/resources-client';
import { ExtraDice } from '../../effects/extra-dice/extra-dice';
import { InspirationOffer } from '../../inspiration/inspiration-offer';
import { LuckyOfferCard } from '../../inspiration/lucky-offer';
import { MultiRoll, type RollField } from '../../combat/multi-roll/multi-roll';
import { RollPicker } from '../../../../shared/roll-picker/roll-picker';
import { CheckMode } from '../../scene/check-mode/check-mode';

/**
 * "Tentar uma dica · Investigação" on the player's phone (MR-038, RN-27, RN-18, E10-12 states 9 and 9b): a skill check the master set wins
 * the player the next hint, for them alone. The button names the skill and **never the DC** (the DC is the master's: the server does not
 * send it). The d20 is rolled the way the table rolls (RN-18): in the app, or the face of a real die typed (1 to 20, without the bonus: the
 * server adds the character's own). A forced mode shows only its way; "cada jogador escolhe" starts on the player's own choice and keeps the
 * other as a link. A pass says "Você conseguiu." and that the hint is theirs; a fail says "Não deu desta vez." and never how far it was.
 * The button is there only while `canTry` (the server's answer says when the player may try for the next hint). A halfling's
 * d20 of 1 is asked about first ("Tirou 1: rolar de novo?"), then a Bardic Inspiration die the character holds.
 */
@Component({
  selector: 'app-hint-try',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [
    CheckMode,
    ExtraDice,
    InspirationOffer,
    LuckyOfferCard,
    MatButtonModule,
    MatIconModule,
    MultiRoll,
    RollPicker,
  ],
  template: `
    @if (result(); as r) {
      @if (r.passed) {
        <div class="mr-notice mr-notice--success" role="status">
          <mat-icon aria-hidden="true">check</mat-icon>
          <p><strong>Você conseguiu.</strong> Esta dica é só sua; se quiser, conte aos outros.@if (total(r)) { <span class="roll"> Seu total: {{ total(r) }}.</span> }@if (note(r)) { <span class="roll"> {{ note(r) }}</span> }@if (luckyLine(r)) { <span class="roll" data-testid="lucky"> {{ luckyLine(r) }}.</span> }</p>
        </div>
      } @else {
        <div class="mr-notice mr-notice--neutral" role="status">
          <mat-icon aria-hidden="true">close</mat-icon>
          <p>@if (total(r)) {<span class="roll">Você tirou {{ total(r) }}. </span>}<strong>Não deu desta vez.</strong> Outro jogador pode tentar, ou o mestre solta uma dica.@if (note(r)) { <span class="roll"> {{ note(r) }}</span> }@if (luckyLine(r)) { <span class="roll" data-testid="lucky"> {{ luckyLine(r) }}.</span> }</p>
        </div>
      }
    }
    @if (result(); as r) {
      @if (modeInfo(r)) {
        <app-check-mode [mode]="r.mode" [sources]="r.sources" [faces]="faces(r)" />
      }
    }
    @if (luckyOffer(); as l) {
      <!-- The d20 of the try shows a 1 and the character is a halfling: Lucky is asked first (SRD 5.1). -->
      <app-lucky-offer
        [offer]="l"
        [canApp]="canApp()"
        [canType]="canType()"
        [preferApp]="prefersApp()"
        [busy]="busy()"
        (reroll)="luckyAnswered.emit({ reroll: true, die: $event })"
        (keep)="luckyAnswered.emit({ reroll: false, die: null })"
      />
    } @else if (offer(); as o) {
      <!-- The d20 of the try is rolled and kept; nothing is written until the player answers (SRD 5.1, Bardic Inspiration). -->
      <app-inspiration-offer
        [offer]="o"
        [canApp]="canApp()"
        [canType]="canType()"
        [busy]="busy()"
        (use)="answered.emit({ use: true, die: $event })"
        (keep)="answered.emit({ use: false, die: null })"
      />
    } @else if (canTry()) {
      @if (typing() && extraFields().length > 0) {
        <app-extra-dice [fields]="extraFields()" [(faces)]="extraFaces" />
      }
      @if (typing() && pair()) {
        <app-multi-roll
          [fields]="pairFields"
          [canApp]="canApp()"
          [canType]="true"
          [preferApp]="false"
          [hint]="'Teste de ' + skill() + ' com 2 d20. Role os dois dados e digite cada número, sem o bônus.'"
          [busy]="busy()"
          [(typing)]="typing"
          (app)="app()"
          (typed)="onPair($event)"
        />
      } @else if (typing()) {
        <app-roll-picker
          [canApp]="canApp()"
          [canType]="true"
          [preferApp]="false"
          [label]="'O d20 que você rolou'"
          [hint]="'Teste de ' + skill() + '. Role o seu d20 e digite o dado, sem o bônus.'"
          totalNote="O seu d20"
          [busy]="busy()"
          [outlined]="true"
          [compact]="true"
          [hideTotal]="true"
          [(typing)]="typing"
          (app)="app()"
          (typed)="onTyped($event)"
        />
      } @else {
        <button matButton="outlined" type="button" class="go" [disabled]="busy()" disabledInteractive (click)="go()">
          <mat-icon aria-hidden="true">casino</mat-icon>Tentar uma dica · {{ skill() }}
        </button>
        <p class="help">{{ help() }}</p>
        @if (offerType()) {
          <button matButton type="button" class="link" (click)="typing.set(true)">Digitar o resultado</button>
        }
      }
    }
  `,
  styles: `
    :host {
      display: flex;
      flex-direction: column;
      gap: var(--mr-space-3);
    }

    :host:empty {
      display: none;
    }

    .go {
      min-height: 48px;
      width: 100%;
    }

    .help {
      margin: 0;
      font-size: 14px;
      line-height: 20px;
      color: var(--mr-ink-muted);
    }

    .link {
      align-self: flex-start;
      margin-inline-start: -12px;
      min-height: 44px;
      color: var(--mr-accent-text);
    }

    .roll {
      color: var(--mr-ink-muted);
    }
  `,
})
export class HintTryControl {
  /** The skill's name in Portuguese ("Investigação"). Never the DC. */
  readonly skill = input.required<string>();
  /** The server says the player may try now. */
  readonly canTry = input(false);
  readonly busy = input(false);
  readonly diceMode = input<DiceMode>(DiceMode.PLAYERS_CHOOSE);
  readonly dicePreference = input<DicePreference>(DicePreference.APP);
  /** The player's last try, until it changes. */
  readonly result = input<HintTry | null>(null);
  /** The try takes two d20 (advantage or disadvantage): the form for a real die has two fields. */
  readonly pair = input(false);
  /** The d4 an effect adds to a try with a real die (the server said the roll takes them): typed with the d20. */
  readonly extraFields = input<readonly ExtraDieField[]>([]);
  readonly extraFaces = model<readonly (number | null)[]>([]);

  /** A try with the d20 rolled in the app. */
  readonly rolled = output<HintDie>();
  /** The question about the Bardic Inspiration die the try waits on, when the character holds one. */
  readonly offer = input<OutsideInspirationOffer | null>(null);
  /** The player's answer about the die: use it (rolled in the app, or its typed face) or keep it. */
  readonly answered = output<{ readonly use: boolean; readonly die: InspirationRoll | null }>();
  /** The Lucky question the try waits on, when a halfling's d20 shows a 1: it comes before the die's. */
  readonly luckyOffer = input<LuckyOffer | null>(null);
  /** The player's answer to Lucky: roll the d20 again (in the app, or its typed face) or keep the 1. */
  readonly luckyAnswered = output<{ readonly reroll: boolean; readonly die: LuckyRoll | null }>();
  /** A try with the face of a real die. */
  readonly typed = output<HintDie>();

  protected readonly typing = signal(false);
  protected readonly pairFields: readonly RollField[] = [
    { key: 'first', label: 'Primeiro d20', min: 1, max: 20 },
    { key: 'second', label: 'Segundo d20', min: 1, max: 20 },
  ];
  protected readonly canApp = computed(() => this.diceMode() !== DiceMode.PHYSICAL);
  protected readonly canType = computed(() => this.diceMode() !== DiceMode.APP);
  protected readonly prefersApp = computed(
    () => effectivePreference(this.diceMode(), this.dicePreference()) === DicePreference.APP,
  );
  /** "Digitar o resultado" as a link: only when the table lets the player choose and they prefer the app. */
  protected readonly offerType = computed(
    () => this.canApp() && this.canType() && this.prefersApp(),
  );
  protected readonly help = computed(() => {
    if (!this.canType()) {
      return 'A rolagem é no app.';
    }
    if (!this.canApp()) {
      return 'A mesa usa dados físicos: você digita o resultado.';
    }
    return 'A rolagem é no app. Com dado físico, você digita o resultado.';
  });

  constructor() {
    // The server asked for the second die: the fields open at once.
    effect(() => {
      if (this.pair() && this.canTry() && this.canType()) {
        this.typing.set(true);
      }
    });
    // The server asked for the d4 of an effect: the fields open with the d20's.
    effect(() => {
      if (this.extraFields().length > 0 && this.canTry() && this.canType()) {
        this.typing.set(true);
      }
    });
  }

  protected go(): void {
    if (this.canApp() && (!this.canType() || this.prefersApp())) {
      this.rolled.emit({ inApp: true });
    } else {
      this.typing.set(true);
    }
  }

  protected onTyped(face: number): void {
    this.typing.set(this.extraFields().length > 0);
    this.typed.emit({ face });
  }

  protected onPair(faces: number[]): void {
    this.typing.set(this.extraFields().length > 0);
    this.typed.emit({ faces });
  }

  protected faces(r: HintTry) {
    return checkFaces(r.roll);
  }

  protected modeInfo(r: HintTry): boolean {
    return hasModeInfo(r.mode, r.sources, this.faces(r));
  }

  protected app(): void {
    this.typing.set(false);
    this.rolled.emit({ inApp: true });
  }

  protected total(r: HintTry): number {
    return r.roll?.total ?? 0;
  }

  /** "O d20 de 6 contou como 10: perícia com proficiência.", when a feature raised the die (Talento Confiável). */
  protected note(r: HintTry): string {
    return r.roll ? (treatedSentence(r.roll) ?? '') : '';
  }

  /** "Sortudo: 1 → 14", when the halfling's Lucky rolled the d20 again; the total is already the new one. */
  protected luckyLine(r: HintTry): string {
    return luckyText(r.roll);
  }
}
