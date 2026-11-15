# #580 web 2, "Pré-requisito perdido" (580-05) and the weapons list (580-16 state 4)

Branch `feat/table-prereq-580-web-2`. Light is "claro", dark is "escuro". All shots at device scale 2, viewports 390x844, 768x1024 and 1280x800. The sheet is Toren (Guerreiro 4); "dono" is the player who owns the sheet, "mestre" is the campaign master. The master saved the sheet with the feat (a guided pick would refuse it), so the table's content is older than the sheet's flags and no "mudou" notice shows, except in the "editado" shots.

Each 580-05 state has two shots at each width and theme: `-pendencias` (the "Pendências de regra" banner on the sheet, top of the page) and `-linha` (a window of one viewport on "Características e traços", with the dashed "Não vale agora" tag on the row of the feat or option). The board's short line on the row was dropped: the row shows only the tag.

| File | Board | State |
|---|---|---|
| `580-05-talento-visivel-dono-{390,768,1280}-{claro,escuro}-{pendencias,linha}` | 580-05 | Lost feat, the missing condition is the SRD's Guerreiro: "Postura Firme pede Guerreiro 7, e Toren está no 4: o talento só vale de novo quando Toren cumprir." |
| `580-05-opcao-visivel-dono-*` | 580-05 | Lost option (Asa Partida, an inline option of the feat "Truques de lâmina"), same condition: "Asa Partida pede Guerreiro 7, e Toren está no 4: a opção só vale de novo quando Toren cumprir." |
| `580-05-talento-oculto-dono-*` | 580-05, 580-09 card 5 | Lost feat whose missing condition is a class the master switched off (RN-23), the owner's view: "Postura Firme pede uma condição que a mesa não mostra: o talento só vale de novo quando Toren cumprir." |
| `580-05-opcao-oculta-dono-*` | 580-05, 580-09 card 5 | The same for the lost option: "Asa Partida pede uma condição que a mesa não mostra: a opção só vale de novo quando Toren cumprir." |
| `580-05-talento-oculto-mestre-*` | 580-05, 580-09 card 5 | The master's view of the hidden feat condition, named: "Postura Firme pede Ordem Esquecida 3, e Toren não tem nível de Ordem Esquecida: o talento só vale de novo quando Toren cumprir." |
| `580-05-opcao-oculta-mestre-*` | 580-05, 580-09 card 5 | The master's view of the hidden option condition, named. |
| `580-05-editado-dono-{390,768,1280}-{claro,escuro}` | 580-05 | The master edited the feat after the sheet was saved (added Guerreiro 7): the line shows inside the changed-content notice for the owner, titled "O talento mudou.", and is not repeated in "Pendências de regra". Reshot after the fix (332ba43). |
| `580-16-4-armas-da-mesa-lista-{1280-claro,1280-escuro,390-claro}` | 580-16 state 4 | Character editor, Equipamento step, the "Armas" list opened by the player with "Machado de Lenhador" (table weapon) checked. See note 3: the app lists "Do livro" first and "Da mesa" after it, not the board's "Da mesa" above "Do SRD". |

## What is not here

- 580-16 state 8 ("A arma mudou." after the master puts "Duas mãos" on the Machado de Lenhador with a shield on the sheet): the app does not do it. A weapon edit raises no sheet issue, so no changed-content notice is made for a weapon (see `backend/internal/characters/weaponedit_580_test.go`, which says so). Nothing was built.

## Notes (differences from the boards)

1. Fixed on the branch (332ba43): a feat's change is titled "O talento mudou." (an option's "A opção mudou."). The board shows "A subclasse mudou." for a subclass option; the shots use a feat.
2. Fixed on the branch (332ba43): a sentence the changed-content notice tells is no longer repeated in "Pendências de regra".
3. The weapons list in the character editor is `Do livro` (SRD) then `Da mesa`, and the mat-select opens with the list scrolled to the checked option, so "Do livro" is above the fold. Board 580-16 state 4 draws "Da mesa" on top, then "Do SRD", with a table-weapon tag per row.
4. Fixed on the branch: at 768 to 1199 the master's "História" card was drawn over "Características e traços" (a bug already on main). The `-mestre-768-` shots were retaken with the two side by side.
