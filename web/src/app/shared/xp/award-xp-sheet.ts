import { Component, computed, effect, ElementRef, inject, signal, viewChild } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import {
  AbstractControl,
  FormControl,
  ReactiveFormsModule,
  ValidationErrors,
  Validators,
} from '@angular/forms';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { startWith } from 'rxjs';

import { XpMode } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  type TreasureToConvert,
  type XPAward,
  XPBlockedReason,
} from '../../../gen/meurpg/progression/v1/progression_pb';
import { ActionKey } from '../../core/connect/idempotency';
import { formatInt, tight } from '../../core/format/text';
import type { ExperienceRow } from '../../core/progression/experience-store';
import { ProgressionClient } from '../../core/progression/progression-client';
import { xpBlocked, xpErrorMessage } from '../../core/progression/xp-errors';
import { parseAmount, shortDivision, splitXp } from '../../core/progression/xp-math';
import { SheetFrame } from '../../pages/live-session/combat/sheet-frame/sheet-frame';
import { injectSheet } from '../../pages/live-session/combat/sheet-host';
import { TreasureStrip } from './treasure-strip';
import { XpActions } from './xp-actions';
import { XpReason } from './xp-reason';
import { type Recipient, XpRecipients } from './xp-recipients';
import { XpSplit } from './xp-split';

/** What the caller hands "Dar XP". The reason, the amount and the combat are
 * filled when it opens from the end-of-combat block ("Agora não" left the
 * combat's XP for later). */
export interface AwardXpData {
  readonly campaignId: string;
  /** How the campaign levels: ENEMIES (by enemies or any amount) or GOLD. */
  readonly xpMode: XpMode;
  /** The living player characters, with their XP now. */
  readonly rows: readonly ExperienceRow[];
  readonly reason?: string;
  readonly amount?: number;
  /** The ended combat the amount came from: sent as an enemies award while
   * the amount is still the combat's total, as an avulso one when edited. */
  readonly encounterId?: string;
  /** By gold: the treasures waiting for "Voltar à cidade", as the host knows
   * them (the sheet reads them again as it opens). */
  readonly treasures?: readonly TreasureToConvert[];
  readonly treasuresTotal?: number;
}

/** What the sheet closes with when the master chose "Voltar à cidade" in its
 * strip: the host closes this one and opens that one (a phone has room for one
 * sheet at a time). */
export interface TownRequest {
  readonly town: true;
}

/** What the sheet closes with when the XP was given. */
export interface AwardXpResult {
  readonly award: XPAward;
  readonly xpEach: number;
  readonly lostXp: number;
}

const filled = (control: AbstractControl): ValidationErrors | null =>
  String(control.value ?? '').trim() ? null : { required: true };
const wholeAmount = (control: AbstractControl): ValidationErrors | null =>
  parseAmount(String(control.value ?? '')) === null ? { amount: true } : null;

/**
 * "Dar XP" at any time (E7-07, MR-016): a dialog on a desktop and a bottom
 * sheet on a phone (`openSheet`), in the shared `sheet-frame`, so the title and
 * the fixed footer (the live division above "Dar 50 XP a cada um") are always
 * in reach, even at 320×568.
 *
 * - **By enemies / any amount** (the campaign counts defeated enemies): the
 *   reason and the XP of the group. **By gold:** the reason and the gold
 *   pieces the group found (1 XP each).
 * - Everyone alive is checked; unchecking changes the division on the spot.
 * - A wrong field says so when the person leaves it, not on each key; the
 *   filled button waits (dashed) and, pressed, goes to the first wrong field.
 * - One idempotency key per set of values: a retry after a lost answer repeats
 *   it, a changed value is a new action.
 */
