import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  computed,
  effect,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import type { Observable } from 'rxjs';

import { HelpKind } from '../../../../../gen/meurpg/play/v1/contest_types_pb';
import { ActionKey } from '../../../../core/connect/idempotency';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { ContestClient } from '../../../../core/combat/contest-client';
import { contestSteps } from '../../../../core/combat/contest-view';
import {
  HELP_TASKS,
  type HelpAlly,
  type HelpTarget,
  attackIntro,
  attackRow,
  helpedText,
  taskRow,
} from '../../../../core/combat/help-view';
import { Segmented, type Segment } from '../../../../shared/segmented/segmented';
import { AttackSteps } from '../attack-sheet/attack-steps';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../../../../shared/sheet/sheet-host';

/** What the page hands the Help sheet. */
export interface HelpSheetData {
  readonly campaignId: string;
  readonly encounterId: string;
  readonly helperId: string;
  /** The allies the player may help (the same side, standing). */
  readonly allies: readonly HelpAlly[];
  /** The creatures the helper sees on the other side, with the distance the server worked out. */
  readonly targets: readonly HelpTarget[];
  readonly state: CombatState;
}

type Kind = 'check' | 'attack';
type Stage = 'ally' | 'what' | 'done';

const STEPS: Record<Kind, readonly string[]> = {
  check: ['Aliado', 'Tarefa'],
  attack: ['Aliado', 'Alvo'],
};
const KINDS: readonly Segment<Kind>[] = [
  { value: 'check', label: 'Em um teste', icon: 'task_alt' },
  { value: 'attack', label: 'Um ataque', icon: 'swords' },
];
const AT: Record<Stage, number> = { ally: 0, what: 1, done: 2 };

export function openHelpSheet(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: HelpSheetData,
): Observable<boolean | undefined> {
  return openSheet<HelpSheet, HelpSheetData, boolean>(dialog, bottomSheet, HelpSheet, {
    data,
    ariaLabel: 'Ajudar',
    labelledBy: 'help-t',
  });
}

/**
 * "Ajudar em um teste" and "Ajudar um ataque" (W7-X, board W7-Xc 9): the Help action spends the action and gives an ally advantage,
 * on their next check of a task or on their first attack against a creature within 1,5 m of the helper (SRD 5.1, Help). The
 * player picks the ally, then the task (a check the effect names, which lasts until the ally's next check of it or the end of
 * the helper's next turn) or the target (a creature at 1,5 m; a farther one stays in the list, off, with the reason). The server
 * judges the choice and the reach; the advantage shows to the table as the source "Ajuda de <nome>".
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-help-sheet',
  imports: [AttackSteps, MatButtonModule, MatIconModule, Segmented, SheetFrame],
  templateUrl: './help-sheet.html',
  styleUrl: '../contest-sheet/contest-sheet.scss',
})
export class HelpSheet {
  private readonly api = inject(ContestClient);
  private readonly sheet = injectSheet<HelpSheetData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly kinds = KINDS;

  protected readonly kind = signal<Kind>('check');
  protected readonly stage = signal<Stage>('ally');
  protected readonly allyId = signal<string | null>(this.data.allies[0]?.id ?? null);
  protected readonly taskKey = signal<string | null>(null);
  protected readonly targetId = signal<string | null>(null);
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  private readonly keys = new ActionKey();
  private readonly frame = viewChild(SheetFrame);
  private readonly done = viewChild('done', { read: ElementRef<HTMLButtonElement> });

  protected readonly ally = computed(
    () => this.data.allies.find((a) => a.id === this.allyId()) ?? null,
  );
  protected readonly title = computed(() =>
    this.kind() === 'check' ? 'Ajudar em um teste' : 'Ajudar um ataque',
  );
  protected readonly stepList = computed(() => contestSteps(STEPS[this.kind()], AT[this.stage()]));
  protected readonly tasks = computed(() => {
    const ally = this.ally();
    return ally ? HELP_TASKS.map((t) => ({ ...t, ...taskRow(ally, t.name) })) : [];
  });
  protected readonly targets = computed(() => {
    const ally = this.ally();
    return ally
      ? this.data.targets.map((t) => ({ id: t.id, target: t, ...attackRow(ally, t) }))
      : [];
  });
  protected readonly intro = computed(() => {
    const ally = this.ally();
    return ally ? attackIntro(ally) : '';
  });
  protected readonly chosenTask = computed(
    () => HELP_TASKS.find((t) => t.key === this.taskKey()) ?? null,
  );
  protected readonly chosenTarget = computed(
    () => this.data.targets.find((t) => t.id === this.targetId()) ?? null,
  );
  protected readonly ready = computed(() =>
    this.kind() === 'check' ? this.chosenTask() !== null : this.chosenTarget()?.reachable === true,
  );
  protected readonly helped = computed(() => {
    const ally = this.ally();
    return ally ? helpedText(ally, this.chosenTask()?.name ?? null, this.chosenTarget()) : null;
  });
  protected readonly durationText =
    'Vale até o próximo teste de {ally} para essa tarefa, ou até o início do seu próximo turno.';
  protected readonly duration = computed(() =>
    this.durationText.replace('{ally}', this.ally()?.label ?? ''),
  );
  protected readonly button = computed(() => `Ajudar ${this.ally()?.label ?? ''}`.trim());

  constructor() {
    effect(() => this.sheet.lock(this.busy()));
    effect(() => {
      if (this.error()) {
        this.frame()?.scrollToTop();
      }
    });
    effect(() => this.done()?.nativeElement.focus());
  }

  protected setKind(kind: Kind): void {
    this.kind.set(kind);
    this.taskKey.set(null);
    this.targetId.set(null);
    this.error.set('');
  }

  protected pickAlly(id: string): void {
    this.allyId.set(id);
    this.taskKey.set(null);
    this.targetId.set(null);
  }

  protected next(): void {
    if (this.allyId() !== null) {
      this.stage.set('what');
      // The first target a help can reach starts chosen, as the board draws it.
      const first = this.data.targets.find((t) => t.reachable);
      if (this.kind() === 'attack' && first && this.targetId() === null) {
        this.targetId.set(first.id);
      }
    }
  }

  protected backToAlly(): void {
    this.stage.set('ally');
    this.error.set('');
  }

  protected async help(): Promise<void> {
    const ally = this.ally();
    if (!ally || !this.ready() || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const input = {
        campaignId: this.data.campaignId,
        encounterId: this.data.encounterId,
        helperId: this.data.helperId,
        kind: this.kind() === 'check' ? HelpKind.CHECK : HelpKind.ATTACK,
        allyId: ally.id,
        taskKey: this.kind() === 'check' ? (this.taskKey() ?? '') : '',
        targetId: this.kind() === 'attack' ? (this.targetId() ?? '') : '',
      };
      const res = await this.api.help(input, this.keys.keyFor(input));
      this.data.state.apply(res.encounter);
      this.stage.set('done');
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'ajudar'));
    } finally {
      this.busy.set(false);
    }
  }

  protected close(): void {
    if (!this.busy()) {
      this.sheet.close(this.stage() === 'done');
    }
  }
}
