import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { TableEntry } from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { SUMMON_LOCK_PT, SUMMON_TURN_LINE_PT } from '../../../core/content/spell-summon';
import { MAX_SUMMON_LIVE, type SummonDraft } from '../../../core/content/summon-draft';
import { FieldNote } from '../../../shared/form-fields/field-note';
import { RadioCards } from '../../../shared/form-fields/radio-cards';
import { TextField } from '../../../shared/form-fields/text-field';
import { REPLACES_CARDS, TURN_CARDS } from '../../../shared/summon-block/summon-cards';
import { SummonCreatures } from '../../../shared/summon-block/summon-creatures';

/** The spell's own path of the summon: a refusal of the server lands under `table_spell.summon.*`. */
const AT = 'table_spell.summon';

/**
 * The "Invocar" mechanic of the spell editor (#578, board C3): the creatures (chips and a search), how many come (a formula that may use
 * the circle of the slot), how many more for each circle above the spell's own, the most at a time, and the two questions a feature asks
 * too (the turn, and what a new casting does to the old creatures). The cost, the time, the duration and the concentration are the
 * spell's own, so this block has no control for them. The lock line says what the engine asks of the spell; `lockIssues` is what it
 * refuses about the summon as a whole.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-spell-summon',
  imports: [FieldNote, MatIconModule, RadioCards, SummonCreatures, TextField],
  template: `
    <app-summon-creatures
      groupLabel="Criaturas que a magia invoca"
      [keys]="summon().creatureKeys"
      [campaignId]="campaignId()"
      [entries]="entries()"
      [path]="path('creature_keys')"
      [issues]="issues('creature_keys')"
      (keysChange)="patch({ creatureKeys: $event })"
    />
    <div class="row">
      <app-text-field label="Quantas vêm" hint="Fórmula; pode usar o círculo do espaço." [value]="summon().count" [path]="path('count')" [issues]="issues('count')" (valueChange)="patch({ count: $event })" />
      <app-text-field label="A mais por círculo acima" hint="De 0 a 10." inputmode="numeric" [value]="summon().perSlot" [path]="path('count_per_slot_level')" [issues]="issues('count_per_slot_level')" (valueChange)="patch({ perSlot: $event })" />
      <app-text-field label="No máximo ao mesmo tempo" [hint]="'0: sem limite. Até ' + maxLive + '.'" inputmode="numeric" [value]="summon().maxLive" [path]="path('max_live')" [issues]="issues('max_live')" (valueChange)="patch({ maxLive: $event })" />
    </div>
    <div class="row row--2">
      <app-radio-cards label="Turno" [options]="turnCards" [value]="'' + (summon().turnMode || 1)" [path]="path('turn_mode')" [issues]="issues('turn_mode')" (valueChange)="patch({ turnMode: +$event })" />
      <app-radio-cards label="Ao usar de novo" [options]="repliesCards" [value]="'' + (summon().replaces || 1)" [path]="path('replaces')" [issues]="issues('replaces')" (valueChange)="patch({ replaces: +$event })" />
    </div>
    <p class="line">{{ turnLine }}</p>
    <p class="line lock" role="note" [attr.data-field]="at" tabindex="-1" [attr.aria-describedby]="lockIssues().length > 0 ? 'spell-summon-note' : null"><mat-icon aria-hidden="true">lock</mat-icon>{{ lockLine }}</p>
    <app-field-note id="spell-summon-note" [issues]="lockIssues()" />
  `,
  styleUrl: './spell-summon.scss',
})
export class SpellSummon {
  readonly summon = input.required<SummonDraft>();
  readonly campaignId = input.required<string>();
  readonly entries = input<readonly TableEntry[]>([]);
  /** The messages of one input by its path: the server's refusals and the form's own. */
  readonly issuesOf = input<(path: string) => readonly string[]>(() => []);
  /** What the engine refuses about the spell as a whole, at `table_spell.summon`. */
  readonly lockIssues = input<readonly string[]>([]);
  readonly summonChange = output<SummonDraft>();

  protected readonly at = AT;
  protected readonly maxLive = MAX_SUMMON_LIVE;
  protected readonly turnCards = TURN_CARDS;
  protected readonly repliesCards = REPLACES_CARDS;
  protected readonly turnLine = SUMMON_TURN_LINE_PT;
  protected readonly lockLine = SUMMON_LOCK_PT;

  protected path(field: string): string {
    return `${AT}.${field}`;
  }

  protected issues(field: string): readonly string[] {
    return this.issuesOf()(this.path(field));
  }

  protected patch(p: Partial<SummonDraft>): void {
    this.summonChange.emit({ ...this.summon(), ...p });
  }
}
