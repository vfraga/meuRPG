import type { Combatant, Encounter } from '../../../../gen/meurpg/play/v1/combat_pb';
import { wallUnderDoor } from '../../../core/maps/door-paint';
import type { DoorSquare } from '../../../core/maps/layers';
import { resourceFlow } from '../../../core/resources/resource-actions';
import { openFamiliarEyes } from '../../../shared/familiar-eyes/familiar-eyes-sheet';
import { openSheet } from '../../../shared/sheet/sheet-host';
import { openSpellDetails } from '../../../shared/spell-details/open-spell-details';
import type { SpellDetailsData } from '../../../shared/spell-details/spell-details';
import { spellDetailsFromGen } from '../../../shared/spell-details/spell-details-map';
import { openWildShape } from '../../../shared/wild-shape/wild-shape-sheet';
import { type DoorSheetData, openDoorSheet } from '../door-sheet/door-sheet';
import { openTrapSearch } from '../traps/trap-search-sheet/trap-search-sheet';
import { ActionEconomy } from '../../../../gen/meurpg/rules/v1/rules_pb';
import {
  TableActionSheet,
  type TableActionSheetData,
} from './table-action-sheet/table-action-sheet';
import type { ActionOption } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { AdjustNpc, type AdjustNpcData } from './adjust-npc/adjust-npc';
import { ConditionsDialog, type ConditionsData } from './conditions-dialog/conditions-dialog';
import { FeatureSheet, type FeatureSheetData } from './feature-sheet/feature-sheet';
import {
  UseFeatureSheet,
  type UseFeatureSheetData,
  type UseFeatureSheetResult,
} from './use-feature-sheet/use-feature-sheet';
import { ReadySheet, type ReadySheetData } from './ready-sheet/ready-sheet';
import type { Observable } from 'rxjs';
import { needsSheet, resourceOf, usedNote } from '../../../core/combat/feature-use';
import { readySpeed } from '../../../core/combat/ready';
import { ActionKey } from '../../../core/connect/idempotency';
import { openBardicInspiration, openFlexibleCasting, openLayOnHands } from './resource-sheets';
import { SpendSheet, type SpendSheetData } from './theatre/spend-sheet';
import type { CombatSheetsContext } from './combat-sheets-context';

/** The dialogs of one purpose the combat screen opens: the door, a spell's description, the conditions, the adjust of an
 * NPC, the movement spent, the search for traps, a class feature, a class resource, the familiar's eyes and Wild Shape. */
export class ActionSheets {
  constructor(private readonly ctx: CombatSheetsContext) {}

  /** The master taps a door of the map: its sheet opens (open, close, lock, or reveal a secret door). The map reads itself again on the stream. */
  door(door: DoorSquare): void {
    const e = this.ctx.encounter();
    if (!e) {
      return;
    }
    const layers = this.ctx.layers();
    const data: DoorSheetData = {
      campaignId: this.ctx.campaignId(),
      mapId: e.mapId,
      door,
      wallSquares: wallUnderDoor(
        layers.walls,
        layers.columns,
        layers.rows,
        this.ctx.mapState().map()?.squareFactor ?? 1,
        door,
      ),
    };
    openDoorSheet(this.ctx.dialog, this.ctx.bottomSheet, data).subscribe();
  }

  /** The "?" of a spell: its description (the SRD's, read once through the catalog), a sheet on a phone. */
  describeSpell(key: string, name: string): void {
    openSpellDetails(this.ctx.dialog, this.ctx.bottomSheet, this.spellDetailsData(key, name));
  }

  /** What the details sheet needs: the name now, and the text when it loads. */
  private spellDetailsData(key: string, name: string): SpellDetailsData {
    const campaignId = this.ctx.campaignId();
    return {
      namePt: name,
      load: async () => {
        const details = await this.ctx.catalog.details(campaignId, key);
        if (!details) {
          throw new Error('spell details unavailable');
        }
        return spellDetailsFromGen(details);
      },
    };
  }

