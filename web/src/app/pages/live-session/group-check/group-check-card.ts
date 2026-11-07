import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  input,
  output,
  signal,
  untracked,
} from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';

import type { DiceMode, DicePreference } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { ContestClient } from '../../../core/combat/contest-client';
import { askedTest } from '../../../core/combat/contest-master';
import { GroupCheckState } from '../../../core/combat/group-check-state';
import { groupCheckHeading, openGroupCheckSheet } from './group-check-sheet';

/**
 * The group check on the player's page (W7-X, board W7-Xc 10): when the master asks the whole party for a check, the sheet opens by
 * itself, once, and the card stays under the header in case it was closed: "Rolar" while the player has not rolled, "Esperando o
 * mestre" after, and the player's own "Passou" or "Falhou" and the group's verdict once the master closed the check and showed the
 * DC (never before, RN-20). It reads the check again on each `group_check_changed` (the page bumps `tick`).
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-group-check-card',
  imports: [MatButtonModule, MatIconModule],
  templateUrl: './group-check-card.html',
  styleUrl: './group-check-card.scss',
})
export class GroupCheckCard {
  private readonly api = inject(ContestClient);
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);

  readonly campaignId = input.required<string>();
  /** Goes up on every `group_check_changed` and every (re)connection of the stream. */
  readonly tick = input(0);
  /** Moves when the stream says the questions of the held rolls changed: the sheet reads them again. */
  readonly heldTick = input(0);
  readonly diceMode = input.required<DiceMode>();
  readonly preference = input.required<DicePreference>();

  protected readonly state = new GroupCheckState();
  /** The checks whose sheet already opened by itself (this page's life), and the closed one the player put away. */
  private readonly opened = new Set<string>();
  private readonly dismissed = signal('');
  private readonly sheetOpen = signal(false);

  /** The player has a group check to roll (the card says "Rolar"): the page lights the scene's "Ações" tab with it. */
  readonly needsRoll = output<boolean>();

  protected readonly check = this.state.view;
  protected readonly own = this.state.own;
  protected readonly mode = computed(() => {
    const c = this.check();
    if (!c) {
      return 'none' as const;
    }
    if (c.open) {
      return c.youRoll ? ('roll' as const) : ('wait' as const);
    }
    const shown = !!this.own()?.passedKnown || c.verdictKnown;
    return shown && this.dismissed() !== c.id ? ('result' as const) : ('none' as const);
  });
  /** "Teste em grupo" for a group check; "Teste" (or "Teste de resistência") for a request judged one by one. */
  protected readonly heading = computed(() => groupCheckHeading(this.check()));
  /** "um teste de resistência de Constituição", "um teste de Percepção". */
  protected readonly asked = computed(() => askedTest(this.check()?.skillNamePt ?? ''));
  protected readonly line = computed(() => {
    const c = this.check();
    if (!c) {
      return '';
    }
    const own = this.own();
    const mine = own?.passedKnown ? `${own.passed ? 'Passou' : 'Falhou'}. ` : '';
    const group = c.verdictKnown ? (c.groupPassed ? 'O grupo passou.' : 'O grupo falhou.') : '';
    return `${mine}${group}`.trim();
  });

  constructor() {
    effect(() => {
      const campaignId = this.campaignId();
      this.tick();
      untracked(() => void this.state.load(this.api, campaignId));
    });
    effect(() => {
      const waits = this.mode() === 'roll';
      untracked(() => this.needsRoll.emit(waits));
    });
    // A new check that waits for this player's roll opens its sheet, once.
    effect(() => {
      const c = this.check();
      if (c?.open && c.youRoll && !this.opened.has(c.id) && !untracked(() => this.sheetOpen())) {
        this.opened.add(c.id);
        untracked(() => this.openSheet());
      }
    });
  }

  protected openSheet(): void {
    this.sheetOpen.set(true);
    openGroupCheckSheet(this.dialog, this.bottomSheet, {
      campaignId: this.campaignId(),
      diceMode: this.diceMode(),
      preference: this.preference(),
      state: this.state,
      heldTick: this.heldTick,
    }).subscribe({
      complete: () => this.sheetOpen.set(false),
      error: () => this.sheetOpen.set(false),
    });
  }

  protected dismiss(): void {
    const c = this.check();
    if (c) {
      this.dismissed.set(c.id);
    }
  }
}
