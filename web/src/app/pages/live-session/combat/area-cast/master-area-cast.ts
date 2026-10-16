import {
  ChangeDetectionStrategy,
  Component,
  Injector,
  afterNextRender,
  computed,
  effect,
  inject,
  signal,
  untracked,
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { create } from '@bufbuild/protobuf';
import { MatIconModule } from '@angular/material/icon';

import { HiddenAreaHitRule } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  AreaPlacement,
  type Combatant,
  type SpellTargets,
  TargetInReachSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  type ActionEconomy,
  SpellAreaShape,
  type SpellDetails,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { TableRulesClient } from '../../../../core/campaigns/table-rules';
import { AreaFlow } from '../../../../core/combat/area-flow';
import {
  areaRows,
  confirmLabel,
  countText,
  masterAllyWarning,
  nobodyText,
  shapeText,
  stepOneTitle,
} from '../../../../core/combat/area-text';
import { castSubtitle, saveAbility, spellKind } from '../../../../core/combat/cast-flow';
import { CombatClient, type SlotRef } from '../../../../core/combat/combat-client';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import { circleLabel } from '../../../../core/combat/combat-grid';
import type { CombatState } from '../../../../core/combat/combat-state';
import { isPlayer } from '../../../../core/combat/combat-view';
import { isCreature } from '../../../../core/combat/creature-names';
import { SpellCatalog } from '../../../../core/combat/spell-catalog';
import { ActionKey } from '../../../../core/connect/idempotency';
import type { AreaShape } from '../../../../core/combat/spell-area';
import { AreaList } from '../../../../shared/area-picker/area-list';
import { AreaMap } from '../../../../shared/area-picker/area-map';
import { AreaStep } from '../../../../shared/area-picker/area-step';
import { HiddenSwitch } from '../../../../shared/hidden-switch/hidden-switch';
import { injectSheet } from '../../../../shared/sheet/sheet-host';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import type { CastMapData } from '../cast-sheet/cast-sheet';

/** What the page hands the master's area cast. */
export interface MasterAreaCastData {
  readonly campaignId: string;
  readonly encounterId: string;
  readonly caster: Combatant;
  readonly spellKey: string;
  readonly name: string;
  /** 0 for a cantrip. */
  readonly level: number;
  /** The slot the NPC casts with (its spell slots are not counted); `null` for a cantrip. */
  readonly slot: SlotRef | null;
  readonly economy: ActionEconomy;
  readonly targets: SpellTargets;
  readonly map: CastMapData;
  readonly state: CombatState;
}

/**
 * "Zuk conjura Bola de Fogo" (PM-02d state 11): the master casts an NPC's area spell with the same picker as the player,
 * in a dialog (the map on the left, who is inside on the right from a laptop up; one column on a phone). The master sees
 * everyone, the hidden ones marked "Escondido", and is held to neither the range (the dimming is a hint) nor the near side
 * of a wall. When the area holds at least one hidden creature, "Revelar as escondidas atingidas" (`role="switch"`) says
 * whether they appear to the players: it starts on the table's rule (on for "Revelar" and "Perguntar a cada vez", off for
 * "Manter escondidas") and goes with the cast as `reveal_hidden`.
 */
@Component({
  selector: 'app-master-area-cast',
  imports: [AreaList, AreaMap, AreaStep, HiddenSwitch, MatButtonModule, MatIconModule, SheetFrame],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './master-area-cast.html',
  styleUrl: './master-area-cast.scss',
})
export class MasterAreaCast {
  private readonly api = inject(CombatClient);
  private readonly rules = inject(TableRulesClient);
  private readonly catalog = inject(SpellCatalog);
  private readonly injector = inject(Injector);
  private readonly sheet = injectSheet<MasterAreaCastData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;

  protected readonly placement = this.data.targets.placement;
  protected readonly Placement = AreaPlacement;
  protected readonly area: AreaShape = {
    shape: this.data.targets.areaShape ?? SpellAreaShape.UNSPECIFIED,
    sizeFt: this.data.targets.areaSizeFt,
    widthFt: this.data.targets.areaWidthFt,
  };
  protected readonly flow = new AreaFlow(this.placement, (area) =>
    this.api.previewSpellArea(
      this.data.campaignId,
      this.data.encounterId,
      this.data.caster.id,
      this.data.spellKey,
      this.data.slot,
      area,
    ),
  );
  protected readonly stepOne = stepOneTitle(this.placement);
  protected readonly confirmText = confirmLabel(this.placement);

  protected readonly details = signal<SpellDetails | null>(null);
  /** The table's rule on hidden creatures an area hits, read when the dialog opens (it may change mid-session). */
  protected readonly rule = signal<HiddenAreaHitRule | null>(null);
  /** The master's own choice on the switch; `null` until he touches it. */
  private readonly chosen = signal<boolean | null>(null);
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  private readonly key = new ActionKey();
  private readonly listTitle = viewChild<AreaStep>('listTitle');

  protected readonly combatants = computed(() => this.data.state.encounter()?.combatants ?? []);
  /** Everyone placed on the map, hidden ones too: "Centrar em…" for the master. */
  protected readonly seen = computed(() =>
    this.combatants()
      .filter((c) => c.placed && c.id !== this.data.caster.id)
      .map((c) => create(TargetInReachSchema, { combatantId: c.id, label: c.label })),
  );
  protected readonly npcs = computed(
    () =>
      new Set(
        this.combatants()
          .filter((c) => !isPlayer(c) && !isCreature(c))
          .map((c) => c.id),
      ),
  );
  protected readonly creatures = computed(
    () =>
      new Set(
        this.combatants()
          .filter(isCreature)
          .map((c) => c.id),
      ),
  );
  protected readonly subtitle = computed(() => {
    const parts = [
      'NPC',
      castSubtitle(
        this.data.economy,
        spellKind(this.data.spellKey, this.details()),
        this.details(),
        this.data.slot?.level ?? this.data.level,
        0,
        '',
        '',
        shapeText(this.area),
      ),
    ];
    if (this.data.slot) {
      parts.push(`espaço de ${circleLabel(this.data.slot.level)}`);
    }
    return parts.join(' · ');
  });
  protected readonly rows = computed(() => {
    const p = this.flow.preview();
    return p ? areaRows(p.targets, p.coverCounts, saveAbility(this.details()), this.placement) : [];
  });
  protected readonly count = computed(() => countText(this.flow.preview()?.targets ?? []));
  protected readonly warning = computed(() => {
    const p = this.flow.preview();
    return p ? masterAllyWarning(p.targets, this.data.caster.label) : null;
  });
  protected readonly hidden = computed(
    () =>
      this.flow
        .preview()
        ?.targets.filter((t) => t.hidden)
        .map((t) => t.label) ?? [],
  );
  /** The switch's value: the master's choice, else the table's rule (on unless it keeps them hidden). */
  protected readonly reveal = computed(
    () => this.chosen() ?? this.rule() !== HiddenAreaHitRule.KEEP_HIDDEN,
  );
  protected readonly switchHint = computed(() => {
    const names = this.hidden();
    const who =
      names.length === 1
        ? `${names[0]} aparece`
        : `${names.slice(0, -1).join(', ')} e ${names.at(-1)} aparecem`;
    const rule =
      this.rule() === HiddenAreaHitRule.KEEP_HIDDEN
        ? '“Manter escondidas”'
        : this.rule() === HiddenAreaHitRule.ASK
          ? '“Perguntar a cada vez”'
          : '“Revelar”';
    return this.reveal()
      ? `${who} para os jogadores depois da magia. Começa na regra da mesa (${rule}).`
      : `Continua${names.length === 1 ? '' : 'm'} escondida${names.length === 1 ? '' : 's'} para os jogadores. Começa na regra da mesa (${rule}).`;
  });
  protected readonly nobody = computed(() =>
    this.flow.nobody() && !this.flow.nobodyAccepted()
      ? nobodyText(true, this.data.slot?.level ?? 0, false)
      : null,
  );

  constructor() {
    void this.catalog
      .details(this.data.campaignId, this.data.spellKey)
      .then((d) => this.details.set(d));
    void this.rules.get(this.data.campaignId).then(
      (vm) => this.rule.set(vm.saved.hiddenAreaHits),
      () => this.rule.set(HiddenAreaHitRule.REVEAL),
    );
    void this.flow.start();
    effect(() => this.sheet.lock(this.busy()));
    effect(() => {
      if (this.flow.step() === 'list') {
        untracked(() =>
          afterNextRender(() => this.listTitle()?.focus(), { injector: this.injector }),
        );
      }
    });
  }

  protected toggle(on: boolean): void {
    this.chosen.set(on);
  }

  protected confirm(): Promise<boolean> {
    return this.flow.confirm();
  }

  protected back(): void {
    this.flow.back();
  }

  protected async cast(): Promise<void> {
    if (this.busy() || this.flow.step() !== 'list') {
      return;
    }
    if (this.nobody()) {
      this.flow.nobodyAccepted.set(true);
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const area = this.flow.choice();
      // The choice goes with the cast only when there is someone hidden to decide on; otherwise the rule stands.
      const revealHidden = this.hidden().length > 0 ? this.reveal() : undefined;
      const die =
        spellKind(this.data.spellKey, this.details()) === 'pool' ? { inApp: true as const } : null;
      const res = await this.api.castSpell(
        this.data.campaignId,
        this.data.encounterId,
        this.data.caster.id,
        this.data.spellKey,
        this.data.slot,
        [],
        die,
        this.key.keyFor([this.data.slot, area, revealHidden]),
        undefined,
        '',
        undefined,
        [],
        { area, revealHidden },
      );
      this.data.state.apply(res.encounter);
      this.sheet.close(true);
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'conjurar a magia'));
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
