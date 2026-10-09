// Guia "Indústria" do Assistente — o que era previsto (PRD / documentos da JC)
// versus o que está no ar. Fonte ÚNICA: a aba do Assistente e o PDF levado ao
// Heverton leem este mesmo arquivo (scripts/gerar_guia_industria_pdf.mjs).
// Ao mudar uma regra do painel, atualize aqui.

export type Escopo = 'ambos' | 'web' | 'mobile'
export type Status = 'ok' | 'parcial' | 'diferente' | 'pendente'

export interface Topico {
  id: string
  grupo: string
  titulo: string
  escopo: Escopo
  status: Status
  previsto: string
  ficou: string
  nota?: string
}

export interface Pergunta {
  n: number
  titulo: string
  contexto: string
  hoje: string
  decidir: string
  escopo: Escopo
  // Preenchido quando a JC já decidiu; some o campo "Precisamos decidir" do destaque.
  decisao?: string
}

export const STATUS_ROTULO: Record<Status, string> = {
  ok: 'Entregue como previsto',
  parcial: 'Entregue em parte',
  diferente: 'Entregue diferente do previsto',
  pendente: 'Não entregue / aguardando decisão',
}

export const ESCOPO_ROTULO: Record<Escopo, string> = {
  ambos: 'Web e Mobile',
  web: 'Só Web',
  mobile: 'Só Mobile',
}

export const INTRO = {
  titulo: 'Painel de Objetivos por Indústria — previsto × entregue',
  resumo:
    'O painel mostra, em segundos, quem está batendo o objetivo de cada indústria (hoje Unilever Food e Unilever HC) e quem não está. ' +
    'Existem dois programas: PONDERADA (as lojas são agrupadas em Redes) e NUMÉRICA (cada cliente/CNPJ conta sozinho, classificado em Num. A, B ou C). ' +
    'Cada programa tem duas métricas: COBERTURA (quanto o cliente comprou em R$) e SORTIMENTO (quantos itens diferentes comprou).',
  comoLer:
    'Cada tópico abaixo tem duas colunas: "Era para ser" (o que foi combinado no PRD e nos documentos da JC) e "O que ficou" (o que está funcionando hoje). ' +
    'A etiqueta de cor diz se bate, se bate em parte, se ficou diferente ou se ainda depende de uma decisão. ' +
    'Ao final há a lista numerada de perguntas para a reunião.',
}

