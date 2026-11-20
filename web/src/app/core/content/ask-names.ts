import type { SelectOption } from '../../shared/form-fields/select-field';
import type { CatalogVm } from './catalog';
import type { FeatureDraft } from './feature-draft';
import type { KeyName } from './key-name';

const TABLE_WORDS = / \(da mesa\)$/;

/**
 * The names of the keys an option of a feature asks for, as the master reads them while editing: a sibling by the name it has now,
 * then what the catalog knows (a class, a spell, a table entry), then the options of the lists ("(da mesa)" taken off). A key nobody
 * knows comes back as it is, and the readers of a prerequisite make a phrase of it.
 */
export function askKeyNamer(
  options: readonly FeatureDraft[],
  catalog: CatalogVm | null,
  requirable: readonly SelectOption[],
): KeyName {
  return (key) => {
    const sibling = options.find((o) => o.key === key && o.name.trim() !== '');
    if (sibling) {
      return sibling.name.trim();
    }
    const named = catalog?.nameOf(key) ?? key;
    if (named !== key) {
      return named;
    }
    return requirable.find((o) => o.value === key)?.label.replace(TABLE_WORDS, '') ?? key;
  };
}
