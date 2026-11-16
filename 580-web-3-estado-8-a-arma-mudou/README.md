# 580 web-3: estado 8 do board 580-16, "A arma mudou." na ficha de Tor

Branch `feat/table-prereq-580-web-3`. Mesa com o "Machado de Lenhador" (simples, 1d6, uma mão); Toren (Guerreiro 4, proficiente em armas simples e escudos) o carrega com um escudo equipado. O mestre edita o machado e põe "Duas mãos"; o dono abre a ficha. Todos os shots com deviceScaleFactor 2, claro e escuro (sufixo `-claro` / `-escuro`). Os números são os reais da ficha de Toren (CA 13 → 11), não os do board (Tor, CA 18 → 16).

| Arquivo (prefixo 580-web-3-estado-8-) | Estado |
| --- | --- |
| ficha-dono-{390,768,1280}-{claro,escuro}.png | A ficha do dono (viewport inteira) com o aviso "A arma mudou." sob o cabeçalho; a CA mostrada na ficha é a de depois (11) |
| aviso-dono-{390,768,1280}-{claro,escuro}.png | Só o aviso, recortado: negrito "A arma mudou.", a frase do mestre com a data, "Os números da ficha já usam as regras novas. O que não combina mais:", o marcador com a frase da arma e o botão "Ver o que mudou" |
| ficha-mestre-{390,1280}-{claro,escuro}.png | A mesma ficha lida pelo mestre: o mesmo aviso |
| ver-o-que-mudou-390-{claro,escuro}.png | O que "Ver o que mudou" abre no celular: folha de baixo "O que mudou: Machado de Lenhador", a frase em aviso, "Quem ajusta: você, na ficha. O aviso some sozinho quando os números voltam a combinar." e "Fechar" |
| ver-o-que-mudou-1280-{claro,escuro}.png | O mesmo, como janela de 560 px a partir do tablet |

## Diferenças em relação ao board (não fechadas)
- O botão "Ver o que mudou" tem o estilo do botão contornado do app (texto vinho, borda cinza), não o marrom do aviso do board: item à parte.
- Em 1280 a frase corre a largura toda do aviso (o board mostra uma coluna estreita): item à parte.
- A ficha não tem o jogador "Tor, CA 18 → 16": o texto é o do servidor com o nome e a CA reais da ficha.

## O que foi corrigido
- O botão ficava a 16 px da última linha da lista; o board tem 8 px (o espaço da coluna, sem margem a mais). Retirada a margem do botão.