export const TOPICOS: Topico[] = [
  // ── Estrutura e navegação ───────────────────────────────────────────────
  {
    id: 'hierarquia', grupo: 'Estrutura e navegação', titulo: 'Hierarquia de visões', escopo: 'ambos', status: 'ok',
    previsto: 'Navegar de cima para baixo: GGV → CRV (supervisor) → RCA → Rede → Cliente → Produto, com o total de cada nível e o farol verde/vermelho.',
    ficou:
      'Web: Ponderada abre GGV, GGV×CRV, GGV×CRV×RCA, Por Rede e Por Cliente. Numérica não tem Rede, então usa os mesmos três níveis de equipe e abre direto no Cliente. ' +
      'Mobile: o GGV e o Supervisor veem um resumo da equipe (por CRV e por RCA) que abre nível a nível; o RCA vê direto as suas Redes/Clientes. No mobile, GGV e Supervisor escolhem o Tipo de relatório: "Resumo Equipe" ou "Lista Clientes" (as duas convivem).',
  },
  {
    id: 'acesso', grupo: 'Estrutura e navegação', titulo: 'Quem acessa e como', escopo: 'ambos', status: 'ok',
    previsto: 'Web com login para Gerentes, GGVs e Supervisores; URL pública para o ION VENDAS (RCA em campo) que continue funcionando como antes.',
    ficou: 'As duas formas funcionam. As URLs /m/CNPJ/SUP/cod e /m/CNPJ/RCA/cod do aplicativo de campo foram mantidas; cada pessoa só enxerga a sua parte da hierarquia.',
  },
  {
    id: 'selecao', grupo: 'Estrutura e navegação', titulo: 'Escolha da Indústria e da Métrica', escopo: 'ambos', status: 'diferente',
    previsto: 'Escolher a Indústria e depois a métrica (Cobertura, Sortimento, ou a visão combinada das duas), em Ponderada ou Numérica.',
    ficou:
      'Em 08/10 o painel passou a oferecer só a visão COMBINADA de cada programa: "Ponderada (Cobertura + Sortimento)" e "Numérica (Cobertura + Sortimento)". ' +
      'As métricas isoladas só aparecem se a indústria tiver apenas uma delas cadastrada.',
    nota: 'Atende ao pedido do Heverton de 08/10. Confirmar na reunião se não faz falta ver Cobertura sozinha.',
  },

  // ── Ponderada ───────────────────────────────────────────────────────────
  {
    id: 'cob-pond', grupo: 'Programa Ponderada (por Rede)', titulo: 'Cobertura Ponderada', escopo: 'ambos', status: 'ok',
    previsto: 'Para cada Rede, média de compra por loja no período. A Rede "atinge" se a média por loja chega a R$ 1.500 (valor líquido da indústria).',
    ficou: 'Igual ao previsto. O limiar (R$ 1.500) fica no vínculo da indústria e pode ser trocado sem mexer em código. Devoluções e cancelamentos são abatidos da venda.',
  },
  {
    id: 'sort-pond', grupo: 'Programa Ponderada (por Rede)', titulo: 'Sortimento Ponderada', escopo: 'ambos', status: 'ok',
    previsto: 'Para cada Rede, média de itens (EAN) diferentes comprados por loja. Item só conta com no mínimo 3 unidades quando a embalagem é unidade. Objetivo por indústria (ex.: 19 itens na Food).',
    ficou: 'Igual ao previsto. A média é dividida por TODAS as lojas da Rede (inclusive as que não compraram). Quando o cadastro repete o mesmo EAN, ele é contado uma vez só.',
  },
  {
    id: 'faixas', grupo: 'Programa Ponderada (por Rede)', titulo: 'Farol e faixas', escopo: 'ambos', status: 'ok',
    previsto: 'Farol binário (verde atingiu / vermelho não atingiu), com faixas de objetivo definidas por vigência (mês).',
    ficou: 'Igual ao previsto. Cada vigência guarda as suas faixas; quando a vigência fecha, o resultado é congelado e não muda mais com recargas.',
  },
  {
    id: 'rede-dono', grupo: 'Programa Ponderada (por Rede)', titulo: 'Rede que atravessa mais de uma equipe', escopo: 'ambos', status: 'parcial',
    previsto: 'Cada Rede pertence a um único RCA/supervisor/GGV para não contar duas vezes.',
    ficou: 'Adotamos a regra "dono = menor CNPJ da Rede". Hoje isso é irrelevante: as 134 Redes cadastradas estão inteiras dentro de uma única equipe.',
    nota: 'Regra provisória (pergunta 11).',
  },

  // ── Numérica ────────────────────────────────────────────────────────────
  {
    id: 'cob-num', grupo: 'Programa Numérica (por Cliente)', titulo: 'Cobertura Numérica', escopo: 'ambos', status: 'ok',
    previsto: 'Cada cliente (CNPJ) é avaliado sozinho, sem Rede. Objetivo de compra por classe: Num. A = R$ 100, Num. B = R$ 50, Num. C = R$ 15, no bimestre móvel.',
    ficou: 'Igual ao previsto. Tipos de venda que contam: 1 e 9. Fluxos disponíveis: Faturado, Emitido e Faturado+Emitido. Janela de venda: a vigência cadastrada é de 1 mês (Setembro = 01/09 a 30/09), mas a apuração soma também o mês anterior: Setembro apura 01/08/2026 a 30/09/2026; em Outubro passa a 01/09/2026 a 31/10/2026. A tela mostra essa janela em um aviso fixo.',
  },
  {
    id: 'sort-num', grupo: 'Programa Numérica (por Cliente)', titulo: 'Sortimento Numérica (PPA)', escopo: 'ambos', status: 'parcial',
    previsto: 'Por cliente, quantos PPAs (famílias de itens) comprou. Vale só para Num. A e B. Teto de PPAs: 15 na HC e 7 na Food. Mínimo de 3 unidades quando a embalagem é unidade.',
    ficou:
      'Implementado com o teto de 15/7 e a classe C fora do Sortimento. O cartão de topo divide o total de PPAs pelos clientes que COMPRARAM algo da indústria no bimestre (decisão provisória).',
    nota: 'Diferenças com a planilha da JC: teto de PPAs (pergunta 1) e denominador (pergunta 7).',
  },
  {
    id: 'ppa-drill', grupo: 'Programa Numérica (por Cliente)', titulo: 'Quais PPAs o cliente comprou e não comprou', escopo: 'ambos', status: 'ok',
    previsto: 'Abrir o cliente e ver a lista de PPAs, destacando os que faltam vender.',
    ficou: 'Web e Mobile abrem a lista com os PPAs não comprados primeiro. PPAs vendidos abaixo do mínimo de 3 unidades aparecem com aviso, para não parecerem erro.',
  },
  {
    id: 'tipos-venda', grupo: 'Programa Numérica (por Cliente)', titulo: 'Tipos de venda considerados', escopo: 'ambos', status: 'parcial',
    previsto: 'Considerar as vendas elegíveis do programa.',
    ficou: 'Cobertura: tipos 1 e 9. Sortimento: tipos 1, 9 e 5. O tipo 10 fica de fora.',
    nota: 'Pergunta 6: o tipo 10 deveria entrar?',
  },

  // ── Dados, período e fluxo ──────────────────────────────────────────────
  {
    id: 'fluxo', grupo: 'Dados, período e fluxo', titulo: 'Fluxo: Faturado, Transmitido, Faturado + Transmitido', escopo: 'ambos', status: 'ok',
    previsto: 'Mostrar o Faturado e o Transmitido (Emitido), e a soma dos dois como reforço de curto prazo, em Ponderada e Numérica.',
    ficou: 'Desde 09/10/2026 as três visões existem na Ponderada e na Numérica, na web e no mobile (GGV, Supervisor e RCA). A soma é Faturado líquido de devolução/cancelamento + Transmitido.',
    nota: 'Pedido do Heverton em 09/10/2026 (Correção 1); respondeu a pergunta 8.',
  },
  {
    id: 'periodo', grupo: 'Dados, período e fluxo', titulo: 'Período de apuração', escopo: 'ambos', status: 'ok',
    previsto: 'Escolher o período: dia anterior, semana, mês, ano; e datas livres.',
    ficou:
      'Web: dia anterior, última semana, mês, ano corrente e período manual (de/até). Mobile: Ontem, semana, mês e ano. ' +
      'Os atalhos usam o último dia realmente importado (limitado a ontem), não o relógio — assim um dia sem carga não zera a tela.',
  },
  {
    id: 'vigencia', grupo: 'Dados, período e fluxo', titulo: 'Vigência, congelamento e atualização', escopo: 'ambos', status: 'ok',
    previsto: 'Resultado do mês fechado não deve mudar; o mês aberto acompanha a venda.',
    ficou: 'Vigência aberta é recalculada todo dia de madrugada (04:30); vigência fechada fica congelada. Isso é o que faz as telas abrirem em poucos segundos.',
  },
  {
    id: 'objetivo-carteira', grupo: 'Dados, período e fluxo', titulo: 'Objetivo por GGV / Supervisor / RCA', escopo: 'ambos', status: 'parcial',
    previsto: 'Cada nível mostra o seu objetivo.',
    ficou: 'O objetivo de cada equipe é calculado pelo tamanho da carteira (Redes ou Clientes que ela atende) — não é um número digitado por equipe.',
    nota: 'Pergunta 10: confirmar que é isso mesmo.',
  },
  {
    id: 'nomes', grupo: 'Dados, período e fluxo', titulo: 'Nomes de GGV, Supervisor e RCA', escopo: 'ambos', status: 'ok',
    previsto: 'Nomes iguais aos dos relatórios da JC.',
    ficou: 'O nome atual vem do cadastro da JC (CADRCA_JC), atualizado todo dia. O código manda; o nome acompanha a troca de pessoa no mesmo código.',
    nota: 'Aguardando Keslley confirmar o caso Gilson/Jocildo (código 350) e Fellipe (349).',
  },

  // ── Uso do painel ───────────────────────────────────────────────────────
  {
    id: 'projecao', grupo: 'Uso do painel', titulo: 'Projeção de fechamento', escopo: 'ambos', status: 'parcial',
    previsto: 'Em qualquer nível, ver para onde o mês caminha (ritmo atual × objetivo).',
    ficou: 'Mobile: aba "Projeção" ao lado de "Oficiais". Web: a projeção existia na visão individual, que saiu com a visão combinada — hoje não aparece na web.',
    nota: 'Pergunta 4.',
  },
  {
    id: 'excel', grupo: 'Uso do painel', titulo: 'Exportar para Excel', escopo: 'web', status: 'ok',
    previsto: 'Levar a tela para planilha.',
    ficou: 'Botão de exportação no painel web, respeitando os filtros da tela.',
  },
  {
    id: 'conferencia', grupo: 'Uso do painel', titulo: 'Conferência com a planilha da JC', escopo: 'web', status: 'ok',
    previsto: 'Poder bater o Farol contra o fechamento oficial da JC (Carlos).',
    ficou: 'Relatório comparativo da Numérica, com opção de incluir ou não a classe Num. C na conferência (pedido do Carlos).',
    nota: 'Pergunta 13.',
  },
  {
    id: 'email', grupo: 'Uso do painel', titulo: 'E-mail executivo / agente automático', escopo: 'web', status: 'pendente',
    previsto: 'Resumo periódico dos objetivos por indústria enviado à diretoria.',
    ficou: 'Funciona para a Ponderada. A Numérica ainda não entra nesse e-mail.',
    nota: 'Pergunta 3.',
  },
]

