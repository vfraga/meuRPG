import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  signal,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import {
  type CatalogEffect,
  EffectAudience,
  EffectDurationKind,
} from '../../../../gen/meurpg/play/v1/lasting_effects_pb';
import { ActionKey } from '../../../core/connect/idempotency';
import { abilityMissing, needsAbility } from '../../../core/effects/ability-choice';
import { EffectsClient } from '../../../core/effects/effects-client';
import { effectsErrorMessage } from '../../../core/effects/effects-errors';
import {
  MAX_LABEL,
  MAX_SECONDS,
  MAX_TARGETS,
  defaultDurationText,
  elapsedText,
  validSeconds,
} from '../../../core/effects/effects-text';
import { SelectField, type SelectOption } from '../../../shared/form-fields/select-field';
import { TextField } from '../../../shared/form-fields/text-field';
import { SheetFrame } from '../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../shared/sheet/sheet-host';
import { AbilityPicker } from './ability-picker/ability-picker';
import { VisibilityFields } from './visibility-fields';

/** What the panel hands "Dar efeito": the characters the master may pick and the catalog read from the server. */
export interface AddCharacterEffectData {
  readonly campaignId: string;
  readonly characters: readonly { readonly id: string; readonly name: string }[];
  readonly catalog: readonly CatalogEffect[];
}

/** What the dialog answers when an effect was given: the effect's name and who got it, for the panel's notice. */
export interface AddCharacterEffectResult {
  readonly effectName: string;
  readonly characterNames: readonly string[];
}

type Visibility = 'catalog' | 'yes' | 'no';
type DurationChoice = 'catalog' | 'time' | 'dismissed' | 'long-rest';

const CUSTOM = 'custom';

const DURATION_OPTIONS: readonly { value: DurationChoice; label: string }[] = [
  { value: 'time', label: 'Um tempo de jogo' },
  { value: 'dismissed', label: 'Até dispensar' },
  { value: 'long-rest', label: 'Até um descanso longo' },
];

const TIME_OPTIONS: readonly SelectOption[] = [
  { value: '6', label: '1 rodada (6 segundos)' },
  { value: '60', label: '1 minuto' },
  { value: '600', label: '10 minutos' },
  { value: '3600', label: '1 hora' },
  { value: '28800', label: '8 horas' },
  { value: CUSTOM, label: 'Outro tempo' },
];

