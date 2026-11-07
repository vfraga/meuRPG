import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  input,
  output,
} from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { CreatureSummary } from '../../../gen/meurpg/rules/v1/rules_pb';
import { TableContentKind, type TableEntry } from '../../../gen/meurpg/rules/v1/table_content_pb';
import { MAX_SUMMON_CREATURES } from '../../core/content/summon-draft';
import { creatureNameOf, rememberCreatureName } from '../../core/content/summon-names';
import { CreaturesClient } from '../../core/creatures/creatures-client';
import { CreaturePick } from '../creature-pick/creature-pick';
import { FieldNote } from '../form-fields/field-note';
import { TableMark } from '../table-mark/table-mark';

let nextId = 0;

/**
 * The creatures a feature's summon offers (board C2): a search in the bestiary (the table's creatures and the SRD's) and the
 * ones picked as chips, 1 to 8, none twice. A table creature carries "Da mesa" (its name is its entry's); an SRD one "· SRD"
 * (its name is read from the bestiary once and kept for the preview). The group carries the path of the refusal.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-summon-creatures',
  imports: [CreaturePick, FieldNote, MatIconModule, TableMark],
  template: `
    <div class="head">
      <app-creature-pick class="head__pick" [campaignId]="campaignId()" label="Criaturas" placeholder="Procurar criatura" [disabled]="full()" (picked)="add($event)" />
      <p class="head__hint" [id]="id + '-hint'">De 1 a {{ max }}. Quem usa escolhe uma delas para cada criatura que vem.</p>
    </div>
    <ul class="chips" role="group" aria-label="Criaturas que a característica invoca" [attr.data-field]="path() || null" tabindex="-1" [attr.aria-describedby]="issues().length > 0 ? id + '-note' : id + '-hint'" [attr.aria-invalid]="issues().length > 0 ? 'true' : null">
      @for (c of chips(); track c.key) {
        <li class="chip">
          <span class="chip__name">{{ c.name }}</span>
          @if (c.table) {
            <app-table-mark [fromTable]="true" />
          } @else {
            <span class="chip__srd">· SRD</span>
          }
          <button type="button" class="chip__x" [attr.aria-label]="'Tirar ' + c.name" (click)="remove(c.key)">
            <mat-icon aria-hidden="true">close</mat-icon>
          </button>
        </li>
      }
    </ul>
    <app-field-note [id]="id + '-note'" [issues]="issues()" />
  `,
  styleUrl: './summon-creatures.scss',
})
export class SummonCreatures {
  private readonly client = inject(CreaturesClient);
  protected readonly id = `sc-${nextId++}`;
  protected readonly max = MAX_SUMMON_CREATURES;

  readonly keys = input.required<readonly string[]>();
  readonly campaignId = input.required<string>();
  /** The table's entries: a table creature is named by its own. */
  readonly entries = input<readonly TableEntry[]>([]);
  readonly path = input('');
  readonly issues = input<readonly string[]>([]);
  readonly keysChange = output<string[]>();

  protected readonly full = computed(() => this.keys().length >= MAX_SUMMON_CREATURES);
  private readonly tableNames = computed(
    () =>
      new Map(
        this.entries()
          .filter((e) => e.kind === TableContentKind.CREATURE)
          .map((e) => [e.key, e.namePt]),
      ),
  );
  protected readonly chips = computed(() =>
    this.keys().map((key) => ({
      key,
      table: key.endsWith('@mesa'),
      name: creatureNameOf(key, (k) => this.tableNames().get(k) ?? k),
    })),
  );

  constructor() {
    // An SRD creature loaded with the entry has no name yet: the bestiary says it (kept for the preview), and a failed read leaves the slug.
    effect(() => {
      const campaignId = this.campaignId();
      for (const key of this.keys().filter((k) => k.startsWith('monster:'))) {
        this.client.statBlock(campaignId, key).then(
          (c) => rememberCreatureName(key, c.summary?.namePt ?? ''),
          () => undefined,
        );
      }
    });
  }

  protected add(c: CreatureSummary): void {
    rememberCreatureName(c.key, c.namePt);
    if (!this.keys().includes(c.key) && !this.full()) {
      this.keysChange.emit([...this.keys(), c.key]);
    }
  }

  protected remove(key: string): void {
    this.keysChange.emit(this.keys().filter((k) => k !== key));
  }
}
