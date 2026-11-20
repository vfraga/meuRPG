import { NgTemplateOutlet } from '@angular/common';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
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
  untracked,
} from '@angular/core';

import type { GearVm } from '../../core/characters/gear';
import { BonusChip } from '../bonus-chip/bonus-chip';
import { Fold } from '../fold/fold';
import { WeaponTags } from '../weapon-tags/weapon-tags';

/** What a row says of the piece's attunement: attuned, asks for it, or nothing. */
export type AttunementChip = '' | 'on' | 'asks';

/** What a tap on a row's button asks: this piece, equipped or stowed. */
export interface GearToggle {
  readonly key: string;
  readonly equipped: boolean;
}

/**
 * The gear of a sheet in two groups, "Equipado" and "Carregado" (SRD 5.1, "Equipment"): each piece with the button
 * that moves it to the other group. The names, the groups and the armor class a change would give all come from the
 * server (`DerivedSheet.gear`); the list only shows them and tells the host what was asked. The same list is on the
 * sheet and in the combat's "Sacar ou guardar" fold (`combat`): there the button says "Sacar" and what the change costs
 * ("Guardar · ação", "Sacar · grátis"), and a long "Carregados" folds.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-gear-list',
  imports: [BonusChip, Fold, MatButtonModule, MatIconModule, NgTemplateOutlet, WeaponTags],
  templateUrl: './gear-list.html',
  styleUrl: './gear-list.scss',
})
export class GearList {
  private static next = 0;
  private static readonly MANY = 4;
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);
  private askedKey: string | null = null;
  private wasBusy = false;
  protected readonly idBase = `gl${GearList.next++}`;

  readonly gear = input.required<readonly GearVm[]>();
  /** Whether the buttons exist: false for a reader who may not change the gear. */
  readonly canChange = input(true);
  /** Whether each row has its "⋮" for its marks or bonus (the owner while the sheet is editable, the master always). */
  readonly canMark = input(false);
  /** A tap on a row's "⋮": the page opens the sheet of its marks (a weapon) or its bonus (armor, shield). */
  readonly marksOpened = output<GearVm>();
  /** The piece being saved: its button waits, the others stay. */
  readonly busyKey = input<string | null>(null);
  /** The word a button carries beside its verb (the cost of the change in a combat: "grátis", "ação"); empty for none. */
  readonly cost = input<(piece: GearVm) => string>(() => '');
  /** The reason a piece cannot change now (a refusal the host knows beforehand): the button waits and the reason shows. */
  readonly blocked = input<(piece: GearVm) => string>(() => '');
  /** The combat's fold: "Sacar" for "Equipar", the cost in the button, "Carregados" folded when there are many. */
  readonly combat = input(false);
  /** The shield counts for nothing (a two-handed weapon is equipped): its row says "Não conta na CA". */
  readonly shieldIdle = input(false);
  /** The pieces whose drawing changes the armor class for the worse (a two-handed weapon with the shield in hand). */
  readonly warned = input<readonly string[]>([]);

  /** The attunement chip of a piece: 'on' when it is attuned, 'asks' when it asks and is not, '' for none. */
  readonly attunement = input<(piece: GearVm) => AttunementChip>(() => '');

  /** The armor class now (`DerivedSheet.armor_class`): the "Equipado" head shows it and a row says what a change would leave. */
  readonly armorClass = input<number | null>(null);
  /** Whether the "Equipado" head carries "CA N" on the right. */
  readonly showArmorClass = input(false);

  readonly toggled = output<GearToggle>();

  constructor() {
    // The call ended (the list is the new one): the focus follows the piece that moved to its button in the other group.
    effect(() => {
      const busy = this.busyKey() !== null;
      untracked(() => {
        if (this.wasBusy && !busy && this.askedKey) {
          const key = this.askedKey;
          this.askedKey = null;
          afterNextRender(() => this.focusRow(key), { injector: this.injector });
        }
        this.wasBusy = busy;
      });
    });
  }

  protected readonly equipped = computed(() => this.gear().filter((g) => g.equipped));
  protected readonly carried = computed(() => this.gear().filter((g) => !g.equipped));
  /** A long "Carregados" is one folded line, as the board draws it (01d). */
  protected readonly folded = computed(() => this.carried().length > GearList.MANY);
  protected readonly carriedTitle = computed(() =>
    this.folded()
      ? `Carregados (${this.carried().length})`
      : this.combat()
        ? 'Carregados'
        : 'Carregado',
  );

  protected readonly kindLabel = (g: GearVm): string =>
    g.kind === 'armor'
      ? 'Armadura'
      : g.kind === 'shield'
        ? 'Escudo'
        : g.twoHanded
          ? 'Arma de duas mãos'
          : 'Arma';

  /** What the button promises about the armor class, said only to a screen reader: the server's number when it differs. */
  protected armorLine(g: GearVm, current: number | null): string {
    if (this.combat() || g.armorClassIfToggled <= 0 || g.armorClassIfToggled === current) {
      return '';
    }
    return `CA ${g.armorClassIfToggled}`;
  }

  /** What a screen reader hears for the "⋮" of a row: the title of the sheet it opens. */
  protected moreLabel(g: GearVm): string {
    return `${g.kind === 'weapon' ? 'Marcas' : 'Bônus'} · ${g.namePt}`;
  }

  /** The marks a row shows: a bonus that counts takes the place of "Mágica" (the attack is magical because of it). */
  protected rowMarks(g: GearVm): GearVm['marks'] {
    return g.marks && g.bonus && g.bonusCounts !== false && !this.idle(g)
      ? { ...g.marks, magical: false }
      : g.marks;
  }

  /** The warning a two-handed weapon in the carried group gives: drawing it leaves the shield idle. */
  protected warnText(g: GearVm): string {
    const ac = this.armorClass();
    return ac === null
      ? ', Sacar desativa o escudo'
      : `, Sacar desativa o escudo: CA ${ac} → ${g.armorClassIfToggled}`;
  }

  protected icon(g: GearVm): string {
    return g.kind === 'armor' ? 'checkroom' : g.kind === 'shield' ? 'shield' : 'swords';
  }

  protected verb(g: GearVm): string {
    return g.equipped ? 'Guardar' : this.combat() ? 'Sacar' : 'Equipar';
  }

  protected idle(g: GearVm): boolean {
    return g.kind === 'shield' && g.equipped && this.shieldIdle();
  }

  /** Whether drawing this piece lowers the armor class (the host lists those by name). */
  protected warns(g: GearVm): boolean {
    return this.warned().includes(g.key);
  }

  protected reasonId(g: GearVm): string {
    return `${this.idBase}-why-${g.key}`;
  }

  /** What a screen reader hears: the words the button shows, in the order it shows them, then the piece (an accessible
   * name starts with the visible text). */
  protected buttonLabel(g: GearVm): string {
    const shown = [this.verb(g), this.cost()(g), this.armorLine(g, this.armorClass())]
      .filter(Boolean)
      .join(' ');
    const many = g.count > 1 ? ` (${g.count} iguais, a troca vale para todas)` : '';
    return `${shown} ${g.namePt}${many}`;
  }

  protected ask(g: GearVm): void {
    this.askedKey = g.key;
    this.toggled.emit({ key: g.key, equipped: !g.equipped });
  }

  /** After a change the focus goes to the moved row's button, which is now in the other group. */
  focusRow(key: string): void {
    this.host.nativeElement
      .querySelector<HTMLElement>(`[data-gear-key="${CSS.escape(key)}"]`)
      ?.focus();
  }
}
