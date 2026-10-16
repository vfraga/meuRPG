import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  effect,
  inject,
  input,
  signal,
  untracked,
} from '@angular/core';
import { type ComponentType } from '@angular/cdk/portal';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';

import {
  EffectAudience,
  EffectEndScope,
  type LastingEffect,
  type TurnClockEntry,
} from '../../../../gen/meurpg/play/v1/lasting_effects_pb';
import type { ListLastingEffectsResponse } from '../../../../gen/meurpg/play/v1/lasting_effects_service_pb';
import type { CombatState } from '../../../core/combat/combat-state';
import { ActionKey } from '../../../core/connect/idempotency';
import { focusWithRing } from '../../../core/creatures/focus-ring';
import { EffectsClient } from '../../../core/effects/effects-client';
import { effectsErrorMessage } from '../../../core/effects/effects-errors';
import {
  cardSubtitle,
  clockTitle,
  endConcentrationText,
  endLine,
  groupOf,
  panelSubtitle,
  saveLine,
  tagsLine,
  targetsText,
  uniqueNames,
  uniqueTargets,
  visibilityText,
} from '../../../core/effects/effects-text';
import { openSheet } from '../../../shared/sheet/sheet-host';
import { AddEffectDialog, type AddEffectData } from './add-effect-dialog';
import { DurationDialog, type DurationData } from './duration-dialog';
import { ExhaustionDialog, type ExhaustionData, type ExhaustionOption } from './exhaustion-dialog';
import { VisibilityDialog, type VisibilityData } from './visibility-dialog';

/** A row of the panel with what the template reads, computed once. */
interface Row {
  readonly effect: LastingEffect;
  readonly targets: string;
  readonly subtitle: string;
  readonly end: string;
  readonly save: string;
  readonly tags: string;
  readonly see: string;
  /** What the phone's pill says after "Jogadores veem: ". */
  readonly seeShort: string;
  /** More than one target: the button names the effect and "Tirar de um alvo" shows. */
  readonly several: boolean;
  /** The sentence that confirms ending the whole concentration. */
  readonly question: string;
  readonly questionTitle: string;
}

/**
 * "Efeitos em jogo" (W7-E board 4, the master's page of a running combat): every effect with what it is, on whom, whose
 * it is, how it ends and what the players see, and under it the clock of the turns, in the order of the initiative from
 * the turn that is running: what the clock will do next (a saving throw at the end of a turn, an effect that ends at
 * the start of one). From a tablet up the effects are a table of focusable rows; on a phone, a card each, with the
 * buttons at full width. "Encerrar" on a concentration asks first, in place, in danger-outline with the focus on
 * "Cancelar": it cuts every effect of the casting and cannot be undone. The panel reads again after its own writes and
 * whenever the combat changes (`encounter_changed`, `combat_log_changed`), so it never shows an older clock.
 */
@Component({
  selector: 'app-effects-panel',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatButtonModule, MatIconModule],
  templateUrl: './effects-panel.html',
  styleUrl: './effects-panel.scss',
})
export class EffectsPanel {
  readonly campaignId = input.required<string>();
  readonly state = input.required<CombatState>();

