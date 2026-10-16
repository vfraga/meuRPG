import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  signal,
  untracked,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { Combatant } from '../../../../gen/meurpg/play/v1/combat_pb';
import {
  type CatalogEffect,
  EffectAudience,
  EffectDurationKind,
} from '../../../../gen/meurpg/play/v1/lasting_effects_pb';
import type { CombatState } from '../../../core/combat/combat-state';
import { ActionKey } from '../../../core/connect/idempotency';
import { type DurationSpec, EffectsClient } from '../../../core/effects/effects-client';
import { abilityMissing, needsAbility } from '../../../core/effects/ability-choice';
import { effectsErrorMessage } from '../../../core/effects/effects-errors';
import {
  MAX_LABEL,
  MAX_ROUNDS,
  MAX_TARGETS,
  defaultDurationText,
  validDc,
  validRounds,
} from '../../../core/effects/effects-text';
import { SelectField, type SelectOption } from '../../../shared/form-fields/select-field';
import { TextField } from '../../../shared/form-fields/text-field';
import { SheetFrame } from '../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../shared/sheet/sheet-host';
import { AbilityPicker } from './ability-picker/ability-picker';
import { VisibilityFields } from './visibility-fields';

/** What the page hands "Adicionar efeito". */
export interface AddEffectData {
  readonly campaignId: string;
  readonly encounterId: string;
  readonly state: CombatState;
  readonly catalog: readonly CatalogEffect[];
  /** The combatant whose turn it is: the first caster and target the form offers. */
  readonly currentCombatantId: string;
}

/** How the players' side of the new effect is left: the catalog's own, or the master's choice. */
type Visibility = 'catalog' | 'yes' | 'no';
type DurationChoice = 'catalog' | 'rounds' | 'end-turn' | 'start-turn' | 'dismissed';

const DURATION_OPTIONS: readonly { value: DurationChoice; label: string }[] = [
  { value: 'rounds', label: 'Outro número de rodadas' },
  { value: 'end-turn', label: 'Até o fim do turno de alguém' },
  { value: 'start-turn', label: 'Até o começo do turno de alguém' },
  { value: 'dismissed', label: 'Até o mestre encerrar' },
];

