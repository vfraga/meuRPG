import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  effect,
  inject,
  untracked,
  viewChild,
  viewChildren,
} from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { AreaPlacement } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { effectivePreference } from '../../../../core/campaigns/dice-labels';
import type { SlotRow } from '../../../../core/combat/cast-flow';
import { CombatClient } from '../../../../core/combat/combat-client';
import { SpellCatalog } from '../../../../core/combat/spell-catalog';
import { grantLine } from '../../../../core/combat/combat-options';
import { castLabel } from '../../../../core/resources/metamagic';
import { ResourceClient } from '../../../../core/resources/resources-client';
import { AreaList } from '../../../../shared/area-picker/area-list';
import { AreaMap } from '../../../../shared/area-picker/area-map';
import { AreaStep } from '../../../../shared/area-picker/area-step';
import { TeleportMap } from '../../../../shared/area-picker/teleport-map';
import { HeldRollPrompt } from '../../../../shared/held-roll-prompt/held-roll-prompt';
import { RollAnimator } from '../../../../shared/roll-overlay/roll-animator';
import { RollPicker } from '../../../../shared/roll-picker/roll-picker';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../../shared/sheet/sheet-host';
import { SlotPicker } from '../../../../shared/slot-picker/slot-picker';
import { openSpellDetails } from '../../../../shared/spell-details/open-spell-details';
import { spellDetailsFromGen } from '../../../../shared/spell-details/spell-details-map';
import { SpellHelp } from '../../../../shared/spell-details/spell-help';
import { AbilityPicker } from '../../effects/ability-picker/ability-picker';
import { CheckRow } from '../../../../shared/form-fields/check-row';
import { MultiRoll } from '../../../../shared/multi-roll/multi-roll';
import { RollModePicker } from '../roll-mode/roll-mode-picker';
import { CastArea } from './cast-area';
import { CastCaller } from './cast-caller';
import { CastChoices } from './cast-choices';
import { CastDamage } from './cast-damage';
import { D20_FIELDS, CastMode } from './cast-mode';
import { CastQuestions } from './cast-questions';
import { CastResult, CastSlots } from './cast-result';
import type { CastSheetData } from './cast-sheet-types';
import { CastState } from './cast-state';
import { CastTargets } from './cast-targets';
import { CastTexts } from './cast-texts';
import { DamageTypePicker } from './damage-type-picker';
import { MetamagicPicker } from './metamagic-picker';
import { SculptPicker } from './sculpt-picker';
import { TableOptionPicker } from './table-option-picker';

export type { CastMapData, CastSheetData } from './cast-sheet-types';

