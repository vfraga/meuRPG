import {
  ChangeDetectionStrategy,
  Component,
  Injector,
  afterNextRender,
  computed,
  effect,
  inject,
  signal,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { ExhaustionLowerReason } from '../../../../gen/meurpg/play/v1/lasting_effects_service_pb';
import type { CombatState } from '../../../core/combat/combat-state';
import { ActionKey } from '../../../core/connect/idempotency';
import { focusWithRing } from '../../../core/creatures/focus-ring';
import { EffectsClient, type ExhaustionSubject } from '../../../core/effects/effects-client';
import { effectsErrorMessage } from '../../../core/effects/effects-errors';
import {
  EXHAUSTION_HINT,
  EXHAUSTION_LEVELS,
  MAX_EXHAUSTION,
} from '../../../core/effects/effects-text';
import { SelectField, type SelectOption } from '../../../shared/form-fields/select-field';
import { SheetFrame } from '../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../shared/sheet/sheet-host';

/** One creature the master may change the exhaustion of: its name, the level the screen shows and who it is for the server. */
export interface ExhaustionOption {
  readonly key: string;
  readonly label: string;
  readonly level: number;
  readonly subject: ExhaustionSubject;
}

/** What the page hands "Exaustão". */
export interface ExhaustionData {
  readonly campaignId: string;
  readonly options: readonly ExhaustionOption[];
  /** The option the dialog starts on (the first one when absent). */
  readonly selected?: string;
  /** The combat the combatants are in: it follows their level and takes the answer. `null` outside a combat. */
  readonly state: CombatState | null;
}

/** What the dialog answers: who changed and the level now. */
export interface ExhaustionResult {
  readonly key: string;
  readonly level: number;
}

/**
 * "Exaustão de Toren" (W7-E board 9): the master picks the level, 0 to 6, in radios (each level adds to the ones under it),
 * "Salvar", "Baixar 1 nível" and "Cancelar". The call always carries the level the screen shows (`expected_level`): if it
 * changed meanwhile the server refuses and the screen says so. Saving level 6 does not kill by itself: the dialog turns
 * into the question "Exaustão nível 6: Toren morre?" in danger-outline, with the focus on "Cancelar".
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-exhaustion-dialog',
  imports: [MatButtonModule, MatIconModule, SheetFrame, SelectField],
  templateUrl: './exhaustion-dialog.html',
  styleUrls: ['./effects-sheet.scss'],
})
export class ExhaustionDialog {
  private readonly api = inject(EffectsClient);
  private readonly sheet = injectSheet<ExhaustionData, ExhaustionResult>();
  private readonly injector = inject(Injector);
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly levels = EXHAUSTION_LEVELS;
  protected readonly hint = EXHAUSTION_HINT;

  protected readonly pickedKey = signal(this.data.selected ?? this.data.options[0]?.key ?? '');
  protected readonly option = computed(
    () => this.data.options.find((o) => o.key === this.pickedKey()) ?? this.data.options[0],
  );
  protected readonly whoOptions: SelectOption[] = this.data.options.map((o) => ({
    value: o.key,
    label: o.label,
  }));
  /** The level the screen shows now: the combat's own when it is a combatant of the running combat. */
  protected readonly current = computed(() => {
    const o = this.option();
    if (!o) {
      return 0;
    }
    if ('combatantId' in o.subject) {
      const id = o.subject.combatantId;
      const live = this.data.state?.encounter()?.combatants.find((c) => c.id === id);
      return live?.exhaustionLevel ?? o.level;
    }
    return o.level;
  });
  protected readonly level = signal(this.current());
  protected readonly confirming = signal(false);
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  private readonly key = new ActionKey();

  protected readonly who = computed(() => this.option()?.label ?? '');
  protected readonly title = computed(() =>
    this.confirming()
      ? `Exaustão nível ${MAX_EXHAUSTION}: ${this.who()} morre?`
      : `Exaustão de ${this.who()}`,
  );
  protected readonly changed = computed(() => this.level() !== this.current());

  protected pick(key: string): void {
    this.pickedKey.set(key);
    this.level.set(this.current());
    this.error.set('');
  }

  /** "Salvar": level 6 asks first. */
  protected save(): Promise<void> | void {
    if (!this.changed()) {
      return;
    }
    if (this.level() === MAX_EXHAUSTION && this.current() !== MAX_EXHAUSTION) {
      this.confirming.set(true);
      afterNextRender(
        () => focusWithRing(document.querySelector<HTMLElement>('[data-initial-focus]')),
        { injector: this.injector },
      );
      return;
    }
    return this.send(false);
  }

  protected confirmDeath(): Promise<void> {
    return this.send(true);
  }

  protected cancelConfirm(): void {
    this.confirming.set(false);
  }

  protected lower(): Promise<void> {
    if (this.current() === 0) {
      return Promise.resolve();
    }
    return this.run(
      (o, expected, key) =>
        this.api.lowerExhaustion(
          this.data.campaignId,
          o.subject,
          { by: 1, expectedLevel: expected, reason: ExhaustionLowerReason.MASTER },
          key,
        ),
      ['lower'],
    );
  }

  private send(confirmDeath: boolean): Promise<void> {
    const level = this.level();
    return this.run(
      (o, expected, key) =>
        this.api.setExhaustion(
          this.data.campaignId,
          o.subject,
          { level, expectedLevel: expected, confirmDeath },
          key,
        ),
      ['set', level, confirmDeath],
    );
  }

  private async run(
    call: (
      o: ExhaustionOption,
      expected: number,
      key: string,
    ) => Promise<{ level: number; encounter?: Parameters<CombatState['apply']>[0] }>,
    what: readonly unknown[],
  ): Promise<void> {
    const o = this.option();
    if (this.busy() || !o) {
      return;
    }
    const expected = this.current();
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await call(o, expected, this.key.keyFor([o.key, expected, ...what]));
      this.key.renew();
      if (res.encounter) {
        this.data.state?.apply(res.encounter);
      }
      this.sheet.close({ key: o.key, level: res.level });
    } catch (err) {
      this.confirming.set(false);
      this.error.set(effectsErrorMessage(err, 'mudar a exaustão', 'exhaustion'));
    } finally {
      this.busy.set(false);
    }
  }

  protected close(): void {
    if (!this.busy()) {
      this.sheet.close(undefined);
    }
  }
}
