import {
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  inject,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import { Code, ConnectError } from '@connectrpc/connect';

import type { Encounter } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { CombatUndone } from '../../../../core/combat/combat-undone';
import { CombatClient } from '../../../../core/combat/combat-client';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatLogState } from '../../../../core/combat/combat-log-state';
import type { CombatState } from '../../../../core/combat/combat-state';
import {
  entryCount,
  latestLine,
  logGroups,
  truncateGroups,
  undoLabel,
} from '../../../../core/combat/combat-log';
import { isPlayer } from '../../../../core/combat/combat-view';
import { isTheatre } from '../../../../core/combat/theatre';
import { PHONE_QUERY, mediaQuery } from '../../../../shared/map-view/media-query';
import { openSheet } from '../../../../shared/sheet/sheet-host';
import { LogList } from './log-list';
import { LogSheet, type LogSheetData } from './log-sheet';

/**
 * "Registro do combate" on the session page (E6-05, E6-11, E6-12, E6-14).
 * On a laptop it is a panel with the whole log (the master) or its latest
 * lines and "Ver o registro todo" (a player). On a phone it is collapsed: the
 * master's says "9 entradas" and opens in place; the player's shows the
 * latest entry in one line, and its 48px chevron opens the tall sheet
 * (E6-15). The master's "Desfazer última ação" asks in place, naming the
 * action from the log entry, the focus on "Voltar"; it sends the log's
 * `undoable_event_id`, and an `aborted` means someone acted first.
 */
@Component({
  selector: 'app-combat-log-panel',
  imports: [LogList, MatButtonModule, MatIconModule],
  templateUrl: './combat-log-panel.html',
  styleUrl: './combat-log-panel.scss',
})
export class CombatLogPanel {
  private readonly api = inject(CombatClient);
  private readonly undone = inject(CombatUndone);
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);
  private readonly injector = inject(Injector);
  protected readonly phone = mediaQuery(PHONE_QUERY);

  readonly log = input.required<CombatLogState>();
  readonly encounter = input.required<Encounter>();
  readonly campaignId = input.required<string>();
  readonly state = input.required<CombatState>();
  readonly master = input(false);
  /** The table hides the death saves (RN-24) and this is not their owner's screen: the log says why a character's rolls are not in it. */
  readonly deathNote = input(false);

  protected readonly open = signal(false);
  protected readonly confirming = signal(false);
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  private readonly safe = viewChild('safe', { read: ElementRef<HTMLButtonElement> });

  /** "Mudar as condições" under a Sono: the master's dialog for that creature. */
  readonly conditions = output<string>();

  protected readonly groups = computed(() =>
    logGroups(this.log().rounds(), this.encounter().round, this.encounter().name, {
      master: this.master(),
      players: new Set(
        this.encounter()
          .combatants.filter(isPlayer)
          .map((c) => c.label),
      ),
      theatre: isTheatre(this.encounter()),
    }),
  );
  protected readonly shown = computed(() =>
    this.master() ? this.groups() : truncateGroups(this.groups(), 6),
  );
  protected readonly more = computed(() => !this.master() && entryCount(this.groups()) > 6);
  protected readonly latest = computed(() => latestLine(this.groups()));
  protected readonly count = computed(() => {
    const n = entryCount(this.groups());
    return `${n} ${n === 1 ? 'entrada' : 'entradas'}`;
  });
  protected readonly undoable = computed(() => (this.master() ? this.log().undoable() : null));
  protected readonly undoText = computed(() => {
    const e = this.undoable();
    return e ? `Desfazer ${undoLabel(e)}?` : '';
  });
  /** The list is drawn: always on a laptop, on a phone only the master's, open. */
  protected readonly showList = computed(() => !this.phone() || (this.master() && this.open()));

  protected toggle(): void {
    this.open.update((v) => !v);
  }

  /** The player's tall sheet, with the whole log. */
  protected openSheet(): void {
    const data: LogSheetData = {
      groups: this.groups,
      encounterName: this.encounter().name,
      master: this.master(),
      deathNote: this.deathNote(),
    };
    openSheet<LogSheet, LogSheetData, void>(this.dialog, this.bottomSheet, LogSheet, {
      data,
      ariaLabel: 'Registro do combate',
      labelledBy: 'log-sheet-title',
      width: '560px',
      tall: true,
    }).subscribe();
  }

  protected askUndo(): void {
    this.error.set('');
    this.confirming.set(true);
    afterNextRender(() => this.safe()?.nativeElement.focus(), { injector: this.injector });
  }

  protected async undo(): Promise<void> {
    const id = this.log().undoableId();
    if (!id || this.busy()) {
      return;
    }
    this.busy.set(true);
    try {
      this.state().apply(await this.api.turn.undo(this.campaignId(), this.encounter().id, id));
      this.undone.mark();
      this.confirming.set(false);
    } catch (err) {
      this.confirming.set(false);
      this.error.set(
        ConnectError.from(err).code === Code.Aborted
          ? 'Outra ação aconteceu antes: confira o registro.'
          : combatErrorMessage(err, 'desfazer a ação'),
      );
      // The log may have moved on: read it again.
      this.state().touchLog();
    } finally {
      this.busy.set(false);
    }
  }
}