/**
 * "Conjurar Mísseis Mágicos" (MR-014, E6-09): the player's cast in a bottom
 * sheet on a phone and a dialog from a tablet up. The slot (a radio group of
 * circles), the targets (one, several, or Magic Missile's darts), the last-slot
 * warning, and then one filled button, "Conjurar X": the slot and the action
 * are spent when it is pressed, not when the damage is rolled (MR-014). A spell
 * with an attack roll asks for the d20 there instead (RN-18, one for each
 * target). The result lists each target (the roll, the save, the dice) and the
 * damage still to roll comes right under it, in the same two ways of rolling as
 * an attack's. The slot count, the Escudo that lost its slot, the concentration
 * and "Sua ação foi usada" close it. The keys (one for the cast, one for each
 * damage) are made once, so a tap repeated after a lost answer never casts
 * twice. Focus: the title opens first; after the result it goes to "Voltar à
 * sua vez".
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-cast-sheet',
  imports: [
    AreaList,
    AreaMap,
    AreaStep,
    CastResult,
    CastSlots,
    CastTargets,
    AbilityPicker,
    CheckRow,
    DamageTypePicker,
    HeldRollPrompt,
    MatButtonModule,
    MetamagicPicker,
    MatIconModule,
    MultiRoll,
    RollModePicker,
    RollPicker,
    SculptPicker,
    SheetFrame,
    SlotPicker,
    SpellHelp,
    TableOptionPicker,
    TeleportMap,
  ],
  templateUrl: './cast-sheet.html',
  styleUrl: './cast-sheet.scss',
})
export class CastSheet {
  private readonly api = inject(CombatClient);
  /** Read when the player answers about the die: a sheet that never asks does not build the client. */
  private readonly lazyInjector = inject(Injector);
  private readonly animator = inject(RollAnimator);
  private readonly catalog = inject(SpellCatalog);
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);
  private readonly sheet = injectSheet<CastSheetData, boolean>();
  private readonly injector = inject(Injector);
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly canApp = this.data.diceMode !== DiceMode.PHYSICAL;
  /** The damage may be typed unless the master made everybody roll in the app. */
  protected readonly canTypeDamage = this.data.diceMode !== DiceMode.APP;
  protected readonly preferApp =
    effectivePreference(this.data.diceMode, this.data.preference) === DicePreference.APP;
  protected readonly Placement = AreaPlacement;
  protected readonly grantLine = grantLine;
  protected readonly d20Fields = D20_FIELDS;

  protected readonly s = new CastState(this.data);
  protected readonly ar = new CastArea(this.s, this.api);
  private readonly pickers = viewChildren(RollPicker);
  private readonly hooks = {
    changed: () => {
      this.caller.newCast();
      for (const picker of this.pickers()) {
        picker.clear();
      }
    },
    newCast: () => this.caller.newCast(),
  };
  protected readonly ch = new CastChoices(
    this.s,
    this.ar,
    this.hooks,
    this.canApp,
    this.canTypeDamage,
  );
  protected readonly md = new CastMode(this.s, this.ch, this.hooks);
  /** The cast of a spell attack held for a question (Lucky's, then the Bardic Inspiration die's): the sheet asks it before
   * the result is told (`CastQuestions`). */
  protected readonly questions = new CastQuestions({
    resources: () => this.lazyInjector.get(ResourceClient),
    campaignId: this.data.campaignId,
    name: this.data.name,
    state: this.data.state,
    animator: this.animator,
    busy: this.s.busy,
    error: this.s.error,
    typing: this.s.typing,
    showCast: (encounter, cast) => this.caller.show(encounter, cast),
  });
  /** The damage of the cast still to roll, with every damage and heal of the cast by ID (`CastDamage`). */
  protected readonly damage = new CastDamage({
    api: this.api,
    campaignId: this.data.campaignId,
    encounterId: this.data.encounterId,
    resume: this.data.resume ?? [],
    state: this.data.state,
    animator: this.animator,
    busy: this.s.busy,
    error: this.s.error,
  });
  protected readonly tx = new CastTexts(this.s, this.ch, this.ar, this.damage);
  protected readonly caller = new CastCaller(
    this.s,
    this.ch,
    this.ar,
    this.questions,
    this.damage,
    { api: this.api, animator: this.animator },
  );
  // What the specs next to this one reach by name.
  protected readonly offer = this.questions.held.offer;
  protected readonly cast = this.s.cast;
  protected readonly typing = this.s.typing;
  protected readonly error = this.s.error;
  protected readonly rows = this.ch.rows;
  /** "Conjurar e gastar 1 ponto" once an option is marked, else "Conjurar X". */
  protected readonly castText = computed(() => castLabel(this.data.name, this.ch.metaCost()));

  private readonly back = viewChild('back', { read: ElementRef<HTMLButtonElement> });
  private readonly frame = viewChild(SheetFrame);
  private readonly listTitle = viewChild<AreaStep>('listTitle');
  private readonly changePlace = viewChild('changePlace', { read: ElementRef<HTMLButtonElement> });
  private readonly backToList = viewChild('backToList', { read: ElementRef<HTMLButtonElement> });

  constructor() {
    void this.catalog.details(this.data.campaignId, this.data.spellKey).then((d) => {
      if (d) {
        this.s.details.set(d);
      } else {
        this.s.detailsFailed.set(true);
      }
    });
    // After a result the focus goes to the one next action, as soon as it is drawn.
    effect(() => this.back()?.nativeElement.focus());
    // The answer of a request in the air has to be shown: the sheet can't be dismissed meanwhile.
    effect(() => this.sheet.lock(this.s.busy()));
    // A sphere around the caster has nothing to place: its list is asked for at once.
    void this.ar.flow?.start();
    // Step 2 opens on its title; with nobody inside, on "Mudar o local", the safe answer.
    effect(() => {
      const step = this.ar.flow?.step();
      const asks = this.ar.asks();
      if (step !== 'list') {
        return;
      }
      untracked(() =>
        afterNextRender(
          () => (asks ? this.changePlace()?.nativeElement.focus() : this.listTitle()?.focus()),
          { injector: this.injector },
        ),
      );
    });
    // "Ninguém está marcado.": the focus goes to "Voltar à lista".
    effect(() => {
      if (this.s.listAsking()) {
        untracked(() =>
          afterNextRender(() => this.backToList()?.nativeElement.focus(), {
            injector: this.injector,
          }),
        );
      }
    });
    effect(() => {
      if (this.s.error()) {
        this.frame()?.scrollToTop();
      }
    });
    // A new stage (the result) opens at its top, not where the last one was
    // scrolled; so does the whole result once the last damage is typed (the
    // player scrolled down to the field).
    effect(() => {
      this.s.done();
      this.damage.owed();
      untracked(() => queueMicrotask(() => this.frame()?.scrollToTop()));
    });
  }

  /** "Confirmar local" (or the second tap): the list of who is inside. */
  protected confirmArea(): Promise<boolean> {
    return this.ar.flow ? this.ar.flow.confirm() : Promise.resolve(false);
  }

  /** "Mudar o local": back to the map, the point where it was. */
  protected changeArea(): void {
    this.ar.flow?.back();
  }

  protected pickSlot(row: SlotRow): void {
    this.ch.pickSlot(row);
  }

  protected async rollTyped(sum: number): Promise<void> {
    if (await this.damage.rollTyped(sum)) {
      this.s.typing.set(false);
    }
  }

  /** The "?" in the header: the spell's description over this sheet, so the slot and the targets chosen stay. */
  protected describe(): void {
    const { campaignId, spellKey, name } = this.data;
    openSpellDetails(
      this.dialog,
      this.bottomSheet,
      {
        namePt: name,
        load: async () => {
          const details = await this.catalog.details(campaignId, spellKey);
          if (!details) {
            throw new Error('spell details unavailable');
          }
          return spellDetailsFromGen(details);
        },
      },
      true,
    );
  }

  protected close(): void {
    if (this.s.busy()) {
      return;
    }
    this.sheet.close(this.s.done() && !this.damage.owed());
  }
}
