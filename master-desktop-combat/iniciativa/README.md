# Iniciativa do mestre em telas largas (fix/master-desktop-combat, item 1)

A tela antes de "Começar o combate", com 3 personagens de jogadores e 4 NPCs (3 "Espada voadora" e 1 "Armadura animada"), a 1024, 1280, 1440, 1728 e 1920 px, tema claro e escuro (`antes-<largura>-<tema>.png` e `depois-<largura>-<tema>.png`).

## O que limitava cada coluna

A página limita o conteúdo a 1280 px. De 1600 px em diante ela põe a coluna da conversa (380 a 420 px) ao lado do tabuleiro; o tabuleiro fica com cerca de 740 px. Dentro dele a tela de iniciativa era uma grade fixa (lista + lateral de 400 px) e cada linha da lista tinha cinco colunas fixas (32 + 28 + nome + 218 + 190 px, mais os espaços = 524 px sem o nome). Com a lista em 316 px o nome ficava com 0 px, a linha crescia para 578 px, a rolagem e os botões saíam da linha e cobriam a coluna do lado (o "Editar" em cima de "Não está escondido"). A barra era uma linha com título, texto e botões, e o texto de ajuda ficava com 98 px de largura.

| Largura | Lista antes | Nome antes | Linha antes | Texto da barra antes | Lista depois | Nome depois | Linha depois | Texto da barra depois |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 1024 | 928 | 366, 1 linha | 85 | 285 px, 3 linhas | 928 | 366, 1 linha | 85 | 886 px, 1 linha |
| 1280 | 760 | 198, 1 linha | 85 | 541 px, 2 linhas | 760 | 198, 1 linha | 85 | 541 px, 2 linhas |
| 1440 | 760 | 198, 1 linha | 85 | 541 px, 2 linhas | 760 | 198, 1 linha | 85 | 541 px, 2 linhas |
| 1728 | 316 | 0, 14 linhas | 578 | 98 px, 10 linhas | 740 | 614, 1 linha | 110 | 698 px, 2 linhas |
| 1920 | 316 | 0, 14 linhas | 578 | 98 px, 10 linhas | 740 | 614, 1 linha | 110 | 698 px, 2 linhas |

Em todas as larguras depois: sem rolagem lateral, nada se sobrepõe, um nome longo termina em reticências em uma linha (conferido também em 1100, 1200, 1279, 1599, 1600 e 1700 px com um nome de 60 caracteres).

## O que mudou

- A grade lista + lateral só tem duas colunas quando o tabuleiro tem 1100 px ou mais (consulta de contêiner, não de tela); abaixo disso a lateral vai para baixo da lista.
- A linha da lista responde à largura da lista: a partir de 760 px, uma linha; de 600 a 759 px, nome e detalhes em cima e a rolagem com os botões embaixo; abaixo de 600 px, como no celular.
- O nome fica em uma linha com reticências (o nome inteiro é a dica `title`).
- A barra, quando mais estreita que 1100 px (a partir de 1024 px de tela), põe o texto de ajuda na linha de baixo, com a largura toda.

## Método

Uma tela de teste descartável monta o `CombatView` real (cliente falso, os mesmos dados dos testes) dentro de uma casca com a mesma largura máxima, os mesmos espaços e a coluna da conversa de 1600 px da página da sessão; as medidas vêm do Playwright (largura da lista, do nome, linhas do nome, altura da linha, sobreposições entre texto, rolagem e botões, largura e linhas do texto da barra). As capturas são da build de desenvolvimento. A fonte de ícones não carrega offline, então os nomes dos ícones aparecem como texto nas imagens.
