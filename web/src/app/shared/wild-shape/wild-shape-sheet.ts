import { Component, computed, effect, inject, signal } from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';

import type { ListWildShapeFormsResponse } from '../../../gen/meurpg/characters/v1/characters_pb';
import type { CharacterVitals } from '../../../gen/meurpg/play/v1/play_pb';
import type { Encounter } from '../../../gen/meurpg/play/v1/combat_pb';
import type { Creature, CreatureSummary } from '../../../gen/meurpg/rules/v1/rules_pb';
import { combatErrorMessage } from '../../core/combat/combat-errors';
import { ActionKey } from '../../core/connect/idempotency';
import { readBlocks } from '../../core/creatures/read-blocks';
import { CreaturesClient } from '../../core/creatures/creatures-client';
import { beastAttacks, beastLine } from '../../core/creatures/creature-format';
import { tight } from '../../core/format/text';
import { SheetFrame } from '../../pages/live-session/combat/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../../pages/live-session/combat/sheet-host';
import { type ChoiceRow, CreatureChoiceList } from '../creatures/creature-choice-list';

/** What the sheet needs: whose Wild Shape, whether it is a combat (it costs the action there) and the uses the character has. */
export interface WildShapeSheetData {
  readonly campaignId: string;
  readonly characterId: string;
  /** In a combat the action is spent too. */
  readonly inCombat: boolean;
  /** "Restam 2 de 2 usos": what the character has now, for "restará 1"; `null` when the sheet does not know. */
  readonly uses: { readonly left: number; readonly total: number } | null;
}

/** What the sheet answers when the character turned into the beast. */
export interface WildShapeResult {
  readonly beastNamePt: string;
  readonly vitals: CharacterVitals | undefined;
  /** The combat as the answer says it, when the character is in one: the page applies it. */
  readonly encounter: Encounter | undefined;
}

/** Opens "Forma Selvagem": a bottom sheet on a phone, a dialog from a tablet up. */
export function openWildShape(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: WildShapeSheetData,
) {
  return openSheet<WildShapeSheet, WildShapeSheetData, WildShapeResult>(
    dialog,
    bottomSheet,
    WildShapeSheet,
    {
      data,
      ariaLabel: 'Forma Selvagem',
      labelledBy: 'wild-t',
      width: '560px',
    },
  );
}

/**
 * "Forma Selvagem" (MR-037, E9-11 state 2): the beasts the druid's level allows, as the server lists them
 * (`ListWildShapeForms`: it knows the limit of challenge rating, the fly and the swim speed), in Portuguese
 * order, with a search by the Portuguese or the English name and one to choose. The chosen row opens its numbers
 * (armor class, hit points, speed, the attack, all of them the book's); the footer says what it costs before it
 * does it: the action and one use (restará 1) in a combat, one use out of it. The app does not count hours: the
 * master ends the form. The key of the call is made anew whenever the choice changes, so a repeated tap after a
 * lost answer never turns the druid twice; a refusal stays here, in words, with the sheet open.
 */
@Component({
  selector: 'app-wild-shape-sheet',
  imports: [CreatureChoiceList, MatButtonModule, MatIconModule, SheetFrame],
  templateUrl: './wild-shape-sheet.html',
  styleUrl: './wild-shape-sheet.scss',
})
export class WildShapeSheet {
  private readonly client = inject(CreaturesClient);
  private readonly sheet = injectSheet<WildShapeSheetData, WildShapeResult>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;

  protected readonly forms = signal<ListWildShapeFormsResponse | null>(null);
  protected readonly blocks = signal<ReadonlyMap<string, Creature>>(new Map());
  protected readonly chosen = signal('');
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');
  private readonly key = new ActionKey();

  /** "Feras de ND até 1/2, sem voo." */
  protected readonly rule = computed(() => {
    const f = this.forms();
    if (!f || f.maxCr === '') {
      return '';
    }
    // What the level forbids, as the server says it: flying, swimming, both or neither.
    const no =
      f.noFly && f.noSwim
        ? ', sem voo nem natação'
        : f.noFly
          ? ', sem voo'
          : f.noSwim
            ? ', sem natação'
            : '';
    return tight(`Feras de ND até ${f.maxCr}${no}.`);
  });
  protected readonly count = computed(() => {
    const n = this.forms()?.forms.length ?? 0;
    return n === 1 ? '1 fera que o seu nível permite' : `${n} feras que o seu nível permite`;
  });
  protected readonly rows = computed<readonly ChoiceRow[]>(() =>
    (this.forms()?.forms ?? []).map((s) => {
      const block = this.blocks().get(s.key);
      return {
        key: s.key,
        title: s.namePt,
        alias: s.name,
        subtitle: beastLine(s, block),
        attacks: block ? beastAttacks(block) : undefined,
      };
    }),
  );
  protected readonly beast = computed<CreatureSummary | null>(
    () => this.forms()?.forms.find((s) => s.key === this.chosen()) ?? null,
  );

  /** What it costs, before it is done. */
  protected readonly cost = computed(() => {
    const u = this.data.uses;
    const left = u ? ` (restará ${Math.max(0, u.left - 1)})` : '';
    return this.data.inCombat
      ? tight(
          `Gasta a ação e 1 uso de Forma Selvagem${left}. O app não conta o tempo: o mestre encerra a forma.`,
        )
      : tight(
          `Gasta 1 uso de Forma Selvagem${left}. O app não conta o tempo: o mestre encerra a forma.`,
        );
  });
  protected readonly buttonLabel = computed(() => {
    const b = this.beast();
    return b ? `Virar ${b.namePt}` : 'Virar fera';
  });

  constructor() {
    void this.load();
  }

  private async load(): Promise<void> {
    try {
      const forms = await this.client.wildShapeForms(this.data.campaignId, this.data.characterId);
      this.forms.set(forms);
      void this.readBlocks(forms.forms.map((f) => f.key));
    } catch (err) {
      this.forms.set({
        forms: [],
        maxCr: '',
        noFly: false,
        noSwim: false,
      } as unknown as ListWildShapeFormsResponse);
      this.error.set(combatErrorMessage(err, 'abrir a lista de feras'));
    }
  }

  /** The numbers of each beast, six at a time: a row shows the book's summary first and its numbers as they arrive. */
  private readBlocks(keys: readonly string[]): Promise<void> {
    return readBlocks(this.client, this.data.campaignId, keys, (key, block) =>
      this.blocks.update((m) => new Map(m).set(key, block)),
    );
  }

  protected pick(key: string): void {
    this.chosen.set(key);
    this.error.set('');
  }

  protected async confirm(): Promise<void> {
    const beast = this.beast();
    if (!beast || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.client.assumeWildShape(
        this.data.campaignId,
        this.data.characterId,
        beast.key,
        this.key.keyFor(beast.key),
      );
      this.sheet.close({ beastNamePt: beast.namePt, vitals: res.vitals, encounter: res.encounter });
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'virar a fera'));
    } finally {
      this.busy.set(false);
    }
  }

  protected close(): void {
    this.sheet.close();
  }
}
