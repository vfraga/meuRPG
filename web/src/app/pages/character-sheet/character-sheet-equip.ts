import { signal } from '@angular/core';
import type { Observable } from 'rxjs';

import { describeCharacterError } from '../../core/characters/character-errors';
import { combatErrorMessage, encounterBlocked } from '../../core/combat/combat-errors';
import type { GearVm, MarksVm } from '../../core/characters/gear';
import type { GearToggle } from '../../shared/gear-list/gear-list';
import type { WeaponMarksData } from './weapon-marks/weapon-marks-sheet';
import { marksOf } from './character-sheet-marks';
import type { CharacterSheetSource, CharacterSheetVm, FullSheetVm } from './character-sheet.types';

/** What the equipping needs of the page that owns it. */
export interface SheetEquipHost {
  readonly source: CharacterSheetSource;
  readonly campaignId: () => string;
  /** The character on the page; empty when the page is gone. */
  readonly characterId: () => string;
  /** Reads the sheet again without the loading state; `true` when the new sheet is on screen. */
  readonly reload: () => Promise<boolean>;
  /** The sheet on screen, when it is a full one. */
  readonly sheet: () => FullSheetVm | null;
  /** The character as the page holds it: its stored full sheet and revision are what a change of the marks is saved over. */
  readonly vm: () => CharacterSheetVm | null;
  /** Shows the answer of a write as the newest word on its character; `false` when the page left it. */
  readonly apply: (characterId: string, vm: CharacterSheetVm) => boolean;
  /** Opens the sheet of a weapon's marks; it answers `true` when they were saved. */
  readonly openMarksSheet: (data: WeaponMarksData) => Observable<boolean | undefined>;
}

/**
 * "Equipar" and "Guardar" on the sheet: the server changes the piece, the page reads the sheet again for the names, the
 * groups and the armor class it derives, and the panel says what moved. A refusal of a running combat is said in words.
 */
export class SheetEquip {
  /** The piece whose change is on its way to the server. */
  readonly busyKey = signal<string | null>(null);
  readonly notice = signal('');
  readonly error = signal('');
  /** The sheet could not be read after a change: "Tentar de novo" asks again. */
  readonly readFailed = signal(false);

  /** What the last save of marks said ("Marcas salvas."), for the live region under the list. */
  readonly marksNotice = signal('');

  constructor(private readonly host: SheetEquipHost) {}

  /** The weapon's "⋮": its marks in a sheet; "Salvar marcas" saves them over the whole stored sheet, at the revision the page holds. */
  openMarks(piece: GearVm): void {
    const vm = this.host.vm();
    if (!vm?.fullSheet) {
      return;
    }
    this.marksNotice.set('');
    const current =
      vm.sheet.kind === 'full'
        ? vm.sheet.weaponMarks.find((m) => m.weaponKey === piece.key)
        : undefined;
    this.host
      .openMarksSheet({
        weaponName: piece.namePt,
        characterName: vm.name,
        isMaster: vm.isMaster,
        marks: current ?? piece.marks ?? { silvered: false, adamantine: false, magical: false },
        save: (marks) => this.saveMarks(piece.key, marks),
      })
      .subscribe((saved) => {
        if (saved) {
          this.marksNotice.set('Marcas salvas.');
        }
      });
  }

  /** Saves one weapon's marks over the whole stored sheet; the answer is the refusal in words, or '' when it is saved. */
  private async saveMarks(weaponKey: string, marks: MarksVm): Promise<string> {
    const vm = this.host.vm();
    const campaignId = this.host.campaignId();
    if (!vm?.fullSheet || !campaignId) {
      return 'A ficha não está aberta.';
    }
    const others = vm.fullSheet.weaponMarks.filter((m) => m.weaponKey !== weaponKey);
    const mine =
      marks.silvered || marks.adamantine || marks.magical ? [{ weaponKey, ...marks }] : [];
    try {
      const saved = await this.host.source.setWeaponMarks(
        campaignId,
        vm.id,
        vm.revision,
        vm.name,
        vm.fullSheet,
        [...others.map((m) => ({ weaponKey: m.weaponKey, ...marksOf(m) })), ...mine],
      );
      this.host.apply(vm.id, saved);
      return '';
    } catch (err) {
      return describeCharacterError(err);
    }
  }

  /** The tab is shown again: the sheet is read again, so a change made from another tab or by the master is not left
   * on screen. Returns what stops the listening. */
  refreshWhenShown(doc: Document): () => void {
    const onShown = () => {
      if (doc.visibilityState === 'visible' && this.busyKey() === null && this.host.characterId()) {
        void this.host.reload();
      }
    };
    doc.addEventListener('visibilitychange', onShown);
    return () => doc.removeEventListener('visibilitychange', onShown);
  }

  /** "Tentar de novo": reads the sheet again after a change whose list could not be read. */
  async retry(): Promise<void> {
    if (await this.host.reload()) {
      this.readFailed.set(false);
      this.error.set('');
    }
  }

  async toggle(change: GearToggle): Promise<void> {
    const campaignId = this.host.campaignId();
    const characterId = this.host.characterId();
    if (!campaignId || !characterId || this.busyKey() !== null) {
      return;
    }
    this.busyKey.set(change.key);
    this.error.set('');
    this.notice.set('');
    this.marksNotice.set('');
    this.readFailed.set(false);
    try {
      await this.host.source.setEquipped(campaignId, characterId, change.key, change.equipped);
      const fresh = await this.host.reload();
      if (characterId !== this.host.characterId()) {
        return;
      }
      if (!fresh) {
        this.readFailed.set(true);
        this.error.set('A troca foi feita, mas não deu para ler a ficha de novo.');
        return;
      }
      const full = this.host.sheet();
      const name = full?.gear.find((g) => g.key === change.key)?.namePt ?? '';
      this.notice.set(
        `${change.equipped ? 'Equipado' : 'Guardado'}: ${name}.${full ? ` Classe de armadura ${full.armorClass}.` : ''}`,
      );
    } catch (err) {
      this.notice.set('');
      this.error.set(
        encounterBlocked(err)
          ? combatErrorMessage(err, 'mudar o equipamento')
          : describeCharacterError(err),
      );
    } finally {
      this.busyKey.set(null);
    }
  }
}
