# 580 web-3: opções de característica com pré-requisito (board 580-03), rodada 2

Branch `feat/table-prereq-580-web-3`. Subclasse "Guarda do Corvo" e classe "Guardião do Vale", característica "Truques de lâmina" (escolha 3), opções com pré-requisito. deviceScaleFactor 2, claro e escuro (`-claro` / `-escuro`), larguras 768 e 1280 (o editor não existe em 390). `580-03-board-1280-claro.png` é o board, para comparar. Os shots das rodadas anteriores que mostravam texto ou estrutura já trocados (ponto antes de "Precisa de ajuste", select nativo, "1 de 10." na dica, etiqueta colada ao nome) foram apagados.

| Arquivo | Estado |
| --- | --- |
| 580-03-r2-opcoes-recolhidas-* | 5 linhas recolhidas, "Opção N · nome" e resumo, setas, moldura cinza |
| 580-03-r2-adicionar-opcao-menu-* | "Outras opções": o botão "Adicionar opção" aberto no painel de menu do sistema; cada escolha com a origem ("Opção N", "SRD" ou "Da mesa") à direita, a lista rola dentro do painel |
| 580-03-r2-chip-opcao-n-* / -recorte-* | A irmã escolhida: chip de cantos retos "Asa Partida" + "Opção 2", o botão "Adicionar opção" na mesma linha e "1 de 10" no fim da linha |
| 580-03-r2-recusa-chip-marcado-* / -recorte-* | Depois do "Salvar subclasse" recusado: a linha 1 aberta e a linha 2 fechada com borda e fundo de perigo e "Precisa de ajuste" na linha do nome; o chip da recusa com borda de perigo e ícone; a frase sob os chips; o painel "Como os jogadores veem" diz "Ainda não salvo: a característica tem um ajuste pendente." |
| 580-03-r2-linha-recusada-cabeca-{aberta,fechada}-* | A cabeça das duas linhas recusadas, sem ponto, a marca sem quebra |
| 580-03-r2-preview-nao-salvo-* | O painel "Como os jogadores veem" com a recusa de pé |
| 580-web-3-r2-editor-de-classe-preview-* / -recorte-* | Editor de classe com a característica aberta e o painel "Como os jogadores veem" logo depois da seção das características |
| 580-web-3-r2-editor-de-classe-faixa-fixa-* | Rolado pelo meio da seção: em 768 só a faixa de seções fica presa sob a barra; o cartão "Para os jogadores" não cobre mais nada |
| 580-web-3-r2-editor-de-classe-subclasse-* | A seção "Subclasse": o rótulo "A subclasse se escolhe no nível" inteiro |
| 580-web-3-r2-editor-de-classe-fim-da-pagina-* | Fim da página: a barra "Salvar classe" assenta na borda do conteúdo |
| 580-03-r2-celular-so-le-390-* | Leitura em 390: o aviso com a primeira frase em negrito e "Escolha 3 de 5 opções." na característica |

## Diferenças em relação ao board (mantidas)
- Em 768 o painel "Como os jogadores veem" fica abaixo do editor (a coluna lateral só aparece a partir de 1100 px). No editor de classe ele fica logo depois da seção das características, em qualquer largura, porque a coluna lateral da classe tem 236 px e é a lista de seções.
- A origem "SRD" só aparece no menu; o chip de uma opção do SRD não leva etiqueta. Nomes iguais de conjuntos diferentes do SRD (ex.: três "Arquearia") aparecem repetidos no menu.
- O servidor marca as duas opções do par (cada uma pede a outra); o board marca uma.
- O cartão "Para os jogadores" mantém o título e a frase sob o interruptor; o board tem só o interruptor e "Ligado".
- A barra de salvar fica na borda do conteúdo; entre ela e o rodapé do app sobra o espaço do rodapé.
