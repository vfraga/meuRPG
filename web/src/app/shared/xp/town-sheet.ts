import { Component, computed, effect, inject, signal, viewChild } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { XpMode } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  type TreasureToConvert,
  XPBlockedReason,
} from '../../../gen/meurpg/progression/v1/progression_pb';
import { ActionKey } from '../../core/connect/idempotency';
import { joinDots } from '../../core/format/text';
import type { ExperienceRow, LoadState } from '../../core/progression/experience-store';
import { ProgressionClient } from '../../core/progression/progression-client';
import { foundLine, moreTreasures, po, totalPo, townCalc } from '../../core/progression/treasure';
import { townErrorMessage, xpBlocked, xpNotFound } from '../../core/progression/xp-errors';
import { nameList } from '../../core/progression/xp-labels';
import { eachLine } from '../../core/progression/xp-math';
import { SheetFrame } from '../../pages/live-session/combat/sheet-frame/sheet-frame';
import { injectSheet } from '../../pages/live-session/combat/sheet-host';
import { CheckBox } from '../check-box/check-box';
import type { AwardXpResult } from './award-xp-sheet';
import { XpActions } from './xp-actions';

/** What the caller hands "Voltar à cidade": who is alive and the treasures it
 * already knows. `treasures` is `undefined` while the host has not read them
 * (the sheet reads them as it opens and says "lendo" until then). */
export interface TownData {
  readonly campaignId: string;
  readonly xpMode: XpMode;
  readonly rows: readonly ExperienceRow[];
  readonly treasures?: readonly TreasureToConvert[];
  readonly total?: number;
}

/** What the award is called in the history when the master does not write a
 * reason (the dialog has no field for it): the title line is made from the
 * treasures, so this is only what the server stores. */
const TOWN_REASON = 'Voltar à cidade';

/**
 * "Voltar à cidade" (E9-09, MR-041, RN-09, RN-10): converts the found, not
 * converted treasures into one XP award of a campaign by gold, 1 XP per PO.
 * A dialog on a desktop and a bottom sheet on a phone (`openSheet`), in the
 * shared `sheet-frame`: the body (the treasures, who receives) scrolls with a
 * shadow where there is more, and the footer, which never scrolls away, holds
 * the live calculation, who receives, and the one filled button with the number
 * ("Dar 105 XP para cada").
 *
 * - Every treasure and every living character comes checked; unchecking
 *   changes the calculation, written out and read by a polite status.
 * - The server sums the PO and splits; the line here is only the preview.
 * - The list is "lendo" until it is known, "nenhum" only after a read that
 *   worked and found none, and an error with "Tentar de novo" when it could not
 *   be read.
 * - A refusal stays in the sheet, in words, and the list is read again: a
 *   treasure that was converted, unmarked or deleted meanwhile leaves it, a
 *   character that cannot receive leaves "Quem recebe", the rest of the choice stays.
 * - One idempotency key per set of choices, as "Dar XP".
 */
@Component({
  selector: 'app-town-sheet',
  imports: [CheckBox, MatButtonModule, MatIconModule, SheetFrame, XpActions],
  templateUrl: './town-sheet.html',
  styleUrl: './town-sheet.scss',
})
export class TownSheet {
  private readonly api = inject(ProgressionClient);
  private readonly sheet = injectSheet<TownData, AwardXpResult | undefined>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;

  protected readonly state = signal<LoadState>(this.data.treasures ? 'ready' : 'loading');
  protected readonly treasures = signal<readonly TreasureToConvert[]>(this.data.treasures ?? []);
  private readonly totalFound = signal(this.data.total ?? this.data.treasures?.length ?? 0);
  /** Who is alive: a character the server says cannot receive leaves it. */
  protected readonly people = signal<readonly ExperienceRow[]>(this.data.rows);
  private known = new Set(this.treasures().map((t) => t.pointId));
  protected readonly checkedTreasures = signal<ReadonlySet<string>>(new Set(this.known));
  protected readonly checkedPeople = signal<ReadonlySet<string>>(
    new Set(this.data.rows.map((r) => r.id)),
  );
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');
  private readonly frame = viewChild.required(SheetFrame);

