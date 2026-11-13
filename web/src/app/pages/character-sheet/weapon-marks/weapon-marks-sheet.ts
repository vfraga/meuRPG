import { ChangeDetectionStrategy, Component, effect, signal } from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';

import type { MarksVm } from '../../../core/characters/gear';
import { SheetFrame } from '../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../../../shared/sheet/sheet-host';

/** What the sheet of a weapon's marks needs. `save` answers the error in words, or an empty text when the marks are saved. */
export interface WeaponMarksData {
  readonly weaponName: string;
  readonly characterName: string;
  /** The master edits at any time and says so; the owner is told when the sheet locks. */
  readonly isMaster: boolean;
  readonly marks: MarksVm;
  readonly save: (marks: MarksVm) => Promise<string>;
}

type MarkName = keyof MarksVm;

/** The three marks: the icon, the word and what each one beats (SRD 5.1: resistance and immunity to nonmagical attacks,
 * "silvered" and "adamantine" weapons; a magical weapon beats all three). */
const MARKS: readonly {
  readonly name: MarkName;
  readonly word: string;
  readonly icon: string;
  readonly beats: string;
}[] = [
  { name: 'silvered', word: 'Prateada', icon: 'dark_mode', beats: 'vence “nem prateado”' },
  { name: 'adamantine', word: 'Adamantina', icon: 'diamond', beats: 'vence “nem adamantino”' },
  {
    name: 'magical',
    word: 'Mágica',
    icon: 'auto_awesome',
    beats: 'vence os três: “não mágico”, “nem prateado”, “nem adamantino”',
  },
];

/** Opens "Marcas da <arma>": a bottom sheet on a phone, a dialog from a tablet up. It answers `true` when the marks were saved. */
export function openWeaponMarks(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: WeaponMarksData,
) {
  return openSheet<WeaponMarksSheet, WeaponMarksData, boolean>(
    dialog,
    bottomSheet,
    WeaponMarksSheet,
    {
      data,
      ariaLabel: `Marcas da ${data.weaponName}`,
      labelledBy: 'sheet-t',
      width: '480px',
    },
  );
}

/**
 * "Marcas da Espada curta" (from the weapon's "⋮"): "A arma é:" and one row for each mark, a checkbox, its icon, its
 * word and what it beats. "Salvar marcas" saves over the whole sheet (the page does it, at the revision it holds); a
 * refusal is said here and the sheet stays open, with the toggles as they were. In a dialog "Cancelar" stands beside
 * "Salvar marcas", the same size; on a phone the ✕ closes it.
 */
@Component({
  selector: 'app-weapon-marks-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatButtonModule, MatIconModule, SheetFrame],
  templateUrl: './weapon-marks-sheet.html',
  styleUrl: './weapon-marks-sheet.scss',
})
export class WeaponMarksSheet {
  private readonly sheet = injectSheet<WeaponMarksData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly options = MARKS;
  protected readonly draft = signal<MarksVm>(this.data.marks);
  protected readonly busy = signal(false);
  protected readonly error = signal('');

  constructor() {
    // A save in the air cannot be dismissed: its answer is always shown.
    effect(() => this.sheet.lock(this.busy()));
  }

  protected set(name: MarkName, on: boolean): void {
    this.draft.update((d) => ({ ...d, [name]: on }));
  }

  protected async save(): Promise<void> {
    if (this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const why = await this.data.save(this.draft());
      if (why) {
        this.error.set(why);
      } else {
        this.sheet.close(true);
      }
    } finally {
      this.busy.set(false);
    }
  }

  protected cancel(): void {
    if (!this.busy()) {
      this.sheet.close();
    }
  }
}
