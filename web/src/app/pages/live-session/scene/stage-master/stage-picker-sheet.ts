import { Component } from '@angular/core';
import type { Signal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';

import type { SceneState } from '../../../../core/play/scene-state';
import type { StageController } from '../../../../core/play/stage-controller';
import type { StageCandidate } from '../../../../core/play/stage-roster';
import { stageCount } from '../../../../core/play/stage-view';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../../shared/sheet/sheet-host';
import { StageList } from './stage-list';

/** What "Pôr em cena" hands the phone's sheet. */
export interface StagePickerData {
  readonly candidates: Signal<readonly StageCandidate[]>;
  readonly state: SceneState;
  readonly ctl: StageController;
  readonly rosterError: Signal<boolean>;
}

/**
 * "Pôr em cena" on a phone (E8-09 state 5): the combat's sheet frame (grab
 * bar, title, ✕, a body that scrolls and a fixed footer) around the list of
 * NPCs. There is no action to confirm, since each tap acts at once, so the
 * footer has only an outlined "Fechar", never a filled button. The sheet stays
 * open for the next NPC; with a full stage, the sentence comes before the list.
 */
@Component({
  selector: 'app-stage-picker-sheet',
  imports: [MatButtonModule, SheetFrame, StageList],
  template: `
    <app-sheet-frame
      #frame
      title="Pôr em cena"
      [subtitle]="subtitle()"
      titleId="stage-picker-title"
      [phone]="inSheet"
      (closed)="close()"
    >
      @if (data.rosterError()) {
        <p class="note" role="alert">Não deu para carregar os NPCs da campanha. Feche e tente de novo.</p>
      } @else if (data.candidates().length === 0) {
        <p class="note">A campanha ainda não tem NPCs. Crie um na página da campanha.</p>
      }
      <app-stage-list
        [candidates]="data.candidates()"
        [state]="data.state"
        [ctl]="data.ctl"
        (exhausted)="closeButton.focus()"
      />
      @if (data.ctl.error(); as message) {
        <div class="mr-notice mr-notice--danger" role="alert">{{ message }}</div>
      }
      <p class="mr-visually-hidden" role="status" aria-live="polite">{{ data.ctl.status() }}</p>
      <div foot>
        <button #closeButton matButton="outlined" type="button" class="foot" (click)="close()">Fechar</button>
      </div>
    </app-sheet-frame>
  `,
  styles: `
    :host {
      display: block;
    }
    .note {
      margin: 0;
      font-size: 14px;
      color: var(--mr-ink-muted);
    }
    .foot {
      width: 100%;
      min-height: 48px;
    }
  `,
})
export class StagePickerSheet {
  private readonly sheet = injectSheet<StagePickerData, void>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;

  protected subtitle(): string {
    return stageCount(this.data.state.stage().length);
  }

  protected close(): void {
    this.sheet.close();
  }
}
