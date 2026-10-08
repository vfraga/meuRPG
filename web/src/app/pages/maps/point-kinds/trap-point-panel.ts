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
  output,
  signal,
  untracked,
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';

import {
  type GetTrapNoticersResponse,
  type MapPoint,
  TrapState,
} from '../../../../gen/meurpg/maps/v1/maps_pb';
import {
  type TrapPreset,
  type TrapSeverity,
  TrapPassOutcome as Outcome,
  TrapSaveApplies as Applies,
  TrapTargets,
  TrapTrigger,
} from '../../../../gen/meurpg/rules/v1/rules_pb';
import { CONDITIONS } from '../../../core/combat/conditions';
import type { PointChanges } from '../../../core/maps/maps-client';
import { MapsClient } from '../../../core/maps/maps-client';
import { TrapPresets } from '../../../core/traps/trap-presets';
import { TrapNoticers } from '../../../shared/trap-noticers/trap-noticers';
import {
  ABILITIES,
  type AttackDraft,
  type ConditionDraft,
  DAMAGE_TYPES,
  type DamageDraft,
  MAX_CONDITION_PARTS,
  MAX_DAMAGE_PARTS,
  type SaveDraft,
  type TrapDraft,
  blankTrapDraft,
  hasTrapErrors,
  isTrapDirty,
  newAttack,
  newCondition,
  newDamage,
  newSave,
  presetSummary,
  trapChangesOf,
  trapDraftFromPreset,
  trapDraftOf,
  trapErrors,
} from './trap-draft';
import { PointFoot } from './point-foot';

/**
 * The panel of an Armadilha point (E9-02 3 and 4, MR-035): "Predefinições do SRD" (the eight sample traps, as radios, and
 * "Começar do zero"), the form they fill and "Quem notaria". The form: name, "Descrição para você", "CD para notar
 * (Percepção)" (optional: empty means nobody notices it alone) and "CD para achar (Investigação)", the warning that the
 * image is visible to the players, "Área de disparo" (1×1 to 4×4), "Gatilho", "Efeito" in parts the master adds and removes
 * as text actions (an attack, damage that always lands, a condition, a saving throw with what a failure and a pass do),
 * who is caught, and the state (Armada, Disparada, Desarmada). The hint under a save's DC and an attack's bonus is the SRD's
 * severity table as the server sends it, and the browser judges nothing; the DC to notice and the numbers of "Quem notaria"
 * are the server's (`GetTrapNoticers`). Nothing is saved until "Salvar ponto". Same contract as the other point panels.
 */