/**
 * "Dar efeito" (W7-E): the master puts an effect of the catalog on player characters outside a combat, a potion or a
 * blessing given between fights. The characters (up to ten), the effect, how long it lasts in game time (a time, until
 * dismissed, until a long rest, or the catalog's own) and what the players see. A dialog from a tablet up, a bottom sheet
 * on a phone; the sheet scrolls inside itself with "Dar efeito" and "Cancelar" in reach.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-add-character-effect-dialog',
  imports: [
    AbilityPicker,
    MatButtonModule,
    MatIconModule,
    SheetFrame,
    SelectField,
    TextField,
    VisibilityFields,
  ],
  templateUrl: './add-character-effect-dialog.html',
  styleUrls: ['./effects-sheet.scss'],
})
export class AddCharacterEffectDialog {
  private readonly api = inject(EffectsClient);
  private readonly sheet = injectSheet<AddCharacterEffectData, AddCharacterEffectResult | null>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly maxTargets = MAX_TARGETS;
  protected readonly timeOptions = TIME_OPTIONS;
  protected readonly maxSeconds = MAX_SECONDS;

  protected readonly catalog = [...this.data.catalog].sort((a, b) =>
    a.namePt.localeCompare(b.namePt, 'pt-BR'),
  );

  protected readonly picked = signal<ReadonlySet<string>>(new Set());
  protected readonly effectKey = signal(this.catalog[0]?.key ?? '');
  protected readonly ability = signal('');
  protected readonly durationChoice = signal<DurationChoice>('catalog');
  protected readonly timeChoice = signal('600');
  protected readonly customSeconds = signal('60');
  protected readonly visibility = signal<Visibility>('catalog');
  protected readonly audience = signal<EffectAudience>(EffectAudience.ALL);
  protected readonly label = signal('');
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  private readonly key = new ActionKey();

  protected readonly chosen = computed(() => this.catalog.find((c) => c.key === this.effectKey()));
  protected readonly effectOptions = computed<SelectOption[]>(() =>
    this.catalog.map((c) => ({ value: c.key, label: c.namePt })),
  );
  protected readonly durationOptions = computed<SelectOption[]>(() => {
    const c = this.chosen();
    return [
      { value: 'catalog', label: c ? defaultDurationText(c) : 'Padrão do catálogo' },
      ...DURATION_OPTIONS,
    ];
  });
  protected readonly asksAbility = computed(() => needsAbility(this.effectKey()));

  protected readonly seconds = computed(() =>
    this.timeChoice() === CUSTOM
      ? Number(this.customSeconds().trim() || NaN)
      : Number(this.timeChoice()),
  );
  protected readonly secondsIssues = computed(() =>
    this.durationChoice() === 'time' &&
    this.timeChoice() === CUSTOM &&
    !validSeconds(this.seconds())
      ? [`Digite de 1 a ${MAX_SECONDS.toLocaleString('pt-BR')} segundos.`]
      : [],
  );
  protected readonly preview = computed(() =>
    this.durationChoice() === 'time' && validSeconds(this.seconds())
      ? `Dura ${elapsedText(this.seconds())} de tempo de jogo.`
      : '',
  );
  protected readonly full = computed(() => this.picked().size >= MAX_TARGETS);
  protected readonly valid = computed(
    () =>
      this.picked().size >= 1 &&
      !!this.chosen() &&
      this.secondsIssues().length === 0 &&
      abilityMissing(this.effectKey(), this.ability()) === '',
  );

  protected toggle(id: string): void {
    const next = new Set(this.picked());
    if (!next.delete(id)) {
      next.add(id);
    }
    this.picked.set(next);
  }

  protected off(id: string): boolean {
    return !this.picked().has(id) && this.full();
  }

  private duration(): { kind: EffectDurationKind; seconds: number } {
    switch (this.durationChoice()) {
      case 'time':
        return { kind: EffectDurationKind.ROUNDS, seconds: this.seconds() };
      case 'dismissed':
        return { kind: EffectDurationKind.UNTIL_DISMISSED, seconds: 0 };
      case 'long-rest':
        return { kind: EffectDurationKind.LONG_REST, seconds: 0 };
      default:
        return { kind: EffectDurationKind.UNSPECIFIED, seconds: 0 };
    }
  }

  protected async give(): Promise<void> {
    const effectChosen = this.chosen();
    if (this.busy() || !this.valid() || !effectChosen) {
      return;
    }
    const visibility = this.visibility();
    const { kind, seconds } = this.duration();
    const characterIds = this.data.characters.filter((c) => this.picked().has(c.id));
    const spec = {
      campaignId: this.data.campaignId,
      characterIds: characterIds.map((c) => c.id),
      catalogKey: effectChosen.key,
      durationKind: kind,
      seconds,
      playerVisible: visibility === 'catalog' ? undefined : visibility === 'yes',
      audience: visibility === 'yes' ? this.audience() : undefined,
      playerLabel: visibility === 'yes' ? this.label().trim().slice(0, MAX_LABEL) : undefined,
      abilityKey: this.asksAbility() ? this.ability() : undefined,
    };
    this.busy.set(true);
    this.error.set('');
    try {
      await this.api.addCharacterEffect(spec, this.key.keyFor(spec));
      this.key.renew();
      this.sheet.close({
        effectName: effectChosen.namePt,
        characterNames: characterIds.map((c) => c.name),
      });
    } catch (err) {
      this.error.set(effectsErrorMessage(err, 'dar o efeito', 'give'));
    } finally {
      this.busy.set(false);
    }
  }

  protected close(): void {
    if (!this.busy()) {
      this.sheet.close(null);
    }
  }
}