@Component({
  selector: 'app-award-xp-sheet',
  imports: [
    MatFormFieldModule,
    MatIconModule,
    MatInputModule,
    ReactiveFormsModule,
    SheetFrame,
    TreasureStrip,
    XpActions,
    XpReason,
    XpRecipients,
    XpSplit,
  ],
  templateUrl: './award-xp-sheet.html',
  styleUrl: './award-xp-sheet.scss',
})
export class AwardXpSheet {
  private readonly api = inject(ProgressionClient);
  private readonly sheet = injectSheet<AwardXpData, AwardXpResult | TownRequest | undefined>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;

  protected readonly gold = this.data.xpMode === XpMode.GOLD;
  // The phone's sheet is short: one line says what the campaign counts, the rest is in the help under the fields.
  protected readonly subtitle = this.gold
    ? this.inSheet
      ? 'Campanha por ouro: 1 XP por PO.'
      : 'Campanha por ouro: 1 XP por peça de ouro (PO). O XP vale a qualquer hora.'
    : this.inSheet
      ? 'Campanha por inimigos.'
      : 'Campanha por inimigos. O XP vale a qualquer hora, dentro ou fora de um combate.';

  protected readonly reason = new FormControl(this.data.reason ?? '', {
    nonNullable: true,
    validators: [filled, Validators.maxLength(120)],
  });
  protected readonly amount = new FormControl(this.data.amount ? String(this.data.amount) : '', {
    nonNullable: true,
    validators: [wholeAmount],
  });
  private readonly amountText = toSignal(
    this.amount.valueChanges.pipe(startWith(this.amount.value)),
    {
      initialValue: this.amount.value,
    },
  );
  private readonly reasonText = toSignal(
    this.reason.valueChanges.pipe(startWith(this.reason.value)),
    {
      initialValue: this.reason.value,
    },
  );

  /** Who is alive: a character the server says cannot receive leaves the list. */
  protected readonly rows = signal<readonly ExperienceRow[]>(this.data.rows);
  protected readonly checked = signal<ReadonlySet<string>>(
    new Set(this.data.rows.map((r) => r.id)),
  );
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');

  private readonly reasonField = viewChild.required(XpReason);
  private readonly frame = viewChild.required(SheetFrame);
  private readonly amountField = viewChild.required<ElementRef<HTMLInputElement>>('amountInput');

  /** The XP being split: the typed amount, or the gold (1 XP per piece). */
  protected readonly total = computed(() => parseAmount(this.amountText()) ?? 0);
  protected readonly split = computed(() => splitXp(this.total(), this.checked().size));
  protected readonly amountError = tight(
    this.gold ? 'Digite as PO de 1 a 1.000.000.' : 'Digite um valor de 1 a 1.000.000.',
  );
  protected readonly amountLabel = this.gold ? 'Ouro encontrado (PO)' : 'XP para o grupo';
  protected readonly amountHint = computed(() => {
    if (!this.gold) {
      return 'O total é dividido entre quem você marcar, arredondando para baixo.';
    }
    const gold = this.total();
    return gold > 0
      ? tight(`Vale 1 XP por PO: ${formatInt(gold)} PO são ${formatInt(gold)} XP.`)
      : 'Vale 1 XP por peça de ouro (PO).';
  });

  /** The sum, for the body of a short screen (the footer shows only the number there). */
  protected readonly sumNote = computed(() =>
    this.split().count === 0
      ? ''
      : shortDivision(this.total(), this.split(), this.gold ? this.total() : 0),
  );

  protected readonly recipients = computed<Recipient[]>(() => {
    const each = this.split().each;
    return this.rows().map((r) => {
      const on = this.checked().has(r.id);
      return {
        id: r.id,
        name: r.name,
        sub: r.sub,
        checked: on,
        amount: on ? (this.total() > 0 ? tight(`+${formatInt(each)} XP`) : '') : 'Não recebe',
      };
    });
  });

