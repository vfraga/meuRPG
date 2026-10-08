import {
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  effect,
  inject,
  input,
  output,
  signal,
  untracked,
  viewChild,
} from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import { RouterLink } from '@angular/router';

import { XpMode } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CombatantState, type Encounter } from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  XPAwardMode,
  XPBlockedReason,
  type XPAward,
} from '../../../../../gen/meurpg/progression/v1/progression_pb';
import { newKey } from '../../../../core/connect/idempotency';
import { article } from '../../../../core/combat/combat-log';
import { groupLabel } from '../../../../core/combat/monsters';
import { isDown, isPlayer, stateWord } from '../../../../core/combat/combat-view';
import { formatInt, tight } from '../../../../core/format/text';
import { ExperienceStore } from '../../../../core/progression/experience-store';
import { ProgressionClient } from '../../../../core/progression/progression-client';
import { XpChanges } from '../../../../core/progression/xp-changes';
import { xpErrorMessage, xpBlocked } from '../../../../core/progression/xp-errors';
import { givenText, nameList } from '../../../../core/progression/xp-labels';
import { splitXp } from '../../../../core/progression/xp-math';
import { LevelUpTag } from '../../../../shared/xp/level-up-tag';
import { openAwardXp } from '../../../../shared/xp/xp-give-button';
import { XpActions } from '../../../../shared/xp/xp-actions';
import { type Recipient, XpRecipients } from '../../../../shared/xp/xp-recipients';
import { XpSplit } from '../../../../shared/xp/xp-split';
import type { CombatantInfo } from '../combat-info';

/** Where the combat's XP stands, for the summary around it: `loading` and
 * `open` ask for the room of the block, `later` and `given` have settled, and
 * `none` is a campaign that gives no XP for enemies (nothing to draw). */
export type CombatXpState = 'loading' | 'none' | 'open' | 'later' | 'given';

/** The longest reason the server takes. */
const REASON_MAX = 120;

/**
 * "Experiência do combate" (E7-06, MR-016): at the end of a combat in an
 * "enemies" campaign the master sees what the defeated NPCs are worth (their
 * `xp_value`, which only the master gets), who receives (those who fought and
 * are not dead, by default; he may uncheck or, for the dead, nothing) and the
 * division live, then gives it with one button. "Agora não" leaves a quiet line
 * that opens "Dar XP" with the reason and the total filled, so the XP is never
 * lost, only postponed. Giving it says what each one got and who can level up.
 *
 * The block reads its own `ExperienceStore` and the history (to know the combat's
 * XP was already given, even after a reload or by another tab). It reads again on
 * `xp_changed`. One idempotency key for the give, made once: a retry after a lost
 * answer repeats it.
 */