  /** "Condições…" in a combatant's menu (the master). */
  conditions(combatantId: string): void {
    const e = this.ctx.encounter();
    const combatant = e?.combatants.find((c) => c.id === combatantId);
    if (!e || !combatant) {
      return;
    }
    const data: ConditionsData = {
      campaignId: this.ctx.campaignId(),
      encounterId: e.id,
      combatant,
      state: this.ctx.state(),
    };
    openSheet<ConditionsDialog, ConditionsData, boolean>(
      this.ctx.dialog,
      this.ctx.bottomSheet,
      ConditionsDialog,
      {
        data,
        ariaLabel: `Condições de ${combatant.label}`,
        labelledBy: 'sheet-t',
        width: '600px',
      },
    ).subscribe();
  }

  /** "Dano/Cura" on an NPC (`AdjustCombatantHitPoints`). */
  adjustNpc(combatantId: string): void {
    const e = this.ctx.encounter();
    const combatant = e?.combatants.find((c) => c.id === combatantId);
    if (!e || !combatant) {
      return;
    }
    openSheet<AdjustNpc, AdjustNpcData, boolean>(this.ctx.dialog, this.ctx.bottomSheet, AdjustNpc, {
      data: {
        campaignId: this.ctx.campaignId(),
        encounterId: e.id,
        combatant,
        state: this.ctx.state(),
      },
      ariaLabel: `Dano ou cura em ${combatant.label}`,
      labelledBy: 'adjust-npc-title',
      width: '440px',
    }).subscribe();
  }

  /** "Gastar movimento" (a combat without a map): the sheet that asks how far. For the player's character, or for one of their creatures. */
  spend(id?: string): void {
    const e = this.ctx.encounter();
    const who = id ?? this.ctx.own()?.id;
    if (!e || !who) {
      return;
    }
    const data: SpendSheetData = {
      campaignId: this.ctx.campaignId(),
      encounterId: e.id,
      combatantId: who,
      state: this.ctx.state(),
    };
    openSheet<SpendSheet, SpendSheetData, boolean>(
      this.ctx.dialog,
      this.ctx.bottomSheet,
      SpendSheet,
      {
        data,
        ariaLabel: 'Gastar movimento',
        labelledBy: 'sheet-t',
      },
    ).subscribe(() => this.ctx.spendClosed());
  }

  /** The search for traps (E9-08): it spends the action through `SearchForTraps`. */
  searchTraps(): void {
    openTrapSearch(this.ctx.dialog, this.ctx.bottomSheet, {
      campaignId: this.ctx.campaignId(),
      skills: this.ctx.trapSkills(),
      diceMode: this.ctx.diceMode(),
      preference: this.ctx.preference(),
      state: this.ctx.mapState(),
      inCombat: true,
    }).subscribe();
  }

  /** "Retomar o Fôlego": the feature that rolls on its own sheet. */
  secondWind(own: Combatant, e: Encounter, name: string, economy: ActionEconomy): void {
    const data: FeatureSheetData = {
      campaignId: this.ctx.campaignId(),
      encounterId: e.id,
      combatantId: own.id,
      actionKey: 'feature:second-wind',
      name,
      cost: economy === ActionEconomy.BONUS_ACTION ? 'Ação bônus' : 'Ação',
      diceMode: this.ctx.diceMode(),
      preference: this.ctx.preference(),
      state: this.ctx.state(),
    };
    openSheet<FeatureSheet, FeatureSheetData, boolean>(
      this.ctx.dialog,
      this.ctx.bottomSheet,
      FeatureSheet,
      { data, ariaLabel: name, labelledBy: 'sheet-t' },
    ).subscribe();
  }

  private readonly useKey = new ActionKey();

