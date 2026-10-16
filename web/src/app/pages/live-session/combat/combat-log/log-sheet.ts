import { Component, type Signal } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { LogGroup } from '../../../../core/combat/combat-log';
import { injectSheet } from '../../../../shared/sheet/sheet-host';
import { LogList } from './log-list';

export interface LogSheetData {
  readonly groups: Signal<readonly LogGroup[]>;
  readonly encounterName: string;
  /** A player's log says it holds only what their character sees. */
  readonly master: boolean;
  /** The table hides the death saves (RN-24) and this is not their owner's screen. */
  readonly deathNote?: boolean;
}

/**
 * "Registro do combate" as a tall bottom sheet on a phone and a dialog from
 * a tablet up (E6-15). The list reads the page's log signal, so a new entry
 * appears while the sheet is open.
 */
@Component({
  selector: 'app-log-sheet',
  imports: [LogList, MatIconModule],
  template: `
    <div class="sheet" [class.sheet--phone]="inSheet">
      @if (inSheet) {
        <span class="sheet__handle" aria-hidden="true"></span>
      }
      <div class="sheet__head">
        <div>
          <h2 class="sheet__title" id="log-sheet-title" tabindex="-1">Registro do combate</h2>
          <p class="sheet__sub">
            {{ data.encounterName }}{{ data.master ? '' : ' · só o que seu personagem vê' }}
          </p>
        </div>
        <button type="button" class="sheet__close" aria-label="Fechar" (click)="close()">
          <mat-icon aria-hidden="true">close</mat-icon>
        </button>
      </div>
      <app-log-list class="sheet__list" [groups]="data.groups()" />
      @if (data.deathNote) {
        <p class="note">
          <mat-icon aria-hidden="true">info</mat-icon>Esta mesa só deixa o dono e o mestre verem os testes contra a morte.
        </p>
      }
    </div>
  `,
  styles: `
    :host {
      display: block;
    }

    .note {
      display: flex;
      align-items: flex-start;
      gap: 8px;
      margin: 4px 0 0;
      font-size: 14px;
      line-height: 19px;
      color: var(--mr-ink-muted);

      mat-icon {
        flex: none;
        width: 20px;
        height: 20px;
        font-size: 20px;
      }
    }

    .sheet {
      display: flex;
      flex-direction: column;
      gap: 6px;
      box-sizing: border-box;
      max-height: 80dvh;
      padding: var(--mr-space-5) var(--mr-space-6) var(--mr-space-4);

      &--phone {
        height: calc(100dvh - 92px);
        max-height: none;
        padding: var(--mr-space-2) 2px var(--mr-space-2);
      }
    }

    .sheet__handle {
      align-self: center;
      width: 36px;
      height: 4px;
      border-radius: var(--mr-radius-pill);
      background: var(--mr-control-line);
    }

    .sheet__head {
      display: flex;
      align-items: flex-start;
      justify-content: space-between;
      gap: 12px;
    }

    .sheet__title {
      margin: 0;
      font-family: var(--mr-font-display);
      font-weight: 800;
      font-size: 26px;
      line-height: 30px;
    }

    .sheet__sub {
      margin: 2px 0 0;
      font-size: 14px;
      line-height: 19px;
      color: var(--mr-ink-muted);
    }

    .sheet__close {
      display: inline-flex;
      flex: none;
      align-items: center;
      justify-content: center;
      width: 44px;
      height: 44px;
      margin: -6px -8px 0 0;
      border: 0;
      background: none;
      color: var(--mr-ink);
      cursor: pointer;
    }

    .sheet__list {
      flex: 1;
      min-height: 0;
    }
  `,
})
export class LogSheet {
  private readonly sheet = injectSheet<LogSheetData>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;

  protected close(): void {
    this.sheet.close();
  }
}
