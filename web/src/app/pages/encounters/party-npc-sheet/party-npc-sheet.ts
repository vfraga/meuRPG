import { Component, computed, effect, inject, signal, untracked } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { RouterLink } from '@angular/router';

import { CharacterKind } from '../../../../gen/meurpg/characters/v1/characters_pb';
import type { EncounterEvaluation } from '../../../../gen/meurpg/play/v1/encounters_pb';
import type { MonsterGroupSpec } from '../../../core/combat/combat-client';
import { combatantInitial, npcKindLabel } from '../../../core/combat/combat-view';
import { type DraftNpc } from '../../../core/encounters/encounter-draft';
import { EncountersClient, type PartyNpcSpec } from '../../../core/encounters/encounters-client';
import { GUIDE_LABEL, encounterErrorMessage } from '../../../core/encounters/encounter-text';
import { formatInt, tight } from '../../../core/format/text';
import type { RosterEntry } from '../../../core/maps/roster-client';
import { CountStepper } from '../../../shared/count-stepper/count-stepper';
import { SheetFrame } from '../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../shared/sheet/sheet-host';

/** What the builder hands the sheet. */
export interface PartyNpcData {
  readonly campaignId: string;
  /** The campaign's NPCs that are not in the party yet. */
  readonly npcs: readonly RosterEntry[];
  /** The encounter and party as they are, to show what the budget becomes. */
  readonly entries: readonly MonsterGroupSpec[];
  readonly party: readonly PartyNpcSpec[];
  /** The last measure, for "Antes". */
  readonly before: EncounterEvaluation | null;
}

/** "Só um nome": no NPC of the campaign, only a name and a level. */
const NAME_ONLY = '';
const LABEL_MAX = 80;

/**
 * "Pôr um NPC no grupo" (MR-043, question 86, E10-09 state 2): an NPC joins the party for this encounter with the level the
 * master gives it (the NPC's sheet needs none). It counts in the budget and in the lowest level that caps a creature's
 * rating. The sheet shows the budget it makes ("O orçamento passa a: Baixa 1.400, Moderada 2.100, Alta 3.000 XP") by asking
 * the server for it (`EvaluateEncounter` with the NPC in), the browser adds nothing. A name alone is allowed too, for a
 * companion who has no sheet.
 */
@Component({
  selector: 'app-party-npc-sheet',
  imports: [CountStepper, MatButtonModule, MatIconModule, RouterLink, SheetFrame],
  templateUrl: './party-npc-sheet.html',
  styleUrl: './party-npc-sheet.scss',
})
export class PartyNpcSheet {
  private readonly api = inject(EncountersClient);
  private readonly sheet = injectSheet<PartyNpcData, DraftNpc>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly guide = GUIDE_LABEL;
  protected readonly nameOnly = NAME_ONLY;
  protected readonly labelMax = LABEL_MAX;
  protected readonly initial = combatantInitial;
  protected readonly kind = npcKindLabel;
  protected readonly CharacterKind = CharacterKind;

  protected readonly pick = signal<string>(this.data.npcs[0]?.id ?? NAME_ONLY);
  protected readonly typed = signal('');
  protected readonly level = signal(3);
  protected readonly preview = signal<EncounterEvaluation | null>(null);
  protected readonly error = signal('');
  protected readonly nameError = signal('');

  private seq = 0;

  /** Who would join: the chosen NPC (with its name) or the typed name. */
  protected readonly candidate = computed<DraftNpc | null>(() => {
    const id = this.pick();
    if (id === NAME_ONLY) {
      const name = this.typed().trim();
      return name === '' ? null : { characterId: '', name, level: this.level(), label: name };
    }
    const npc = this.data.npcs.find((n) => n.id === id);
    return npc ? { characterId: npc.id, name: '', level: this.level(), label: npc.name } : null;
  });
  private readonly measureKey = computed(
    () => `${this.pick()}|${this.level()}|${this.candidate() !== null}`,
  );
  protected readonly before = computed(() => this.line(this.data.before));
  protected readonly after = computed(() => this.line(this.preview()));

  constructor() {
    // Each change of the NPC or the level asks the server what the budget becomes; an answer that is no longer the
    // newest question is dropped.
    // The name is not part of the budget: only the NPC, the level and whether there is someone to add ask the server again.
    effect(() => {
      this.measureKey();
      untracked(() => void this.measure(this.candidate()));
    });
  }

  private line(
    ev: EncounterEvaluation | null,
  ): { low: string; moderate: string; high: string } | null {
    return ev?.budget
      ? {
          low: formatInt(ev.budget.low),
          moderate: formatInt(ev.budget.moderate),
          high: formatInt(ev.budget.high),
        }
      : null;
  }

  private async measure(candidate: DraftNpc | null): Promise<void> {
    const mine = ++this.seq;
    if (!candidate) {
      this.preview.set(null);
      return;
    }
    try {
      const ev = await this.api.evaluate(this.data.campaignId, this.data.entries, [
        ...this.data.party,
        { characterId: candidate.characterId, name: candidate.name, level: candidate.level },
      ]);
      if (mine === this.seq) {
        this.preview.set(ev);
        this.error.set('');
      }
    } catch (err) {
      if (mine === this.seq) {
        this.preview.set(null);
        this.error.set(encounterErrorMessage(err, 'evaluate'));
      }
    }
  }

  protected beforeText(): string {
    const b = this.before();
    return b ? tight(`Antes: ${b.low} · ${b.moderate} · ${b.high}.`) : '';
  }

  protected add(): void {
    const candidate = this.candidate();
    if (!candidate) {
      this.nameError.set('Dê um nome ao NPC do grupo.');
      return;
    }
    this.sheet.close(candidate);
  }

  protected close(): void {
    this.sheet.close();
  }
}