  /**
   * "Usar" on a feature taken with `UseFeature` (#743; `TakeAction` refuses its key). One that asks who or which, and any use of
   * the master acting as the character, opens the sheet; the player's own use with nothing to ask is called at once. Either way
   * the note of the turn says what it gave and what is left of the resource.
   */
  async takeFeature(own: Combatant, e: Encounter, option: ActionOption): Promise<void> {
    const resource = resourceOf(option, this.ctx.ownVitals()?.resources);
    const name = option.action?.namePt ?? '';
    if (needsSheet(option, this.ctx.isMaster())) {
      this.useFeature(own, e, option).subscribe((result) => {
        if (result) {
          this.ctx.note(usedNote(name, result, option, resource));
        }
      });
      return;
    }
    const actionKey = option.action?.key ?? '';
    let result = { healed: 0, tempHp: 0 };
    const made = await this.ctx.calls.run(async (current) => {
      const res = await this.ctx.api.features.useFeature(
        {
          campaignId: this.ctx.campaignId(),
          encounterId: current.id,
          combatantId: own.id,
          actionKey,
        },
        this.useKey.keyFor({ combatant: own.id, key: actionKey }),
      );
      this.useKey.renew();
      result = res;
      return res.encounter;
    });
    if (made) {
      this.ctx.note(usedNote(name, result, option, resource));
    }
  }

  /** "Usar" on a feature taken with `UseFeature` that asks who or which, and always for the master acting as the character
   * (the confirmation). Answers what the use healed and gave when it worked. */
  useFeature(
    own: Combatant,
    e: Encounter,
    option: ActionOption,
  ): Observable<UseFeatureSheetResult | undefined> {
    const key = option.action?.key ?? '';
    const data: UseFeatureSheetData = {
      campaignId: this.ctx.campaignId(),
      encounterId: e.id,
      combatantId: own.id,
      userLabel: own.label,
      option,
      targets: this.ctx.options()?.featureTargets.find((t) => t.actionKey === key)?.targets ?? [],
      resource: resourceOf(option, this.ctx.ownVitals()?.resources),
      master: this.ctx.isMaster(),
      state: this.ctx.state(),
    };
    const name = option.action?.namePt ?? '';
    return openSheet<UseFeatureSheet, UseFeatureSheetData, UseFeatureSheetResult>(
      this.ctx.dialog,
      this.ctx.bottomSheet,
      UseFeatureSheet,
      { data, ariaLabel: name, labelledBy: 'sheet-t', width: '480px' },
    );
  }

  /**
   * "Preparar" (#582): the sheet of the Ready action. It takes what `GetTurnOptions` listed (the attacks, the spells and the standard
   * actions), the creatures of the fight to name in the trigger and where the readier stands (no map, no speed, a concentration).
   * The call, its refusals and the combat it answers are the sheet's own.
   */
  openReady(): void {
    const own = this.ctx.own();
    const e = this.ctx.encounter();
    const options = this.ctx.options();
    if (!own || !e || !options?.options) {
      return;
    }
    const data: ReadySheetData = {
      campaignId: this.ctx.campaignId(),
      encounterId: e.id,
      combatantId: own.id,
      ownerLabel: own.label,
      options: options.options,
      summaries: new Map(),
      spellTargets: options.spellTargets,
      theatre: this.ctx.theatre(),
      ...readySpeed(own.speedFt, options),
      concentrating: own.concentrationSpell ? own.concentrationSpellNamePt : '',
      combatants: e.combatants,
      state: this.ctx.state(),
    };
    openSheet<ReadySheet, ReadySheetData, boolean>(
      this.ctx.dialog,
      this.ctx.bottomSheet,
      ReadySheet,
      { data, ariaLabel: 'Preparar uma ação', labelledBy: 'sheet-t', width: '880px' },
    ).subscribe();
  }

  private readonly giveUpKey = new ActionKey();

  /** "Desistir da ação preparada" (#582): `CancelReadied` on the readier's own turn; the action spent to ready it does not come back. */
  giveUp(readiedId: string): Promise<boolean> {
    return this.ctx.calls.run(async (e) => {
      const enc = await this.ctx.api.ready.cancelReadied(
        this.ctx.campaignId(),
        e.id,
        readiedId,
        this.giveUpKey.keyFor({ readied: readiedId }),
      );
      this.giveUpKey.renew();
      return enc;
    });
  }