  protected readonly chosen = computed(() =>
    this.treasures().filter((t) => this.checkedTreasures().has(t.pointId)),
  );
  protected readonly receivers = computed(() =>
    this.people().filter((r) => this.checkedPeople().has(r.id)),
  );
  protected readonly calc = computed(() =>
    townCalc(this.chosen().length, totalPo(this.chosen()), this.receivers().length),
  );
  /** Treasures the list does not hold (it stops at 100). */
  protected readonly more = computed(() =>
    Math.max(0, this.totalFound() - this.treasures().length),
  );
  protected readonly moreText = computed(() => moreTreasures(this.more(), 'para a próxima vez'));
  /** One line above the list when a find was made outside a session (the rows carry a short tag). */
  protected readonly anyOutside = computed(() => this.treasures().some((t) => !t.foundInSession));
  /** "Para 3: Pensantus, Toren e Brisa": who receives, always in sight in the footer. */
  protected readonly toLine = computed(() => {
    const names = this.receivers().map((r) => r.name);
    return names.length === 0 ? '' : `Para ${names.length}: ${nameList(names)}`;
  });

  /** Why the filled button waits, in words, or empty when it can go. */
  protected readonly waiting = computed(() => {
    if (this.chosen().length === 0 || this.receivers().length === 0) {
      return 'Marque pelo menos um tesouro e um personagem.';
    }
    if (this.calc().split.each === 0) {
      return 'O total é pequeno demais: cada um precisa receber pelo menos 1 XP.';
    }
    return '';
  });
  protected readonly primaryLabel = computed(() =>
    this.waiting() ? 'Dar XP' : `Dar ${eachLine(this.calc().split)}`,
  );
  protected readonly announcement = computed(
    () => this.waiting() || `${eachLine(this.calc().split)}.`,
  );
  /** Nothing to convert (after a read that worked) or nothing known yet: no filled button. */
  protected readonly noList = computed(
    () => this.state() !== 'ready' || this.treasures().length === 0,
  );

  protected readonly sub = (t: TreasureToConvert) =>
    joinDots([foundLine(t), ...(t.foundInSession ? [] : ['fora de uma sessão'])]);
  protected readonly po = po;

  private readonly key = new ActionKey();

  constructor() {
    // The master may have marked or converted treasures since the host read them (or not read them yet).
    void this.reload();
  }

  protected toggleTreasure(id: string): void {
    this.checkedTreasures.update((set) => flip(set, id));
  }

  protected togglePerson(id: string): void {
    this.checkedPeople.update((set) => flip(set, id));
  }

  protected retry(): void {
    this.state.set('loading');
    void this.reload();
  }

  /** Reads the list again: what left it is unchecked for good, what is new comes checked. */
  private async reload(): Promise<void> {
    try {
      const res = await this.api.listTreasures(this.data.campaignId);
      const now = new Set(res.treasures.map((t) => t.pointId));
      const fresh = res.treasures.filter((t) => !this.known.has(t.pointId)).map((t) => t.pointId);
      this.checkedTreasures.update(
        (set) => new Set([...[...set].filter((id) => now.has(id)), ...fresh]),
      );
      this.known = new Set([...this.known, ...now]);
      this.treasures.set(res.treasures);
      this.totalFound.set(res.total);
      this.state.set('ready');
    } catch {
      // A list the host gave stays (the server refuses a stale choice anyway); with none, say it could not read.
      this.state.set(this.treasures().length > 0 || this.data.treasures ? 'ready' : 'error');
    }
  }

  private drop(characterId: string): void {
    this.people.update((rows) => rows.filter((r) => r.id !== characterId));
    this.checkedPeople.update((set) => {
      const next = new Set(set);
      next.delete(characterId);
      return next;
    });
  }

  protected async give(): Promise<void> {
    if (this.busy() || this.waiting()) {
      return;
    }
    const ids = this.chosen().map((t) => t.pointId);
    const people = this.receivers().map((r) => r.id);
    // New choices are a new award; the same ones again are a retry.
    const key = this.key.keyFor([ids, people]);
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.award(
        this.data.campaignId,
        { mode: 'town', treasurePointIds: ids },
        TOWN_REASON,
        people,
        key,
      );
      this.sheet.close(
        res.award ? { award: res.award, xpEach: res.xpEach, lostXp: res.lostXp } : undefined,
      );
    } catch (err) {
      this.error.set(townErrorMessage(err));
      this.frame().scrollToTop();
      const blocked = xpBlocked(err);
      if (
        blocked?.reason === XPBlockedReason.XP_BLOCKED_REASON_CHARACTER_NOT_ELIGIBLE &&
        blocked.characterId
      ) {
        this.drop(blocked.characterId);
      }
      if (blocked || xpNotFound(err)) {
        await this.reload();
      }
    } finally {
      this.busy.set(false);
    }
  }

  protected cancel(): void {
    this.sheet.close(undefined);
  }
}

function flip(set: ReadonlySet<string>, id: string): ReadonlySet<string> {
  const next = new Set(set);
  if (!next.delete(id)) {
    next.add(id);
  }
  return next;
}
