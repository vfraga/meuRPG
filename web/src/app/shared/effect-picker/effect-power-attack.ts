import { ChangeDetectionStrategy, Component, computed, input, output } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import { POWER_COPY } from '../../core/content/effect-field-copy';
import { type EffectDraft, type EffectMenuVm } from '../../core/content/effect-draft';
import { withNumber } from '../../core/content/effect-number';
import { powerAttackIssues, powerLimits } from '../../core/content/power-attack-limits';
import { powerAttackReading } from '../../core/content/power-attack-reading';
import { UNREADABLE_COUNT, shownWhole } from '../../core/content/whole-number';
import { refusalTexts, titled } from './effect-picker-parts';
import { CheckRow } from '../form-fields/check-row';
import { FieldNote } from '../form-fields/field-note';
import { RadioCards, type RadioCard } from '../form-fields/radio-cards';
import { SearchPick, type SearchChip, type SearchOption } from '../form-fields/search-pick';
import { TextField } from '../form-fields/text-field';

/** The fields the block draws: the effect picker leaves them out of its own list of fields. */
export const POWER_BLOCK_FIELDS: ReadonlySet<string> = new Set([
  'attack_penalty',
  'damage_bonus',
  'weapon',
  'weapon_properties',
  'only_proficient',
]);

let nextId = 0;

/**
 * The block of a power attack ("Ataque poderoso", #586): what the trade takes off the attack roll and adds to the damage, with which
 * weapons it goes (the kind as three cards, the properties the weapon must all have as chips, the proficiency switch) and, under them,
 * "Vale para:", the filter read back in words. The resource, "Quando vale" and the text are the picker's own fields under "Mais opções".
 * Every control carries its path (`basePath` + the field) as `data-field`, so a refusal of the server lands on it.
 */
@Component({
  selector: 'app-effect-power-attack',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [CheckRow, FieldNote, MatIconModule, RadioCards, SearchPick, TextField],
  template: `
    <div class="power">
      <div class="power__numbers">
        <app-text-field
          [label]="'Tira do ataque'"
          inputmode="numeric"
          [value]="shown('attack_penalty')"
          [path]="path('attack_penalty')"
          [issues]="issuesAt('attack_penalty')"
          [hint]="penaltyHint()"
          (valueChange)="setNumber('attack_penalty', $event)"
        />
        <app-text-field
          [label]="'Soma no dano'"
          inputmode="numeric"
          [value]="shown('damage_bonus')"
          [path]="path('damage_bonus')"
          [issues]="issuesAt('damage_bonus')"
          [hint]="damageHint()"
          (valueChange)="setNumber('damage_bonus', $event)"
        />
      </div>
      <app-radio-cards
        label="Com que armas"
        [options]="cards"
        [value]="weapon()"
        [path]="path('weapon')"
        [issues]="issuesAt('weapon')"
        (valueChange)="setWeapon($event)"
      />
      <div class="power__props">
        <app-search-pick
          label="A arma precisa ter"
          [placeholder]="placeholder"
          emptyText="Nenhuma propriedade com esse nome."
          [options]="propertyOptions()"
          [chips]="propertyChips()"
          [path]="path('weapon_properties')"
          [issues]="issuesAt('weapon_properties')"
          [alsoDescribedBy]="propsHintId"
          (added)="addProperty($event)"
          (removed)="removeProperty($event)"
        />
        <p class="power__hint" [id]="propsHintId">{{ propertiesHint() }}</p>
      </div>
      <div>
        <app-check-row
          label="Só armas em que o personagem tem proficiência"
          [checked]="effect().onlyProficient"
          [path]="path('only_proficient')"
          (toggled)="setProficient($event)"
        />
        <app-field-note [issues]="issuesAt('only_proficient')" />
      </div>
      <p class="power__reading" role="status" data-power-reading>
        <mat-icon aria-hidden="true">swords</mat-icon>
        <span><strong>{{ label }}</strong> {{ reading() }}</span>
      </p>
    </div>
  `,
  styleUrl: './effect-power-attack.scss',
})
export class EffectPowerAttack {
  readonly effect = input.required<EffectDraft>();
  readonly menu = input.required<EffectMenuVm>();
  /** "table_feat.effects[0]": the path the fields' own names are added to. */
  readonly basePath = input.required<string>();
  /** What the server refused at a path, in Portuguese. */
  readonly issuesOf = input<(path: string) => readonly string[]>(() => []);
  /** The effects of the entry: the resource a use spends is named from them. */
  readonly siblings = input<readonly EffectDraft[]>([]);
  readonly effectChange = output<EffectDraft>();

