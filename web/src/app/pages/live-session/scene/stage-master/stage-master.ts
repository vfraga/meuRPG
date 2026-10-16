import {
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  inject,
  input,
  signal,
  viewChild,
} from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';

import type { StageNpc } from '../../../../../gen/meurpg/play/v1/scene_pb';
import { SceneClient } from '../../../../core/play/scene-client';
import type { SceneState } from '../../../../core/play/scene-state';
import { StageController } from '../../../../core/play/stage-controller';
import { type StageCandidate, StageRoster } from '../../../../core/play/stage-roster';
import { STAGE_FULL_REASON, stageCount } from '../../../../core/play/stage-view';
import { PHONE_QUERY, mediaQuery } from '../../../../shared/map-view/media-query';
import { Portrait } from '../../../../shared/portrait/portrait';
import { openSheet } from '../../../../shared/sheet/sheet-host';
import { StageList } from './stage-list';
import { StagePickerSheet, type StagePickerData } from './stage-picker-sheet';

/** A card of the stage: the entry and what the roster knows of the NPC. */
interface StageCard {
  readonly npc: StageNpc;
  readonly kind: string;
}

/**
 * "Em cena", the master's side of the stage (MR-031, E8-09): the first section
 * of the open scene. One card per NPC on the stage, in the order they came in
 * (portrait, name, kind), each with "Dar a fala" / "Fala agora" (a toggle,
 * `aria-pressed`; the speaker's card gets the 2px accent border) and "Tirar de
 * cena" (at once). Below, "Pôr em cena" lists the campaign's NPCs in place on
 * a desktop and in a sheet on a phone; a full stage ("4 de 4") says why in a
 * sentence and shows the dashed button.
 *
 * The writes go through one `StageController`: one in flight per NPC, the
 * answer applied at once, an error by code. Focus: "Tirar de cena" sends it to
 * the next card (or the previous one, or "Pôr em cena"); opening the list sends
 * it to the list's title, and "Fechar" gives it back to "Pôr em cena".
 */
@Component({
  selector: 'app-stage-master',
  imports: [MatButtonModule, MatIconModule, Portrait, StageList],
  templateUrl: './stage-master.html',
  styleUrl: './stage-master.scss',
})
export class StageMaster {
  private readonly api = inject(SceneClient);
  private readonly roster = inject(StageRoster);
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);
  private readonly injector = inject(Injector);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);

  readonly campaignId = input.required<string>();
  readonly state = input.required<SceneState>();

  protected readonly phone = mediaQuery(PHONE_QUERY);
  protected readonly candidates = signal<readonly StageCandidate[]>([]);
  protected readonly rosterError = signal(false);
  protected readonly listOpen = signal(false);
  protected readonly reason = STAGE_FULL_REASON;

  protected readonly ctl = computed(
    () =>
      new StageController(
        this.api,
        this.state(),
        () => this.campaignId(),
        (id) =>
          this.state()
            .stage()
            .find((n) => n.characterId === id)?.name ??
          this.candidates().find((c) => c.characterId === id)?.name ??
          'O NPC',
      ),
  );
  protected readonly stage = computed(() => this.state().stage());
  protected readonly full = computed(() => !this.ctl().hasRoom());
  protected readonly count = computed(() => stageCount(this.stage().length));
  protected readonly cards = computed<StageCard[]>(() =>
    this.stage().map((npc) => ({
      npc,
      kind: this.candidates().find((c) => c.characterId === npc.characterId)?.kindLabel ?? '',
    })),
  );

  private readonly listTitle = viewChild<ElementRef<HTMLElement>>('listTitle');
  private readonly putButton = viewChild('putButton', { read: ElementRef<HTMLElement> });

  constructor() {
    afterNextRender(() => void this.loadRoster());
  }

  private async loadRoster(): Promise<void> {
    try {
      this.candidates.set(await this.roster.list(this.campaignId()));
      this.rosterError.set(false);
    } catch {
      // Best effort: the cards keep what the stage says (name and portrait).
      this.rosterError.set(true);
    }
  }

  protected async speak(card: StageCard): Promise<void> {
    await this.ctl().speak(card.npc.characterId, card.npc.speaking);
  }

  protected async take(card: StageCard, index: number): Promise<void> {
    if (!(await this.ctl().take(card.npc.characterId))) {
      return;
    }
    afterNextRender(
      () => {
        const cards = this.host.nativeElement.querySelectorAll<HTMLElement>('[data-stage-card]');
        const target =
          cards[Math.min(index, cards.length - 1)]?.querySelector<HTMLElement>('button');
        (target ?? this.putButton()?.nativeElement)?.focus();
      },
      { injector: this.injector },
    );
  }

  /** "Pôr em cena": the list in place, or the sheet on a phone. */
  protected async openList(): Promise<void> {
    if (this.full()) {
      return; // the dashed button answers nothing; the sentence says why
    }
    if (this.rosterError() || this.candidates().length === 0) {
      await this.loadRoster();
    }
    const ctl = this.ctl();
    ctl.clearMessages();
    if (this.phone()) {
      const data: StagePickerData = {
        candidates: this.candidates,
        state: this.state(),
        ctl,
        rosterError: this.rosterError,
      };
      openSheet<StagePickerSheet, StagePickerData, void>(
        this.dialog,
        this.bottomSheet,
        StagePickerSheet,
        {
          data,
          ariaLabel: 'Pôr em cena',
        },
      ).subscribe(() => this.putButton()?.nativeElement.focus());
      return;
    }
    this.listOpen.set(true);
    afterNextRender(() => this.listTitle()?.nativeElement.focus(), { injector: this.injector });
  }

  protected closeList(): void {
    this.listOpen.set(false);
    afterNextRender(() => this.putButton()?.nativeElement.focus(), { injector: this.injector });
  }

  protected focusTitle(): void {
    this.listTitle()?.nativeElement.focus();
  }
}
