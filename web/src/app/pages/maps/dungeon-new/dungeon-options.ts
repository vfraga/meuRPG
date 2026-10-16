import { ChangeDetectionStrategy, Component, computed, effect, input, output } from '@angular/core';
import { FormControl, ReactiveFormsModule } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';

import type {
  DungeonCorridorStyle,
  DungeonDoorMix,
  DungeonMask,
} from '../../../../gen/meurpg/maps/v1/dungeons_pb';
import type { OptionField } from '../../../core/maps/dungeon-errors';
import { dungeonSizeText } from '../../../core/maps/dungeon-layout';
import {
  CORRIDORS,
  DOOR_MIXES,
  type DungeonForm,
  MASKS,
  SIZE_PRESETS,
  STAIRS_MAX,
  shortSideOf,
  sideOf,
  wholeNumber,
} from '../../../core/maps/dungeon-options';
import { SQUARE_FT, metersNumber } from '../../../core/units';
import { type Choice, ChoiceRow } from '../../../shared/choice-row/choice-row';

/** The "Outro" of the size choice: the master types the longer side. */
const OTHER = -1;

/**
 * The options of "Gerar masmorra" (E10-05 1): size (presets and "Outro"), shape, the rooms' sides, corridors, doors, dead ends, stairs and the
 * seed with "Outra semente". Presentational: the page owns the form and the seed, asks the server for a preview on every change, and hands
 * back the reasons (its own checks and the server's refusals) by field; this draws each reason in place, with an icon and words, ties it to
 * its field (`aria-invalid`, `aria-describedby`) and never relies on colour alone. `locked` freezes everything while the map is created.
 */
@Component({
  selector: 'app-dungeon-options',
  imports: [
    ChoiceRow,
    MatButtonModule,
    MatFormFieldModule,
    MatIconModule,
    MatInputModule,
    ReactiveFormsModule,
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './dungeon-options.html',
  styleUrl: './dungeon-options.scss',
})
export class DungeonOptions {
  readonly form = input.required<DungeonForm>();
  /** The reason a field cannot be used, by field: the page's checks and the server's refusals. */
  readonly problems = input<Partial<Record<OptionField, string>>>({});
  readonly seedText = input.required<string>();
  readonly seedProblem = input<string | null>(null);
  readonly locked = input(false);

  readonly formChange = output<DungeonForm>();
  readonly seedChange = output<string>();
  readonly anotherSeed = output<void>();

  protected readonly masks = MASKS as readonly Choice<DungeonMask>[];
  protected readonly corridors = CORRIDORS as readonly Choice<DungeonCorridorStyle>[];
  protected readonly doorMixes = DOOR_MIXES as readonly (Choice<DungeonDoorMix> & {
    about: string;
  })[];
  protected readonly stairsMax = STAIRS_MAX;
  protected readonly sizeChoices: readonly Choice<number>[] = [
    ...SIZE_PRESETS.map((p) => ({ value: p.side, label: p.label })),
    { value: OTHER, label: 'Outro' },
  ];

  protected readonly sizeControl = new FormControl('', { nonNullable: true });
  protected readonly roomMinControl = new FormControl('', { nonNullable: true });
  protected readonly roomMaxControl = new FormControl('', { nonNullable: true });
  protected readonly seedControl = new FormControl('', { nonNullable: true });

  /** The chosen size choice: a preset's side, or "Outro". */
  protected readonly sizeValue = computed(() => this.form().preset ?? OTHER);
  protected readonly isOther = computed(() => this.form().preset === null);
  protected readonly sizeHint = computed(() => {
    const side = sideOf(this.form());
    if (side === null || this.problems().size) {
      return '';
    }
    const short = shortSideOf(side);
    return `${side} × ${short} quadrados · ${dungeonSizeText(side, short)}. Cada quadrado vale 1,5 m.`;
  });
  protected readonly roomsHint = computed(() => {
    const min = wholeNumber(this.form().roomMinText);
    const max = wholeNumber(this.form().roomMaxText);
    if (
      min === null ||
      max === null ||
      this.problems().room_side_min ||
      this.problems().room_side_max
    ) {
      return '';
    }
    return `Salas de ${metersNumber(min * SQUARE_FT)} m a ${metersNumber(max * SQUARE_FT)} m de lado.`;
  });
  protected readonly doorAbout = computed(
    () => this.doorMixes.find((d) => d.value === this.form().doors)?.about ?? '',
  );

  constructor() {
    // The fields hold their own text: sync them from the form when it changes from outside (a preset sets the text empty), not on each keystroke.
    effect(() => {
      const f = this.form();
      this.sync(this.sizeControl, f.sizeText);
      this.sync(this.roomMinControl, f.roomMinText);
      this.sync(this.roomMaxControl, f.roomMaxText);
    });
    effect(() => this.sync(this.seedControl, this.seedText()));
    // The reasons are the fields' errors (the red outline); the words are drawn under each field.
    effect(() => {
      const p = this.problems();
      this.mark(this.sizeControl, !!p.size && this.isOther());
      this.mark(this.roomMinControl, !!p.room_side_min);
      this.mark(this.roomMaxControl, !!p.room_side_max);
      this.mark(this.seedControl, this.seedProblem() !== null);
    });
    effect(() => {
      for (const c of [
        this.sizeControl,
        this.roomMinControl,
        this.roomMaxControl,
        this.seedControl,
      ]) {
        if (this.locked()) {
          c.disable({ emitEvent: false });
        } else {
          c.enable({ emitEvent: false });
        }
      }
    });
  }

  private sync(control: FormControl<string>, value: string): void {
    if (control.value !== value) {
      control.setValue(value, { emitEvent: false });
    }
  }

  private mark(control: FormControl<string>, invalid: boolean): void {
    if (invalid) {
      control.setErrors({ problem: true });
      control.markAsTouched();
    } else if (control.errors) {
      control.setErrors(null);
    }
  }

  protected setSize(value: number): void {
    if (value === OTHER) {
      this.formChange.emit({
        ...this.form(),
        preset: null,
        sizeText: this.form().sizeText || String(sideOf(this.form()) ?? ''),
      });
    } else {
      this.formChange.emit({ ...this.form(), preset: value });
    }
  }

  protected typeSize(): void {
    this.formChange.emit({ ...this.form(), preset: null, sizeText: this.sizeControl.value });
  }

  protected typeRooms(): void {
    this.formChange.emit({
      ...this.form(),
      roomMinText: this.roomMinControl.value,
      roomMaxText: this.roomMaxControl.value,
    });
  }

  protected set<K extends keyof DungeonForm>(key: K, value: DungeonForm[K]): void {
    this.formChange.emit({ ...this.form(), [key]: value });
  }

  protected setDeadends(event: Event): void {
    this.set('deadends', Number((event.target as HTMLInputElement).value));
  }

  protected moreStairs(delta: number): void {
    const next = Math.min(STAIRS_MAX, Math.max(0, this.form().stairs + delta));
    if (next !== this.form().stairs) {
      this.set('stairs', next);
    }
  }
}