@Component({
  selector: 'app-combat-xp',
  imports: [
    LevelUpTag,
    MatButtonModule,
    MatIconModule,
    RouterLink,
    XpActions,
    XpRecipients,
    XpSplit,
  ],
  providers: [ExperienceStore],
  templateUrl: './combat-xp.html',
  styleUrl: './combat-xp.scss',
})
export class CombatXp {
  private readonly api = inject(ProgressionClient);
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);
  private readonly changes = inject(XpChanges);
  private readonly injector = inject(Injector);
  protected readonly store = inject(ExperienceStore);

  readonly campaignId = input.required<string>();
  readonly encounter = input.required<Encounter>();
  readonly info = input<ReadonlyMap<string, CombatantInfo>>(new Map());

  /** Where the combat's XP stands, so "Voltar à sessão" and the layout follow. */
  readonly stateChange = output<CombatXpState>();

  protected readonly busy = signal(false);
  protected readonly error = signal('');
  /** The master pressed "Agora não". */
  protected readonly postponed = signal(false);
  /** Who is unchecked / checked, once the rows are known. */
  private readonly checkedIds = signal<ReadonlySet<string> | null>(null);
  /** The award just given from here, with each one's XP before it. */
  protected readonly just = signal<{ award: XPAward; before: ReadonlyMap<string, number> } | null>(
    null,
  );

  private readonly confirmation = viewChild<ElementRef<HTMLElement>>('confirmation');
  private readonly laterButton = viewChild('later', { read: ElementRef<HTMLElement> });
  private key = newKey();
  private keyFor = '';

  protected readonly defeated = computed(() =>
    this.encounter().combatants.filter((c) => !isPlayer(c) && c.defeated),
  );
  protected readonly total = computed(() => this.defeated().reduce((sum, c) => sum + c.xpValue, 0));
  /** "200 + 50 + 50 + 50". */
  protected readonly sum = computed(() =>
    this.defeated()
      .map((c) => formatInt(c.xpValue))
      .join(' + '),
  );
  /**
   * When monsters of the bestiary were defeated (RN-29), what each kind is worth by its challenge rating: "Bandido 1 a 3 · ND 1/8 · 25 XP
   * cada" and the kind's total. The XP is the server's `xp_value` of each (the master's alone); an NPC that is not a monster is its own
   * row. Empty with no monster, so the block is as it always was.
   */
  protected readonly kinds = computed(() => {
    const defeated = this.defeated();
    if (!defeated.some((c) => c.bestiaryCreatureKey)) {
      return [];
    }
    const groups = new Map<string, typeof defeated>();
    for (const c of defeated) {
      const key = c.bestiaryCreatureKey ? `${c.bestiaryCreatureKey}|${c.xpValue}` : c.id;
      groups.set(key, [...(groups.get(key) ?? []), c]);
    }
    return [...groups.values()].map((g) => ({
      key: g[0].id,
      label: groupLabel(g.map((c) => c.label)),
      nd: g[0].challengeRating,
      each: tight(`${formatInt(g[0].xpValue)} XP cada`),
      total: tight(`${formatInt(g.reduce((n, c) => n + c.xpValue, 0))} XP`),
    }));
  });
  protected readonly totalLabel = computed(() => {
    const n = this.defeated().length;
    return n === 1 ? 'Total do derrotado' : `Total dos ${n} derrotados`;
  });
  protected readonly totalText = computed(() => tight(`${formatInt(this.total())} XP`));

  /** The award given from here was undone since (elsewhere): it no longer stands. */
  private readonly justUndone = computed(() => {
    const id = this.just()?.award.id;
    return !!id && this.store.awards().some((a) => a.id === id && a.undone);
  });

  /** The award of this combat that still stands (given here, or earlier). */
  protected readonly award = computed<XPAward | null>(
    () =>
      (this.justUndone() ? null : this.just()?.award) ??
      this.store
        .awards()
        .find(
          (a) =>
            a.mode === XPAwardMode.XP_AWARD_MODE_ENEMIES &&
            a.encounterId === this.encounter().id &&
            !a.undone,
        ) ??
      null,
  );

  protected readonly view = computed<CombatXpState>(() => {
    if (this.store.rowsState() === 'loading' || this.store.awardsState() === 'loading') {
      return 'loading';
    }
    if (this.store.rowsState() === 'error' || this.store.xpMode() !== XpMode.ENEMIES) {
      return 'none';
    }
    if (this.award()) {
      return 'given';
    }
    return this.postponed() ? 'later' : 'open';
  });

  /** The party of the combat, with what each one is: who can receive. */
  private readonly people = computed(() =>
    this.encounter()
      .combatants.filter(isPlayer)
      .map((c) => ({ c, row: this.store.rows().find((r) => r.id === c.characterId) ?? null })),
  );
  protected readonly checked = computed(
    () =>
      this.checkedIds() ??
      new Set(
        this.people()
          .filter((p) => p.row && p.c.state !== CombatantState.DEAD)
          .map((p) => p.c.characterId),
      ),
  );
  protected readonly split = computed(() => splitXp(this.total(), this.checked().size));

  protected readonly recipients = computed<Recipient[]>(() =>
    this.people().map(({ c, row }) => {
      const on = this.checked().has(c.characterId);
      const dead = c.state === CombatantState.DEAD || !row;
      const feminine = article(c.label) === 'a';
      const classLine = this.info().get(c.characterId)?.classSummary || '';
      return {
        id: c.characterId,
        name: c.label,
        sub: [classLine, row ? tight(`${formatInt(row.xp)} XP agora`) : '']
          .filter(Boolean)
          .join(' · '),
        checked: on,
        disabled: dead,
        amount: on ? tight(`+${formatInt(this.split().each)} XP`) : 'Não recebe',
        tag: dead
          ? { label: stateWord(CombatantState.DEAD, c.label), icon: 'close' }
          : isDown(c)
            ? { label: stateWord(c.state, c.label), icon: 'warning' }
            : undefined,
        note: dead
          ? 'Morreu: não recebe XP.'
          : on && isDown(c)
            ? feminine
              ? 'Está viva, então recebe a parte dela.'
              : 'Está vivo, então recebe a parte dele.'
            : undefined,
      };
    }),
  );

  protected readonly reasonToWait = computed(() => {
    if (this.checked().size === 0) {
      return 'Marque pelo menos um personagem';
    }
    if (this.total() === 0) {
      return 'Nenhum derrotado dá XP. Use "Dar XP" para dar um valor avulso.';
    }
    if (this.split().each === 0) {
      return 'O total é pequeno demais: cada um precisa receber pelo menos 1 XP.';
    }
    return '';
  });
  protected readonly primaryLabel = computed(() =>
    this.reasonToWait() === ''
      ? tight(`Dar ${formatInt(this.split().each)} XP a cada um`)
      : 'Dar XP',
  );

  /** The confirmation: what was given, and each one's new XP. */
  protected readonly confirmed = computed(() => {
    const award = this.award();
    if (!award) {
      return null;
    }
    const each = award.shares[0]?.xp ?? 0;
    const lost = award.totalXp - each * award.shares.length;
    const before = this.just()?.before;
    const lines = award.shares.map((s) => {
      const row = this.store.rows().find((r) => r.id === s.characterId);
      const now = row?.xp ?? 0;
      const was = before?.get(s.characterId);
      return {
        id: s.characterId,
        name: s.characterName,
        math:
          was !== undefined
            ? tight(`${formatInt(was)} + ${formatInt(s.xp)} = ${formatInt(was + s.xp)} XP`)
            : tight(`Recebeu ${formatInt(s.xp)} XP`),
        of: row
          ? row.nextLevelXp > 0
            ? tight(`${formatInt(now)} de ${formatInt(row.nextLevelXp)} XP`)
            : tight(`${formatInt(now)} XP`)
          : '',
        levelUp: row?.canLevelUp ?? false,
      };
    });
    const up = lines.filter((l) => l.levelUp).map((l) => l.name);
    return {
      text: givenText(award.totalXp, each, lost),
      lines,
      up: up.length
        ? `${nameList(up)} ${up.length > 1 ? 'chegaram' : 'chegou'} ao XP do próximo nível. Suba o nível na ficha.`
        : '',
    };
  });

  constructor() {
    // The block's room and the summary's button follow where the XP stands.
    effect(() => this.stateChange.emit(this.view()));
    // `xp_changed` on the stream (an award, an undo, a milestone): read again.
    let seen = untracked(() => this.changes.version());
    effect(() => {
      const v = this.changes.version();
      if (v !== seen) {
        seen = v;
        untracked(() => void this.store.refresh());
      }
    });
    effect(() => {
      const id = this.campaignId();
      untracked(() => void this.store.load(id, true));
    });
  }

  protected toggle(id: string): void {
    const next = new Set(this.checked());
    if (!next.delete(id)) {
      next.add(id);
    }
    this.checkedIds.set(next);
  }

  protected notNow(): void {
    this.postponed.set(true);
    this.error.set('');
    // The form is gone: the focus goes to the line that replaced it.
    afterNextRender(() => this.laterButton()?.nativeElement.focus(), { injector: this.injector });
  }

  /** The quiet line opens "Dar XP" with the reason and the total filled. */
  protected openLater(): void {
    const enc = this.encounter();
    openAwardXp(this.dialog, this.bottomSheet, {
      campaignId: this.campaignId(),
      xpMode: this.store.xpMode(),
      rows: this.store.rows(),
      reason: `Combate: ${enc.name}`.slice(0, REASON_MAX),
      amount: this.total() > 0 ? this.total() : undefined,
      encounterId: enc.id,
    }).subscribe(async (result) => {
      // Never a "Voltar à cidade" request here: this sheet opens for a combat's XP, with no treasure strip.
      if (result && 'award' in result) {
        this.just.set({ award: result.award, before: this.snapshot() });
        await this.store.refresh();
        this.focusConfirmation();
      }
    });
  }

  protected async give(): Promise<void> {
    if (this.busy() || this.reasonToWait() !== '') {
      return;
    }
    const enc = this.encounter();
    const ids = this.people()
      .filter((p) => this.checked().has(p.c.characterId))
      .map((p) => p.c.characterId);
    const reason = `Combate: ${enc.name}`.slice(0, REASON_MAX);
    // The same people for the same combat are a retry; another set is a new award.
    const signature = JSON.stringify([enc.id, ids]);
    if (this.justUndone()) {
      // The earlier award is gone: giving again is a new action, not its retry.
      this.just.set(null);
      this.keyFor = '';
    }
    if (signature !== this.keyFor) {
      this.keyFor = signature;
      this.key = newKey();
    }
    const before = this.snapshot();
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.award(
        this.campaignId(),
        { mode: 'enemies', encounterId: enc.id },
        reason,
        ids,
        this.key,
      );
      if (res.award) {
        this.just.set({ award: res.award, before });
      }
      await this.store.refresh();
      this.focusConfirmation();
    } catch (err) {
      this.error.set(xpErrorMessage(err, 'dar o XP'));
      // Already given (or the list is stale): read again, the state follows.
      const blocked = xpBlocked(err);
      if (blocked?.reason === XPBlockedReason.XP_BLOCKED_REASON_ALREADY_AWARDED) {
        await this.store.refresh();
      }
    } finally {
      this.busy.set(false);
    }
  }

  private snapshot(): ReadonlyMap<string, number> {
    return new Map(this.store.rows().map((r) => [r.id, r.xp]));
  }

  /** The form is gone: the focus goes to the confirmation. */
  private focusConfirmation(): void {
    afterNextRender(() => this.confirmation()?.nativeElement.focus(), { injector: this.injector });
  }
}