  private readonly api = inject(EffectsClient);
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);

  protected readonly data = signal<ListLastingEffectsResponse | null>(null);
  protected readonly loadError = signal('');
  protected readonly error = signal('');
  protected readonly done = signal('');
  /** The effect an "Encerrar" question is open for. */
  protected readonly asking = signal<string | null>(null);
  /** The effect "Tirar de um alvo" is open for. */
  protected readonly removing = signal<string | null>(null);
  protected readonly busy = signal(false);

  private readonly endKey = new ActionKey();
  private readonly removeKey = new ActionKey();
  private reads = 0;

  private readonly encounterId = computed(() => this.state().encounter()?.id ?? '');

  protected readonly rows = computed<readonly Row[]>(() => {
    const all = this.data()?.effects ?? [];
    const concentrations = this.data()?.concentrations ?? [];
    return all.map((effect) => {
      const group = groupOf(effect, all);
      const names = uniqueNames(group);
      const caster = concentrations.find((c) => c.effectIds.includes(effect.id))?.casterLabel ?? '';
      return {
        effect,
        targets: targetsText(effect),
        subtitle: cardSubtitle(effect),
        end: endLine(effect),
        save: saveLine(effect),
        tags: tagsLine(effect),
        see: visibilityText(effect),
        seeShort: !effect.playerVisible
          ? 'Não'
          : effect.playersSeePt ||
            (effect.audience === EffectAudience.OWNER ? 'só o dono do alvo' : 'Sim'),
        several: effect.targetIds.length > 1,
        question: endConcentrationText(caster, names, uniqueTargets(group)),
        questionTitle: caster
          ? `Encerrar ${names.join(' e ')} de ${caster}?`
          : `Encerrar ${names.join(' e ')}?`,
      };
    });
  });

  protected readonly clock = computed(() => {
    const d = this.data();
    return (d?.turnClock ?? []).map((entry) => ({
      entry,
      title: clockTitle(entry, d?.effects ?? []),
      icon: entry.isSave ? 'casino' : 'schedule',
    }));
  });

  /** The turn that is running, by name. */
  protected readonly turnLabel = computed(() => {
    const id = this.data()?.currentCombatantId;
    return id
      ? (this.state()
          .encounter()
          ?.combatants.find((c) => c.id === id)?.label ?? '')
      : '';
  });
  protected readonly subtitle = computed(() =>
    panelSubtitle(this.rows().length, this.data()?.round ?? 0, this.turnLabel()),
  );

  constructor() {
    // The combat changed (a revision, a new log line): read the effects and the clock again.
    effect(() => {
      const e = this.state().encounter();
      this.state().logTick();
      if (e) {
        untracked(() => void this.reload(e.id));
      }
    });
  }

  /** Reads the panel again; an older answer never replaces a newer one. */
  async reload(encounterId = this.encounterId()): Promise<void> {
    if (!encounterId) {
      return;
    }
    const seq = ++this.reads;
    try {
      const res = await this.api.list(this.campaignId(), encounterId);
      if (seq === this.reads) {
        this.data.set(res);
        this.loadError.set('');
        this.pruneOpen(res);
      }
    } catch (err) {
      if (seq === this.reads) {
        this.loadError.set(effectsErrorMessage(err, 'ler os efeitos', 'combat'));
      }
    }
  }

  /** A question or a list open for an effect that is gone closes. */
  private pruneOpen(res: ListLastingEffectsResponse): void {
    const has = (id: string | null) => id !== null && res.effects.some((e) => e.id === id);
    if (!has(this.asking())) {
      this.asking.set(null);
    }
    if (!has(this.removing())) {
      this.removing.set(null);
    }
  }

  /** "Encerrar": a concentration asks first; anything else ends at once. */
  protected end(row: Row): void {
    if (this.busy()) {
      return;
    }
    this.removing.set(null);
    if (row.effect.concentration) {
      this.asking.set(row.effect.id);
      afterNextRender(
        () =>
          focusWithRing(
            this.host.nativeElement.querySelector<HTMLElement>(
              `[data-ask="${row.effect.id}"] [data-initial-focus]`,
            ),
          ),
        { injector: this.injector },
      );
      return;
    }
    void this.finishEnd(row.effect, EffectEndScope.THIS);
  }

  protected confirmEnd(row: Row): Promise<void> {
    return this.finishEnd(row.effect, EffectEndScope.CONCENTRATION_GROUP);
  }

  protected cancelAsk(row: Row): void {
    this.asking.set(null);
    focusWithRing(
      this.host.nativeElement.querySelector<HTMLElement>(`[data-end="${row.effect.id}"]`),
    );
  }

  private async finishEnd(lasting: LastingEffect, scope: EffectEndScope): Promise<void> {
    const encounterId = this.encounterId();
    if (this.busy() || !encounterId) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.end(
        this.campaignId(),
        encounterId,
        lasting.id,
        scope,
        this.endKey.keyFor([lasting.id, scope]),
      );
      this.endKey.renew();
      if (res.encounter) {
        this.state().apply(res.encounter);
      }
      this.asking.set(null);
      this.done.set(`Efeito encerrado: ${lasting.sourceNamePt}.`);
      await this.reload(encounterId);
      this.focusTitle();
    } catch (err) {
      this.error.set(effectsErrorMessage(err, 'encerrar o efeito', 'combat'));
    } finally {
      this.busy.set(false);
    }
  }

  protected toggleRemoving(row: Row): void {
    this.asking.set(null);
    this.removing.set(this.removing() === row.effect.id ? null : row.effect.id);
  }

  /** "Tirar Toren": takes one combatant off an effect that has several. */
  protected async removeTarget(row: Row, index: number): Promise<void> {
    const encounterId = this.encounterId();
    const combatantId = row.effect.targetIds[index];
    if (this.busy() || !encounterId || !combatantId) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const encounter = await this.api.removeTarget(
        this.campaignId(),
        encounterId,
        row.effect.id,
        combatantId,
        this.removeKey.keyFor([row.effect.id, combatantId]),
      );
      this.removeKey.renew();
      if (encounter) {
        this.state().apply(encounter);
      }
      this.removing.set(null);
      this.done.set(`${row.effect.sourceNamePt}: tirado de ${row.effect.targetLabels[index]}.`);
      await this.reload(encounterId);
      this.focusTitle();
    } catch (err) {
      this.error.set(effectsErrorMessage(err, 'tirar o efeito do alvo', 'combat'));
    } finally {
      this.busy.set(false);
    }
  }

  private focusTitle(): void {
    afterNextRender(
      () =>
        focusWithRing(this.host.nativeElement.querySelector<HTMLElement>('[data-effects-title]')),
      { injector: this.injector },
    );
  }

  protected openAdd(): void {
    const e = this.state().encounter();
    const catalog = this.data()?.catalog ?? [];
    if (!e) {
      return;
    }
    const data: AddEffectData = {
      campaignId: this.campaignId(),
      encounterId: e.id,
      state: this.state(),
      catalog,
      currentCombatantId: this.data()?.currentCombatantId ?? e.currentCombatantId,
    };
    this.open<AddEffectDialog, AddEffectData>(
      AddEffectDialog,
      data,
      'Adicionar um efeito',
      'add-effect-t',
    );
  }

  protected openDuration(row: Row): void {
    const e = this.state().encounter();
    if (!e) {
      return;
    }
    this.open<DurationDialog, DurationData>(
      DurationDialog,
      { campaignId: this.campaignId(), encounterId: e.id, state: this.state(), effect: row.effect },
      `Mudar a duração de ${row.effect.sourceNamePt} em ${row.effect.targetLabels.join(', ')}`,
      'duration-t',
    );
  }

  protected openVisibility(row: Row): void {
    const e = this.state().encounter();
    if (!e) {
      return;
    }
    this.open<VisibilityDialog, VisibilityData>(
      VisibilityDialog,
      { campaignId: this.campaignId(), encounterId: e.id, state: this.state(), effect: row.effect },
      `O que os jogadores veem de ${row.effect.sourceNamePt} em ${row.effect.targetLabels.join(', ')}`,
      'visibility-t',
    );
  }

  protected openExhaustion(): void {
    const e = this.state().encounter();
    if (!e) {
      return;
    }
    const options: ExhaustionOption[] = e.combatants.map((c) => ({
      key: c.id,
      label: c.label,
      level: c.exhaustionLevel,
      subject: { combatantId: c.id, encounterId: e.id },
    }));
    const data: ExhaustionData = {
      campaignId: this.campaignId(),
      options,
      selected: this.data()?.currentCombatantId,
      state: this.state(),
    };
    this.open<ExhaustionDialog, ExhaustionData>(ExhaustionDialog, data, 'Exaustão', 'exhaustion-t');
  }

  private open<C, D>(
    component: ComponentType<C>,
    data: D,
    ariaLabel: string,
    labelledBy: string,
  ): void {
    openSheet<C, D, unknown>(this.dialog, this.bottomSheet, component, {
      data,
      ariaLabel,
      labelledBy,
      width: '600px',
      tall: true,
    }).subscribe((result) => {
      if (result) {
        void this.reload();
      }
    });
  }

  protected trackClock(entry: TurnClockEntry): string {
    return `${entry.round}:${entry.combatantId}:${entry.phase}:${entry.effectId}:${entry.isSave}`;
  }
}
