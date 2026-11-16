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
