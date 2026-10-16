import { computed, effect, signal, untracked } from '@angular/core';

import { CharacterKind, CreatureSource } from '../../../../gen/meurpg/characters/v1/characters_pb';
import { DeathSaveVisibility } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type {
  Combatant,
  Encounter,
  GetTurnOptionsResponse,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import type { Creature, SpellDetails } from '../../../../gen/meurpg/rules/v1/rules_pb';
import type { TableRulesClient } from '../../../core/campaigns/table-rules';
import { isDown, npcKindLabel } from '../../../core/combat/combat-view';
import { questions } from '../../../core/combat/hidden-reveal';
import type { SpellCatalog } from '../../../core/combat/spell-catalog';
import type { CreaturesClient } from '../../../core/creatures/creatures-client';
import type { RosterClient } from '../../../core/maps/roster-client';
import type { CombatantInfo } from './combat-info';

/** What `CombatReads` reads from the screen it serves. */
export interface CombatReadsContext {
  readonly campaignId: () => string;
  readonly isMaster: () => boolean;
  readonly encounter: () => Encounter | null;
  readonly own: () => Combatant | null;
  /** The player's `GetTurnOptions`, for the spells the list shows. */
  readonly options: () => GetTurnOptionsResponse | null;
  readonly tableRules: Pick<TableRulesClient, 'get'>;
  readonly catalog: Pick<SpellCatalog, 'details'>;
  readonly roster: Pick<RosterClient, 'list'>;
  readonly creatures: Pick<CreaturesClient, 'list' | 'statBlock'>;
}

/**
 * What the combat screen reads from the services around the combat, each once or when its subject changes, and keeps for
 * the screen: the table's rule on the death saves, the SRD's details of the spells in the list, the names of the spells
 * a question about hidden creatures is about, the master's roster, the druid's beast and the player's familiar. Build it
 * in an injection context (a field initializer): every read is an effect over the context.
 */
export class CombatReads {
  /** The table hides the death saves from the other players (RN-24); `null` while the rule is not read (then every screen shows them as before). */
  private readonly deathRule = signal<DeathSaveVisibility | null>(null);
  readonly deathsHidden = computed(() => this.deathRule() === DeathSaveVisibility.OWNER_AND_MASTER);
  /** The SRD's details of each spell in the list (read once, cached by the catalog): the line under a spell's name. */
  readonly spellDetails = signal<ReadonlyMap<string, SpellDetails>>(new Map());
  /** The Portuguese name of each spell a question is about, read through the catalog. */
  readonly revealSpellNames = signal<ReadonlyMap<string, string>>(new Map());
  readonly rosterInfo = signal<ReadonlyMap<string, CombatantInfo>>(new Map());
  /** The book's stat block of the beast the druid is, for its traits (read once per beast). */
  readonly beastBlock = signal<Creature | null>(null);
  /** The player's familiar ("Nanquim"), for the action "Ver pelos olhos do Nanquim": read once for the character, and again when the combat changes who they are. */
  readonly familiar = signal<string | null>(null);

  private ruleKey = '';
  private familiarRequest = 0;
  /** Whose familiar to ask for: the character's id, so a re-read of the combat changes nothing. */
  private readonly familiarOwner = computed(() =>
    this.ctx.isMaster() || !!this.ctx.own()?.familiarSightCreatureId
      ? ''
      : (this.ctx.own()?.characterId ?? ''),
  );

  constructor(private readonly ctx: CombatReadsContext) {
    this.watchDeathRule();
    this.watchFamiliar();
    this.watchSpellDetails();
    // The master's lists need the roster (the kind of each NPC, the class line, the player's name).
    effect(() => {
      if (this.ctx.isMaster() && this.ctx.campaignId()) {
        void this.loadRoster(this.ctx.campaignId());
      }
    });
    this.watchBeast();
    this.watchRevealSpells();
  }

  /** The table's rule on who sees the death saves (RN-24): read when a combat shows and again when someone falls, so a rule changed meanwhile is caught. */
  private watchDeathRule(): void {
    effect(() => {
      const e = this.ctx.encounter();
      const campaignId = this.ctx.campaignId();
      const key = e ? `${e.id}:${e.combatants.some(isDown)}` : '';
      untracked(() => {
        if (key !== '' && key !== this.ruleKey) {
          this.ruleKey = key;
          void this.readDeathRule(campaignId);
        }
      });
    });
  }

  private async readDeathRule(campaignId: string): Promise<void> {
    try {
      this.deathRule.set((await this.ctx.tableRules.get(campaignId)).saved.deathSaves);
    } catch {
      // Without the rule the screens show the marks as they always did.
    }
  }

  /** The familiar's name, for the action "Ver pelos olhos do Nanquim" (the owner's list: RN-20). */
  private watchFamiliar(): void {
    effect(() => {
      const characterId = this.familiarOwner();
      const campaignId = this.ctx.campaignId();
      const request = ++this.familiarRequest;
      if (!characterId) {
        untracked(() => this.familiar.set(null));
        return;
      }
      untracked(
        () =>
          void this.ctx.creatures.list(campaignId, characterId).then(
            (list) => {
              if (request === this.familiarRequest) {
                this.familiar.set(
                  list.find((c) => c.source === CreatureSource.FAMILIAR)?.name ?? null,
                );
              }
            },
            () => {
              if (request === this.familiarRequest) {
                this.familiar.set(null);
              }
            },
          ),
      );
    });
  }

  private watchSpellDetails(): void {
    effect(() => {
      const spells = this.ctx.options()?.options?.spells ?? [];
      const campaignId = this.ctx.campaignId();
      for (const s of spells) {
        const key = s.spell?.key ?? '';
        if (!key || untracked(() => this.spellDetails().has(key))) {
          continue;
        }
        void this.ctx.catalog.details(campaignId, key).then((d) => {
          if (d) {
            this.spellDetails.update((m) => new Map(m).set(key, d));
          }
        });
      }
    });
  }

  private async loadRoster(campaignId: string): Promise<void> {
    try {
      const entries = await this.ctx.roster.list(campaignId);
      this.rosterInfo.set(
        new Map(
          entries.map((e) => [
            e.id,
            {
              classSummary: e.classSummary,
              playerName: e.playerName,
              kindLabel: e.kind === CharacterKind.PLAYER ? '' : npcKindLabel(e.kind),
              raceName: e.raceName,
            },
          ]),
        ),
      );
    } catch {
      // Best effort: the lists fall back to what the combat itself says.
    }
  }

  /** The beast's book (its armor class and traits), read once for each beast the druid becomes. */
  private watchBeast(): void {
    effect(() => {
      const key = this.ctx.own()?.wildShapeBeastKey ?? '';
      const campaignId = this.ctx.campaignId();
      untracked(() => {
        if (!key) {
          this.beastBlock.set(null);
          return;
        }
        void this.ctx.creatures.statBlock(campaignId, key).then(
          (block) => this.beastBlock.set(block),
          () => this.beastBlock.set(null),
        );
      });
    });
  }

  /** The questions about hidden creatures name their spell: read once per spell through the catalog. */
  private watchRevealSpells(): void {
    effect(() => {
      const e = this.ctx.encounter();
      const campaignId = this.ctx.campaignId();
      const keys = e && this.ctx.isMaster() ? questions(e).map((q) => q.spellKey) : [];
      untracked(() => {
        for (const key of new Set(keys)) {
          if (this.revealSpellNames().has(key)) {
            continue;
          }
          void this.ctx.catalog.details(campaignId, key).then((d) => {
            const name = d?.spell?.namePt || d?.spell?.name;
            if (name) {
              this.revealSpellNames.update((m) => new Map(m).set(key, name));
            }
          });
        }
      });
    });
  }
}
