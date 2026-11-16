# 580 web-3: opções de característica com pré-requisito (board 580-03)

Branch `feat/table-prereq-580-web-3`. Subclasse "Guarda do Corvo", característica "Truques de lâmina" (escolha 3), 5 opções. Todos os shots com deviceScaleFactor 2, claro e escuro.

Sufixos: `-claro` / `-escuro`. Larguras 768 e 1280 (editor). O editor de subclasse não existe em 390 (só no notebook): em 390 há só a página de leitura.

| Arquivo (prefixo 580-03-) | Estado |
| --- | --- |
| opcoes-recolhidas-{768,1280}-{claro,escuro}.png | 5 linhas recolhidas com resumo ("Opção N · nome" e "efeito · pede X"), setas no primeiro (Subir desligado), no meio e no último (Descer desligado), moldura cinza na característica |
| opcao-aberta-{768,1280}-{claro,escuro}.png | Uma linha aberta (Opção 2), moldura de destaque só nela, corpo recuado 36px |
| outras-opcoes-irmas-{768,1280}-{claro,escuro}.png | "Outras opções" aberta: irmãs salvas com selo "Opção N", nunca a própria opção; a lista é o select nativo renderizado como listbox, por isso no escuro o texto das opções aparece apagado (artefato do headless, não do app) |
| dica-opcao-nova-{768,1280}-{claro,escuro}.png | A dica: "Uma opção nova só pode ser pedida depois de salvar a característica." mantendo "Nunca a própria opção." |
| como-os-jogadores-veem-{768,1280}-{claro,escuro}.png | Painel "Como os jogadores veem" com a linha apagada "Pede …" por opção (mesma frase da leitura) |
| recusa-no-campo-{768,1280}-{claro,escuro}.png | Recusa do servidor no campo "Outras opções" ("Este valor não é aceito. Confira o campo."), a linha abre sozinha e mostra "Precisa de ajuste" |
| celular-so-le-390-{claro,escuro}.png | Página de leitura em 390 com a linha "Pede …" apagada nas opções |

## Diferenças em relação ao board (não fechadas)
- No 768 o painel "Como os jogadores veem" fica abaixo do editor (a coluna lateral só aparece a partir de 1100px).
- A recusa do servidor usa o texto genérico "Este valor não é aceito. Confira o campo."; o texto do board é mais específico e depende de mensagem do servidor.
- A lista "Outras opções" usa o select nativo, não um menu desenhado.

## Rodada 1 de revisão
Editor de subclasse "Guarda do Vale" e de classe "Guardião do Vale", 768 e 1280, claro e escuro (sufixo `-{768,1280}-{claro,escuro}`), deviceScaleFactor 2. Os recortes de elemento podem mostrar a barra "Salvar" fixa por cima.

| Arquivo | Estado |
| --- | --- |
| 580-03-board-1280-claro.png | O board 580-03 (a exportação do board, para comparar) |
| 580-web-3-r1-editor-de-classe-preview-aberto-* / -recorte-* | Editor de classe com uma característica aberta: o painel "Como os jogadores veem" (sem ele não há marco "Como os jogadores veem") |
| 580-03-r1-chip-opcao-n-* / -recorte-* | "Outras opções" com a irmã escolhida: o chip "Asa Partida" com a etiqueta "Opção 2" a 8 px do nome |
| 580-03-r1-recusa-irma-com-motivo-* / -recorte-* | Depois do "Salvar subclasse" recusado: a opção 1 aberta com a frase "Esta opção e a que ela pede esperam uma pela outra: nenhuma das duas poderia ser escolhida. Tire um dos pedidos." no campo, e as linhas 1 e 2 com "Precisa de ajuste" |
| 580-03-r1-linha-recusada-cabeca-* | A cabeça da linha 1 recusada: setas e "⋮" com o tamanho das demais, o ponto depois do resumo e a etiqueta fora dele |
| 580-03-r1-linha-recusada-setas-ponto-* | A linha 2 recusada inteira (em 768 o ponto fecha a primeira linha e a etiqueta desce) |
| 580-03-r1-recusa-segue-a-opcao-* | Depois de "Descer a opção Asa Partida": a marca vai com a opção |

## Diferenças em relação ao board (não fechadas)
- O servidor marca as duas opções do par (cada uma pede a outra); o board marca uma.
- Em 390 não há editor de subclasse nem de classe.
