# #1089: the note when a read takes a pick back

Taken on fix/pick-taken-back-1089 at b228fb8a, at 390, 768 and 1280 px, light and dark (2x).

- `1-ferramenta-tirada-*`: the end state of the tool-pick row of `scanPickLists`. After the refused save, the editor reads the choices again. The read takes the smiths' tools back, and the card reads "Falta escolher". The note "O mestre mudou as opções da mesa…" shows above the steps.
- `antes-1-ferramenta-tirada-*`: the same state on the base (ac8bf410), with no note.

The note is the same one the session's stream hint shows, in the same place. The gap between the note and the steps card is the note's existing layout, which this branch does not change.