  protected readonly cards: readonly RadioCard[] = POWER_COPY.weaponCards;
  protected readonly placeholder = POWER_COPY.propertiesPlaceholder;
  protected readonly label = POWER_COPY.readingLabel;
  protected readonly propsHintId = `power-props-hint-${nextId++}`;

  private readonly limits = computed(() => powerLimits(this.menu()));
  protected readonly penaltyHint = computed(() => POWER_COPY.penaltyHint(this.limits().penalty));
  protected readonly damageHint = computed(() => POWER_COPY.damageHint(this.limits().damage));
  protected readonly propertiesHint = computed(() =>
    POWER_COPY.propertiesHint(this.limits().properties),
  );
  /** The editor's own refusals, before the server's: a number outside its range, too many properties. */
  private readonly own = computed(() => powerAttackIssues(this.effect(), this.limits()));
  protected readonly weapon = computed(() =>
    this.effect().weapon === 'any' ? '' : this.effect().weapon,
  );

  protected readonly propertyOptions = computed<SearchOption[]>(() => {
    const picked = new Set(this.effect().weaponProperties);
    return this.menu()
      .list('weapon_properties')
      .filter((o) => !picked.has(o.key))
      .map((o) => ({ value: o.key, label: o.namePt, hint: '', group: '' }));
  });

  protected readonly propertyChips = computed<SearchChip[]>(() => {
    const menu = this.menu();
    const max = this.limits().properties;
    const known = new Set(menu.list('weapon_properties').map((o) => o.key));
    return this.effect().weaponProperties.map((key, i) => ({
      value: key,
      label: menu.nameOf('weapon_properties', key),
      hint: '',
      tag: '',
      // The ones over the limit, and one the menu no longer has, are outlined: they are what the refusal is about.
      invalid: i >= max || (known.size > 0 && !known.has(key)),
    }));
  });

  protected readonly reading = computed(() => {
    const menu = this.menu();
    const resource = this.siblings().find(
      (e) => e.type === 'resource' && e.resource.trim() === this.effect().resource.trim(),
    );
    return powerAttackReading(this.effect(), {
      property: (key) => menu.nameOf('weapon_properties', key),
      resource: (key) => (resource ? titled(key) : key),
    });
  });

  protected path(field: string): string {
    return `${this.basePath()}.${field}`;
  }

  /** The refusals of a field: the server's and the editor's own, a sentence once; a number that reads as none says so. */
  protected issuesAt(field: string): readonly string[] {
    const own = this.own()[field];
    const unreadable = Number.isNaN(this.numberOf(field)) ? [UNREADABLE_COUNT] : [];
    return [
      ...new Set([
        ...refusalTexts(this.effect(), field, this.issuesOf()(this.path(field))),
        ...(own ? [own] : []),
        ...unreadable,
      ]),
    ];
  }

  /** The number a field of the block holds; 0 for a field that is no number. */
  private numberOf(field: string): number {
    const e = this.effect();
    return field === 'attack_penalty'
      ? e.attackPenalty
      : field === 'damage_bonus'
        ? e.damageBonus
        : 0;
  }

  /** What a number field shows: the text typed while it still reads as the number, else the number. */
  protected shown(field: string): string {
    return shownWhole(this.numberOf(field), this.effect().typed?.[field]);
  }

  protected setNumber(name: string, text: string): void {
    this.effectChange.emit(withNumber(this.effect(), name, text, false));
  }

  protected setWeapon(value: string): void {
    this.effectChange.emit({ ...this.effect(), weapon: value });
  }

  protected setProficient(on: boolean): void {
    this.effectChange.emit({ ...this.effect(), onlyProficient: on });
  }

  protected addProperty(key: string): void {
    this.effectChange.emit({
      ...this.effect(),
      weaponProperties: [...this.effect().weaponProperties, key],
    });
  }

  protected removeProperty(key: string): void {
    this.effectChange.emit({
      ...this.effect(),
      weaponProperties: this.effect().weaponProperties.filter((k) => k !== key),
    });
  }
}