  private readonly fireKey = new ActionKey();

  /** "Disparar" (#582): the master says the trigger of a readied action happened; `FireReadied` opens the readier's window. */
  fire(readiedId: string): Promise<boolean> {
    return this.ctx.calls.run(async (e) => {
      const enc = await this.ctx.api.ready.fireReadied(
        this.ctx.campaignId(),
        e.id,
        readiedId,
        this.fireKey.keyFor({ readied: readiedId }),
      );
      this.fireKey.renew();
      return enc;
    });
  }

  /** A table feature's action that asks how many to spend, for whom, or the dice of its heal (MR-025). */
  tableAction(own: Combatant, e: Encounter, option: ActionOption): void {
    const data: TableActionSheetData = {
      campaignId: this.ctx.campaignId(),
      encounterId: e.id,
      combatantId: own.id,
      option,
      combatants: e.combatants,
      diceMode: this.ctx.diceMode(),
      preference: this.ctx.preference(),
      state: this.ctx.state(),
    };
    const name = option.action?.namePt ?? '';
    openSheet<TableActionSheet, TableActionSheetData, boolean>(
      this.ctx.dialog,
      this.ctx.bottomSheet,
      TableActionSheet,
      { data, ariaLabel: name, labelledBy: 'sheet-t' },
    ).subscribe();
  }

  /** The dialog of a class resource (Cura pelas Mãos, Conjuração Flexível, Inspiração de Bardo): `TakeAction` refuses
   * these actions, `ResourceService` runs them. */
  resource(
    flow: NonNullable<ReturnType<typeof resourceFlow>>,
    key: string,
    actorId: string,
    encounterId: string,
  ): void {
    const base = {
      campaignId: this.ctx.campaignId(),
      encounterId,
      actorId,
      vitals: this.ctx.ownVitals,
      state: this.ctx.state(),
    };
    const targets =
      this.ctx.options()?.resourceTargets.find((t) => t.actionKey === key)?.targets ?? [];
    switch (flow) {
      case 'lay-on-hands':
        openLayOnHands(this.ctx.dialog, this.ctx.bottomSheet, { ...base, targets }).subscribe();
        break;
      case 'bardic-give':
        openBardicInspiration(this.ctx.dialog, this.ctx.bottomSheet, {
          ...base,
          targets,
        }).subscribe();
        break;
      default:
        openFlexibleCasting(this.ctx.dialog, this.ctx.bottomSheet, {
          ...base,
          direction: flow === 'flexible-create' ? 'create' : 'convert',
        }).subscribe();
    }
  }

  /** "Ver pelos olhos do Nanquim": the question first; the stream brings the band, the action spent and the blind line. */
  familiarEyes(familiarName: string): void {
    const mine = this.ctx.own();
    if (!mine) {
      return;
    }
    openFamiliarEyes(this.ctx.dialog, this.ctx.bottomSheet, {
      campaignId: this.ctx.campaignId(),
      characterId: mine.characterId,
      characterName: mine.label,
      familiarName,
      inCombat: true,
    }).subscribe();
  }

  /** "Transformar": the list of beasts the level allows. */
  wildShape(): void {
    const own = this.ctx.own();
    if (!own) {
      return;
    }
    const resource = this.ctx.ownVitals()?.resources?.find((r) => r.key === 'wild_shape');
    openWildShape(this.ctx.dialog, this.ctx.bottomSheet, {
      campaignId: this.ctx.campaignId(),
      characterId: own.characterId,
      inCombat: true,
      uses: resource ? { left: resource.total - resource.used, total: resource.total } : null,
    }).subscribe((result) => {
      if (result?.encounter) {
        this.ctx.state().apply(result.encounter);
      }
    });
  }
}