export const PERGUNTAS: Pergunta[] = [
  {
    n: 1, escopo: 'ambos', titulo: 'Teto de PPAs por cliente',
    contexto: 'O sistema limita a contagem a 15 PPAs (HC) e 7 (Food) por cliente. O arquivo de acompanhamento da JC não aplica o teto, e isso gera cerca de 1% de diferença.',
    hoje: 'Teto aplicado.',
    decidir: 'O teto vale ou não vale na apuração oficial? Se vale, a diferença é do arquivo; se não vale, retiramos do sistema.',
  },
  {
    n: 2, escopo: 'ambos', titulo: 'Limiar A/B/C e teto: por vínculo ou por vigência',
    contexto: 'Hoje o limiar de cada classe (100/50/15) e o teto ficam no vínculo da indústria, valendo para todos os meses. As faixas ficam na vigência.',
    hoje: 'Um valor único para todos os meses.',
    decidir: 'Se o desafio mudar de um mês para o outro, precisamos levar esses valores para a vigência. Isso muda o cadastro e exige fechar Setembro antes. Vai mudar?',
    decisao: 'Decidido pelo Heverton (out/2026): se mudar em Outubro, o sistema recalcula Setembro com o valor novo e a JC refaz Setembro. Os parâmetros continuam no vínculo.',
  },
  {
    n: 3, escopo: 'web', titulo: 'Numérica no e-mail executivo',
    contexto: 'O e-mail automático com o painel por indústria só traz a Ponderada.',
    hoje: 'Numérica fora do e-mail.',
    decidir: 'A diretoria quer receber também a Numérica? Em que formato (um bloco a mais ou e-mail separado)?',
  },
  {
    n: 4, escopo: 'web', titulo: 'Projeção de fechamento na web',
    contexto: 'Ao tirar as visões individuais (08/10), a projeção deixou de aparecer na web. No mobile ela continua.',
    hoje: 'Só no mobile.',
    decidir: 'Quer a projeção de volta na web, dentro da visão combinada? Em quais níveis?',
  },
  {
    n: 5, escopo: 'ambos', titulo: 'Lista de Outubro e vigência de Agosto da Numérica',
    contexto: 'A lista de clientes e PPAs de Outubro ainda não foi carregada, e a Numérica não tem vigência de Agosto.',
    hoje: 'Numérica começa em Setembro.',
    decidir: 'Quando recebemos a lista de Outubro? Precisamos de Agosto para a Numérica ou começamos em Setembro?',
  },
  {
    n: 6, escopo: 'ambos', titulo: 'Tipo de venda 10',
    contexto: 'Cobertura usa os tipos 1 e 9; Sortimento usa 1, 9 e 5. O tipo 10 não entra em nenhuma das duas.',
    hoje: 'Tipo 10 excluído.',
    decidir: 'Isso está certo? Há algum tipo de venda que devia entrar ou sair em cada métrica?',
  },
  {
    n: 7, escopo: 'ambos', titulo: 'Denominador da média de Sortimento Numérica',
    contexto: 'O total de PPAs é dividido pelos clientes que compraram algo da indústria no bimestre. A alternativa é dividir por todos os clientes da lista (comprando ou não).',
    hoje: 'Só quem comprou (decisão provisória nossa).',
    decidir: 'Qual é o denominador oficial do programa? A diferença muda o número do cartão de topo.',
  },
  {
    n: 8, escopo: 'ambos', titulo: 'Faturado + Emitido na Ponderada',
    contexto: 'A soma Faturado+Emitido foi decidida para a Numérica. O documento geral do programa fala em reforço de curto prazo sem dizer a qual programa se aplica.',
    hoje: 'Só na Numérica.',
    decidir: 'A Ponderada também deve ter a soma?',
    decisao: 'Sim. Heverton pediu em 09/10/2026 (Correção 1) a visão Faturado + Transmitido também na Ponderada; implementada.',
  },
  {
    n: 9, escopo: 'web', titulo: 'Página "AJUSTE" em branco no PDF enviado',
    contexto: 'No PDF "Ajustes IA – Unilever" uma das páginas veio em branco.',
    hoje: 'Não sabemos se havia pedido nessa página.',
    decidir: 'Existe algum ajuste que ficou fora do que recebemos? Pode reenviar essa página?',
  },
  {
    n: 10, escopo: 'ambos', titulo: 'Objetivo por equipe = tamanho da carteira',
    contexto: 'O objetivo de um GGV, Supervisor ou RCA é a soma dos objetivos de cada Rede/Cliente que ele atende.',
    hoje: 'Calculado, sem número manual por equipe.',
    decidir: 'Confirma que é assim? Ou cada equipe tem um objetivo próprio que deve ser digitado?',
  },
  {
    n: 11, escopo: 'ambos', titulo: 'Rede que fica entre duas equipes',
    contexto: 'Se uma Rede tiver lojas em equipes diferentes, hoje o dono é quem tem o menor CNPJ.',
    hoje: 'Regra provisória. Nenhum caso real hoje (134 Redes inteiras em uma equipe).',
    decidir: 'Qual deve ser a regra caso o caso apareça?',
  },
  {
    n: 12, escopo: 'ambos', titulo: 'Cliente com código diferente por filial',
    contexto: 'O mesmo CNPJ pode ter um código de cliente na filial 1 e outro na filial 12. Resolvemos agrupando por CNPJ.',
    hoje: 'Soma por CNPJ (validado: R$ 24.895,22 igual ao do Heverton).',
    decidir: 'A JC vai unificar os códigos no cadastro, ou continuaremos por CNPJ?',
  },
  {
    n: 13, escopo: 'web', titulo: 'Num. C na conferência com a planilha',
    contexto: 'O Carlos disse que o grupo Num. C não precisa entrar na conferência de Sortimento. Colocamos uma chave para ligar/desligar.',
    hoje: 'Chave disponível; padrão ligado.',
    decidir: 'Qual deve ser o padrão do relatório de conferência?',
  },
]
