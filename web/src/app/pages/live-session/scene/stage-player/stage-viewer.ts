import { Component, computed, effect, signal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';

import type { StageNpc } from '../../../../../gen/meurpg/play/v1/scene_pb';
import type { SceneState } from '../../../../core/play/scene-state';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../../shared/sheet/sheet-host';
import { StageFigure } from './stage-figure';

/** What a tap on a figure hands the view: the scene's state and the stage entry. */
export interface StageViewerData {
  readonly state: SceneState;
  /** The entry's `StageNpc.id`, not the character's (a player never has that). */
  readonly entryId: string;
}

/**
 * A character of the stage, larger (MR-031, Q63, E8-10 state 5): the name as
 * the title, the portrait, and the combat's sheet frame around it: a bottom
 * sheet on a phone, a 480px dialog from a tablet up. Nothing else: no kind, no
 * hit points, no armor class, no note and no action, because the scene is the
 * master's and a player only looks. A fixed footer has an outlined "Fechar",
 * never a filled button. Focus starts on the title; Esc, ✕, "Fechar" and a tap
 * outside close it.
 *
 * It follows the stage: when the NPC leaves it closes by itself, and when
 * the portrait goes (404 once the NPC is off stage) the initials stay.
 */
@Component({
  selector: 'app-stage-viewer',
  imports: [MatButtonModule, SheetFrame, StageFigure],
  template: `
    <app-sheet-frame [title]="npc()?.name ?? ''" titleId="stage-viewer-title" [phone]="inSheet" (closed)="close()">
      <div class="big" [class.big--phone]="inSheet">
        <app-stage-figure class="big__fig" [src]="npc()?.portraitUrl ?? ''" [name]="npc()?.name ?? ''" />
      </div>
      <div foot>
        <button matButton="outlined" type="button" class="foot" (click)="close()">Fechar</button>
      </div>
    </app-sheet-frame>
  `,
  styleUrl: './stage-viewer.scss',
})
export class StageViewer {
  private readonly sheet = injectSheet<StageViewerData, void>();
  protected readonly inSheet = this.sheet.inSheet;
  /** The entry on the stage right now, or `undefined` once the NPC left. */
  private readonly onStage = computed(() =>
    this.sheet.data.state.stage().find((n) => n.id === this.sheet.data.entryId),
  );
  /** The last entry seen, so the title does not empty while the view closes. */
  private readonly last = signal<StageNpc | undefined>(undefined);
  protected readonly npc = computed(() => this.onStage() ?? this.last());

  constructor() {
    effect(() => {
      const npc = this.onStage();
      if (npc) {
        this.last.set(npc);
      } else {
        this.sheet.close();
      }
    });
  }

  protected close(): void {
    this.sheet.close();
  }
}
