import { ChangeDetectionStrategy, Component, computed, input, output, signal } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { TableEntry } from '../../../gen/meurpg/rules/v1/table_content_pb';
import { SpellDurationKind } from '../../../gen/meurpg/rules/v1/rules_pb';
import {
  MAX_SUMMON_LIVE,
  NO_RESOURCE_TEXT,
  FIXED_OFF_TEXT,
  hasFixedOff,
  resetFixed,
  type SummonDraft,
  type SummonResource,
  emptySummon,
} from '../../core/content/summon-draft';
import {
  durationOf,
  durationOptions,
  durationValue,
  timeOf,
  timeOptions,
  timeValue,
} from '../../core/content/summon-options';
import { FieldNote } from '../form-fields/field-note';
import { RadioCards } from '../form-fields/radio-cards';
import { SelectField, type SelectOption } from '../form-fields/select-field';
import { SwitchField } from '../form-fields/switch-field';
import { TextField } from '../form-fields/text-field';
import { REPLACES_CARDS, TURN_CARDS } from './summon-cards';
import { SummonCreatures } from './summon-creatures';

let nextId = 0;

/**
 * "Invoca criaturas" on a feature's card (#578, board C2): a switch, and, when it is on, the creatures the holder may bring, how
 * many, how many at a time, the resource a use is spent from, the turn, what a new casting does to the old creatures, the time
 * and the duration. The holder casts it like a summoning spell with a use of the resource in the place of a slot. Off sends
 * no summon. Each refusal of the server sits under its own field (`basePath.summon.*`); the feature's own resources
 * (`resources`) are what the cost can be spent from, and with none the block says so and the save waits.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-summon-block',
  imports: [
    FieldNote,
    MatIconModule,
    RadioCards,
    SelectField,
    SummonCreatures,
    SwitchField,
    TextField,
  ],
  templateUrl: './summon-block.html',
  styleUrl: './summon-block.scss',
})
export class SummonBlock {
  protected readonly id = `sb-${nextId++}`;

  readonly summon = input.required<SummonDraft | null>();
  readonly resources = input.required<readonly SummonResource[]>();
  /** The feature's own path ("table_class.levels[2].features[0]"): the block's inputs carry `<it>.summon.*`. */
  readonly basePath = input.required<string>();
  readonly issuesOf = input<(path: string) => readonly string[]>(() => []);
  readonly campaignId = input.required<string>();
  readonly entries = input<readonly TableEntry[]>([]);
  readonly summonChange = output<SummonDraft | null>();

  protected readonly turnCards = TURN_CARDS;
  protected readonly repliesCards = REPLACES_CARDS;
  protected readonly maxLive = MAX_SUMMON_LIVE;
  protected readonly noResource = NO_RESOURCE_TEXT;
  /** What was switched off, kept while the card stays open, so a switch turned by mistake gives the creatures back. */
  private readonly stash = signal<SummonDraft | null>(null);

  protected readonly at = computed(() => `${this.basePath()}.summon`);
  protected path(field: string): string {
    return `${this.at()}.${field}`;
  }
  protected issues(field: string): readonly string[] {
    return this.issuesOf()(this.path(field));
  }

  /** The feature's resources, and a stored key it does not define as one more option: the server accepts the class's. */
  protected readonly resourceOptions = computed<SelectOption[]>(() => {
    const key = this.summon()?.resourceKey.trim() ?? '';
    const own = this.resources().map((r) => ({ value: r.key, label: r.label }));
    return key === '' || own.some((o) => o.value === key)
      ? own
      : [...own, { value: key, label: `${key} (da classe)` }];
  });
  protected readonly hasResource = computed(() => this.resourceOptions().length > 0);
  protected readonly fixedOff = computed(() => {
    const s = this.summon();
    return s !== null && hasFixedOff(s);
  });
  protected readonly fixedOffText = FIXED_OFF_TEXT;
  protected readonly timeOptions = computed(() => timeOptions(this.summon()!.time));
  protected readonly durationOptions = computed(() => durationOptions(this.summon()!.duration));
  protected readonly timed = computed(
    () => this.summon()?.duration.kind === SpellDurationKind.TIMED,
  );
  protected readonly instant = computed(() => {
    const kind = this.summon()?.duration.kind;
    return kind === SpellDurationKind.UNSPECIFIED || kind === SpellDurationKind.INSTANTANEOUS;
  });

  protected toggle(on: boolean): void {
    const s = this.summon();
    if (!on) {
      this.stash.set(s);
      this.summonChange.emit(null);
      return;
    }
    this.summonChange.emit(this.stash() ?? emptySummon(this.resources()[0]?.key ?? ''));
  }

  protected patch(p: Partial<SummonDraft>): void {
    this.summonChange.emit({ ...this.summon()!, ...p });
  }

  protected setDuration(value: string): void {
    const duration = durationOf(value, this.summon()!.duration);
    // Concentration needs a duration by time: a duration that is not one takes it back.
    this.patch({
      duration,
      concentration: duration.kind === SpellDurationKind.TIMED && this.summon()!.concentration,
    });
  }

  protected setTime(value: string): void {
    this.patch({ time: timeOf(value, this.summon()!.time) });
  }

  protected readonly timeValue = timeValue;
  protected readonly durationValue = durationValue;

  protected resetDefaults(): void {
    this.patchAll(resetFixed(this.summon()!));
  }

  private patchAll(s: SummonDraft): void {
    this.summonChange.emit(s);
  }
}