/**
 * "Adicionar um efeito" (W7-E board 4b): the master puts an effect of the catalog on one or more combatants, so a
 * Velocidade or a Teia can be set by hand. The target, the effect, who cast it, how long it lasts, the saving throw's DC
 * and what the players see. Left alone, the duration and the visibility are the catalog's own. A dialog from a tablet
 * up, a bottom sheet on a phone; the sheet scrolls inside itself with "Adicionar" and "Cancelar" in reach.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-add-effect-dialog',
  imports: [
    AbilityPicker,
    MatButtonModule,
    MatIconModule,
    SheetFrame,
    SelectField,
    TextField,
    VisibilityFields,
  ],
  templateUrl: './add-effect-dialog.html',
  styleUrls: ['./effects-sheet.scss'],
})
export class AddEffectDialog {
  private readonly api = inject(EffectsClient);
  private readonly sheet = injectSheet<AddEffectData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly maxTargets = MAX_TARGETS;

  protected readonly combatants = computed<readonly Combatant[]>(
    () => this.data.state.encounter()?.combatants ?? [],
  );
  protected readonly catalog = [...this.data.catalog].sort((a, b) =>
    a.namePt.localeCompare(b.namePt, 'pt-BR'),
  );

  protected readonly target = signal(this.firstTarget());
  protected readonly moreTargets = signal<ReadonlySet<string>>(new Set());
  protected readonly effectKey = signal(this.catalog[0]?.key ?? '');
  protected readonly casterId = signal('');
  protected readonly durationChoice = signal<DurationChoice>('catalog');
  protected readonly rounds = signal('10');
  protected readonly anchorId = signal(this.data.currentCombatantId || this.firstTarget());
  protected readonly dc = signal('');
  protected readonly ability = signal('');
  protected readonly visibility = signal<Visibility>('catalog');
  protected readonly audience = signal<EffectAudience>(EffectAudience.ALL);
  protected readonly label = signal('');
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  private readonly key = new ActionKey();

  protected readonly chosen = computed(() => this.catalog.find((c) => c.key === this.effectKey()));

  protected readonly targetOptions = computed<SelectOption[]>(() =>
    this.combatants().map((c) => ({ value: c.id, label: c.label })),
  );
  protected readonly effectOptions = computed<SelectOption[]>(() =>
    this.catalog.map((c) => ({ value: c.key, label: c.namePt })),
  );
  protected readonly casterOptions = computed<SelectOption[]>(() => [
    { value: '', label: 'Alguém fora do combate' },
    ...this.combatants().map((c) => ({ value: c.id, label: c.label })),
  ]);
  protected readonly durationOptions = computed<SelectOption[]>(() => {
    const c = this.chosen();
    return [
      { value: 'catalog', label: c ? defaultDurationText(c) : 'Padrão do catálogo' },
      ...DURATION_OPTIONS,
    ];
  });
  protected readonly anchorOptions = computed<SelectOption[]>(() => this.targetOptions());
  /** The ones the first target is not: the list of "Mais alvos". */
  protected readonly others = computed(() =>
    this.combatants().filter((c) => c.id !== this.target()),
  );

  protected readonly roundsNumber = computed(() => Number(this.rounds().trim() || NaN));
  protected readonly roundsIssues = computed(() =>
    this.durationChoice() === 'rounds' && !validRounds(this.roundsNumber())
      ? [`Digite de 1 a ${MAX_ROUNDS} rodadas.`]
      : [],
  );
  protected readonly dcIssues = computed(() =>
    this.dc().trim() !== '' && !validDc(Number(this.dc().trim()))
      ? ['Digite uma CD de 1 a 40.']
      : [],
  );
  protected readonly asksAbility = computed(() => needsAbility(this.effectKey()));
  protected readonly targetIds = computed(() => [this.target(), ...this.moreTargets()]);
  protected readonly valid = computed(
    () =>
      !!this.target() &&
      !!this.chosen() &&
      this.targetIds().length <= MAX_TARGETS &&
      this.roundsIssues().length === 0 &&
      this.dcIssues().length === 0 &&
      abilityMissing(this.effectKey(), this.ability()) === '',
  );

  constructor() {
    // The caster starts as who is on turn when the effect has one and that is not the target (the caster concentrates on
    // a spell, never the target it was put on: SRD 5.1, Concentration); otherwise "alguém fora do combate", which no
    // combatant concentrates on. The form follows the effect picked.
    effect(() => {
      const c = this.chosen();
      const turn = this.data.currentCombatantId;
      this.casterId.set(c?.hasCaster && turn !== untracked(() => this.target()) ? turn : '');
      if (c?.defaultDurationKind === EffectDurationKind.ROUNDS && c.defaultRounds > 0) {
        this.rounds.set(String(c.defaultRounds));
      }
    });
  }

  private firstTarget(): string {
    const list = this.data.state.encounter()?.combatants ?? [];
    return list.find((c) => c.id === this.data.currentCombatantId)?.id ?? list[0]?.id ?? '';
  }

  protected setTarget(id: string): void {
    this.target.set(id);
    const more = new Set(this.moreTargets());
    more.delete(id);
    this.moreTargets.set(more);
  }

  protected toggleMore(id: string): void {
    const more = new Set(this.moreTargets());
    if (!more.delete(id)) {
      more.add(id);
    }
    this.moreTargets.set(more);
  }

  protected moreFull(id: string): boolean {
    return !this.moreTargets().has(id) && this.targetIds().length >= MAX_TARGETS;
  }

  private duration(): DurationSpec {
    const first = this.target();
    switch (this.durationChoice()) {
      case 'rounds':
        return { kind: EffectDurationKind.ROUNDS, rounds: this.roundsNumber() };
      case 'end-turn':
        return {
          kind: EffectDurationKind.UNTIL_END_OF_TURN_OF,
          anchorCombatantId: this.anchorId(),
        };
      case 'start-turn':
        return {
          kind: EffectDurationKind.UNTIL_START_OF_TURN_OF,
          anchorCombatantId: this.anchorId(),
        };
      case 'dismissed':
        return { kind: EffectDurationKind.UNTIL_DISMISSED };
      default: {
        const c = this.chosen()!;
        return {
          kind: c.defaultDurationKind,
          rounds: c.defaultRounds || undefined,
          anchorCombatantId: needsAnchor(c.defaultDurationKind) ? first : undefined,
        };
      }
    }
  }

  protected async add(): Promise<void> {
    const effectChosen = this.chosen();
    if (this.busy() || !this.valid() || !effectChosen) {
      return;
    }
    const visibility = this.visibility();
    const spec = {
      campaignId: this.data.campaignId,
      encounterId: this.data.encounterId,
      targetIds: this.targetIds(),
      catalogKey: effectChosen.key,
      casterId: this.casterId() || undefined,
      duration: this.duration(),
      saveDc: this.dc().trim() === '' ? undefined : Number(this.dc().trim()),
      playerVisible: visibility === 'catalog' ? undefined : visibility === 'yes',
      audience: visibility === 'yes' ? this.audience() : undefined,
      playerLabel: visibility === 'yes' ? this.label().trim().slice(0, MAX_LABEL) : undefined,
      abilityKey: this.asksAbility() ? this.ability() : undefined,
    };
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.add(spec, this.key.keyFor(spec));
      this.key.renew();
      if (res.encounter) {
        this.data.state.apply(res.encounter);
      }
      this.sheet.close(true);
    } catch (err) {
      this.error.set(effectsErrorMessage(err, 'adicionar o efeito', 'combat'));
    } finally {
      this.busy.set(false);
    }
  }

  protected close(): void {
    if (!this.busy()) {
      this.sheet.close(false);
    }
  }
}

function needsAnchor(kind: EffectDurationKind): boolean {
  return (
    kind === EffectDurationKind.UNTIL_START_OF_TURN_OF ||
    kind === EffectDurationKind.UNTIL_END_OF_TURN_OF
  );
}