  /** Why the filled button waits, in words, or empty when the fields are what is missing. */
  protected readonly reasonToWait = computed(() => {
    if (this.checked().size === 0) {
      return 'Marque pelo menos um personagem';
    }
    if (this.total() > 0 && this.split().each === 0) {
      return 'O total é pequeno demais: cada um precisa receber pelo menos 1 XP.';
    }
    return '';
  });
  protected readonly blocked = computed(
    () => this.reasonToWait() !== '' || this.total() === 0 || this.reasonText().trim() === '',
  );
  protected readonly primaryLabel = computed(() =>
    !this.blocked() ? tight(`Dar ${formatInt(this.split().each)} XP a cada um`) : 'Dar XP',
  );

  /** By gold: what waits for "Voltar à cidade", the host's list and then the server's. */
  protected readonly treasures = signal<readonly TreasureToConvert[]>(this.data.treasures ?? []);
  protected readonly treasuresTotal = signal(this.data.treasuresTotal ?? 0);
  protected readonly treasuresState = signal<'loading' | 'ready' | 'error'>(
    this.data.treasures ? 'ready' : 'loading',
  );

  private readonly key = new ActionKey();

  constructor() {
    // The strip is only there for a gold campaign's own "Dar XP" (a combat's has none): no read for it otherwise.
    if (this.gold && !this.data.encounterId) {
      void this.readTreasures();
    }
  }

  protected async readTreasures(): Promise<void> {
    try {
      const res = await this.api.listTreasures(this.data.campaignId);
      this.treasures.set(res.treasures);
      this.treasuresTotal.set(res.total);
      this.treasuresState.set('ready');
    } catch {
      // The host's list stays when there is one (even an empty one that was read); with none, say it could not read.
      if (!this.data.treasures) {
        this.treasuresState.set('error');
      }
    }
  }

  protected retryTreasures(): void {
    this.treasuresState.set('loading');
    void this.readTreasures();
  }

  protected goTown(): void {
    this.sheet.close({ town: true });
  }

  protected toggle(id: string): void {
    this.checked.update((set) => {
      const next = new Set(set);
      if (!next.delete(id)) {
        next.add(id);
      }
      return next;
    });
  }

  protected async give(): Promise<void> {
    if (this.busy()) {
      return;
    }
    // Wrong or missing fields: show every error and go to the first one.
    if (this.reason.invalid || this.amount.invalid) {
      this.reason.markAsTouched();
      this.amount.markAsTouched();
      if (this.reason.invalid) {
        this.reasonField().focus();
      } else {
        this.amountField().nativeElement.focus();
      }
      return;
    }
    if (this.blocked()) {
      return;
    }
    const reason = this.reason.value.trim();
    const total = this.total();
    const ids = this.rows()
      .filter((r) => this.checked().has(r.id))
      .map((r) => r.id);
    const fromCombat =
      !this.gold && !!this.data.encounterId && total === this.data.amount
        ? this.data.encounterId
        : '';
    const input = this.gold
      ? ({ mode: 'gold', gold: total } as const)
      : fromCombat
        ? ({ mode: 'enemies', encounterId: fromCombat } as const)
        : ({ mode: 'manual', amount: total } as const);

    // New values are a new award; the same values again are a retry.
    const key = this.key.keyFor([input, reason, ids]);
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.award(this.data.campaignId, input, reason, ids, key);
      if (res.award) {
        this.sheet.close({ award: res.award, xpEach: res.xpEach, lostXp: res.lostXp });
      } else {
        this.sheet.close(undefined);
      }
    } catch (err) {
      this.error.set(xpErrorMessage(err, 'dar o XP'));
      // A character that cannot receive (died, left) leaves the list, so the retry can go.
      const blocked = xpBlocked(err);
      if (
        blocked?.reason === XPBlockedReason.XP_BLOCKED_REASON_CHARACTER_NOT_ELIGIBLE &&
        blocked.characterId
      ) {
        this.rows.update((rows) => rows.filter((r) => r.id !== blocked.characterId));
        this.checked.update((set) => {
          const next = new Set(set);
          next.delete(blocked.characterId);
          return next;
        });
      }
      this.frame().scrollToTop();
    } finally {
      this.busy.set(false);
    }
  }

  protected cancel(): void {
    this.sheet.close(undefined);
  }
}
