# #574 (`feat/table-choices-574`): the notice of an entry that loads with a part left out

Light is "claro", dark is "escuro". All shots are at device scale 2, taken from the a11y row "o aviso de uma entrada que carrega com uma parte de fora" (`e2e/tests/a11y.spec.ts`) with `E2E_SHOTS_DIR` set (the shot lines were added for the run only and are not in the commit). The row rewrites the real `ListTableEntries` response: it sets `loadedWithIssues` with one issue on the background "Cais do Farol" and puts `skill:athletics` in the second place of the tool list that the issue names. The Go tests cover what the server sends; the shots show what the page draws.

| File | State |
|---|---|
| `574-aviso-da-entrada-390-claro.png`, `574-aviso-da-entrada-390-escuro.png` | Phone: the master reads the background. The "Para criar ou editar" note, then the warning notice ("Esta entrada tem um aviso." and the sentence naming "Atletismo"), then the read rows. Whole page down to "Característica". |
| `574-aviso-da-entrada-768-claro.png`, `574-aviso-da-entrada-768-escuro.png` | Tablet: the master's editor with the warning above the form, no "Fora da campanha" tag. Cropped to the header, the notice and the top of "Dados do antecedente". |
| `574-aviso-da-entrada-1280-claro.png`, `574-aviso-da-entrada-1280-escuro.png` | Laptop: the same state, cropped the same way. |