@Component({
  selector: 'app-trap-point-panel',
  imports: [
    MatButtonModule,
    MatFormFieldModule,
    MatIconModule,
    MatInputModule,
    PointFoot,
    TrapNoticers,
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './trap-point-panel.html',
  styleUrls: ['./point-panel-kinds.scss', './trap-point-panel.scss'],
})
export class TrapPointPanel {
  private readonly api = inject(MapsClient);
  private readonly presetsApi = inject(TrapPresets);
  private readonly injector = inject(Injector);

  readonly point = input.required<MapPoint>();
  readonly campaignId = input.required<string>();
  readonly saving = input(false);
  readonly error = input<string | null>(null);
  readonly focusName = input(false);

  readonly saveRequested = output<void>();
  readonly removeConfirmed = output<void>();
  readonly dirtyChange = output<boolean>();

  protected readonly Armed = TrapState.ARMED;
  protected readonly Triggered = TrapState.TRIGGERED;
  protected readonly Disarmed = TrapState.DISARMED;
  protected readonly Enter = TrapTrigger.ENTER;
  protected readonly Manual = TrapTrigger.MANUAL;
  protected readonly Area = TrapTargets.AREA;
  protected readonly ManualTargets = TrapTargets.MANUAL;
  protected readonly Half = Outcome.HALF;
  protected readonly NoneOutcome = Outcome.NONE;
  protected readonly Caught = Applies.CAUGHT;
  protected readonly Hit = Applies.HIT;
  protected readonly sizes = [1, 2, 3, 4] as const;
  protected readonly abilities = ABILITIES;
  protected readonly damageTypes = DAMAGE_TYPES;
  protected readonly conditions = CONDITIONS;
  protected readonly maxDamage = MAX_DAMAGE_PARTS;
  protected readonly maxConditions = MAX_CONDITION_PARTS;

  protected readonly presets = signal<readonly TrapPreset[]>([]);
  protected readonly severities = signal<readonly TrapSeverity[]>([]);
  protected readonly presetsFailed = signal(false);
  protected readonly draft = signal<TrapDraft>(blankTrapDraft());
  protected readonly show = signal(false);
  protected readonly noticers = signal<GetTrapNoticersResponse | undefined>(undefined);
  protected readonly noticersFailed = signal(false);

  protected readonly errors = computed(() => trapErrors(this.draft()));
  protected readonly shown = computed(() =>
    this.show() ? this.errors() : { parts: {} as Record<string, string> },
  );
  /** The state the form opened with: what the master has not touched is left to the server. */
  private readonly openedState = signal(TrapState.ARMED);
  protected readonly dirty = computed(() =>
    isTrapDirty(this.draft(), this.point(), this.openedState()),
  );
  protected readonly saved = computed(() => this.point().trap !== undefined);
  protected readonly saveSeverity = computed(() =>
    this.severities()
      .map((s) => `${s.namePt.toLocaleLowerCase('pt-BR')} ${s.saveDcMin} a ${s.saveDcMax}`)
      .join(' · '),
  );
  protected readonly attackSeverity = computed(() =>
    this.severities()
      .map(
        (s) => `${s.namePt.toLocaleLowerCase('pt-BR')} +${s.attackBonusMin} a +${s.attackBonusMax}`,
      )
      .join(' · '),
  );
  protected readonly areaWords = computed(() => {
    const n = this.draft().areaSize;
    return n === 1
      ? 'Só o quadrado do ponto.'
      : `Um bloco de ${n} × ${n} quadrados em volta do ponto. Quem entra na área para no primeiro quadrado dela.`;
  });

  private readonly nameField = viewChild('nameField', { read: ElementRef<HTMLInputElement> });
  private currentId = '';
  private noticersAsked = 0;

  constructor() {
    effect(() => {
      const campaignId = this.campaignId();
      void untracked(() =>
        this.presetsApi.list(campaignId).then(
          (res) => {
            this.presets.set(res.presets);
            this.severities.set(res.severities);
          },
          () => this.presetsFailed.set(true),
        ),
      );
    });
    effect(() => {
      const point = this.point();
      untracked(() => {
        if (point.id !== this.currentId) {
          this.currentId = point.id;
          this.reset();
          if (this.focusName()) {
            afterNextRender(() => this.nameField()?.nativeElement.focus(), {
              injector: this.injector,
            });
          }
        }
      });
    });
    effect(() => this.dirtyChange.emit(this.dirty()));
    // "Quem notaria": read when the trap is opened and again when it changes (its DC, its area, its place).
    effect(() => {
      const p = this.point();
      const t = p.trap;
      // Read again when the trap changes: its DC, its area, its state, its place.
      void [p.id, p.xBp, p.yBp, t?.noticeDc, t?.areaSize, t?.state];
      untracked(() => void this.readNoticers(p));
    });
  }

  private reset(): void {
    const draft = trapDraftOf(this.point());
    this.openedState.set(draft.state);
    this.draft.set(draft);
    this.show.set(false);
  }

  protected async readNoticers(point: MapPoint = this.point()): Promise<void> {
    const mine = ++this.noticersAsked;
    this.noticersFailed.set(false);
    try {
      const res = await this.api.getTrapNoticers(this.campaignId(), point.mapId, point.id);
      if (mine === this.noticersAsked) {
        this.noticers.set(res);
      }
    } catch {
      if (mine === this.noticersAsked) {
        this.noticersFailed.set(true);
      }
    }
  }

  // ---- the form ----

  protected patch(change: Partial<TrapDraft>): void {
    this.draft.update((d) => ({ ...d, ...change }));
  }

  protected text(field: 'name' | 'description' | 'noticeDc' | 'findDc', event: Event): void {
    this.patch({ [field]: (event.target as HTMLInputElement).value });
  }

  protected choosePreset(preset: TrapPreset): void {
    this.draft.set(trapDraftFromPreset(preset));
    this.show.set(false);
  }

  protected fromScratch(): void {
    this.draft.set(blankTrapDraft(this.draft().name));
    this.show.set(false);
  }

  // ---- the effect, in parts ----

  protected addAttack(): void {
    this.patch({ attack: newAttack() });
  }

  protected removeAttack(): void {
    this.patch({ attack: null });
  }

  protected patchAttack(change: Partial<AttackDraft>): void {
    const a = this.draft().attack;
    if (a) {
      this.patch({ attack: { ...a, ...change } });
    }
  }

  protected patchAttackDamage(change: Partial<DamageDraft>): void {
    const a = this.draft().attack;
    if (a) {
      this.patchAttack({ damage: { ...a.damage, ...change } });
    }
  }

  protected addDamage(): void {
    this.patch({ damage: [...this.draft().damage, newDamage()] });
  }

  protected removeDamage(i: number): void {
    this.patch({ damage: this.draft().damage.filter((_, n) => n !== i) });
  }

  protected patchDamage(i: number, change: Partial<DamageDraft>): void {
    this.patch({ damage: this.draft().damage.map((d, n) => (n === i ? { ...d, ...change } : d)) });
  }

  protected addCondition(): void {
    this.patch({ conditions: [...this.draft().conditions, newCondition()] });
  }

  protected removeCondition(i: number): void {
    this.patch({ conditions: this.draft().conditions.filter((_, n) => n !== i) });
  }

  protected patchCondition(i: number, change: Partial<ConditionDraft>): void {
    this.patch({
      conditions: this.draft().conditions.map((c, n) => (n === i ? { ...c, ...change } : c)),
    });
  }

  protected addSave(): void {
    this.patch({ save: newSave() });
  }

  protected removeSave(): void {
    this.patch({ save: null });
  }

  protected patchSave(change: Partial<SaveDraft>): void {
    const s = this.draft().save;
    if (s) {
      this.patch({ save: { ...s, ...change } });
    }
  }

  protected addFailDamage(): void {
    const s = this.draft().save;
    if (s) {
      this.patchSave({ failDamage: [...s.failDamage, newDamage()] });
    }
  }

  protected removeFailDamage(i: number): void {
    const s = this.draft().save;
    if (s) {
      this.patchSave({ failDamage: s.failDamage.filter((_, n) => n !== i) });
    }
  }

  protected patchFailDamage(i: number, change: Partial<DamageDraft>): void {
    const s = this.draft().save;
    if (s) {
      this.patchSave({
        failDamage: s.failDamage.map((d, n) => (n === i ? { ...d, ...change } : d)),
      });
    }
  }

  protected patchFailCondition(change: Partial<ConditionDraft> | null): void {
    const s = this.draft().save;
    if (s) {
      this.patchSave({
        failCondition:
          change === null ? null : { ...(s.failCondition ?? newCondition()), ...change },
      });
    }
  }

  /** What a preset is, under its name: the fall of a pit, or the parts of its effect ("1 perfurante, 2d10 veneno · resistência de Constituição"). */
  protected presetSub(p: TrapPreset): string {
    return presetSummary(p);
  }

  protected val(event: Event): string {
    return (event.target as HTMLInputElement | HTMLSelectElement).value;
  }

  protected num(event: Event): number {
    return Number(this.val(event));
  }

  /** What "Salvar ponto" sends; `null` when nothing changed or the form breaks a rule (the fields say which). */
  changes(): PointChanges | null {
    this.show.set(true);
    if (hasTrapErrors(this.errors())) {
      return null;
    }
    return trapChangesOf(this.draft(), this.point(), this.openedState());
  }

  discard(): void {
    this.reset();
  }

  /** Arrow keys inside a radio list or a segmented choice. */
  protected onKey(event: KeyboardEvent): void {
    const keys = ['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight'];
    if (!keys.includes(event.key)) {
      return;
    }
    event.preventDefault();
    const step = event.key === 'ArrowUp' || event.key === 'ArrowLeft' ? -1 : 1;
    const buttons = Array.from(
      (event.currentTarget as HTMLElement).querySelectorAll<HTMLElement>('[role="radio"]'),
    );
    const here = buttons.findIndex((b) => b === document.activeElement);
    const next = buttons[(Math.max(0, here) + step + buttons.length) % buttons.length];
    next?.focus();
    next?.click();
  }
}
