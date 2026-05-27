/* ═══════════════════════════════════════════════════════════════════════
 * FAROL Maquete v0 — Lógica completa client-side
 *
 * Sem backend. Sem framework de build. Vanilla JS + PapaParse.
 *
 * Responsável por:
 *  1. Importar CSVs (Base Comparativa + Base Atual) via FileReader
 *  2. Calcular todos os indicadores (% atingimento, positivação, mix, etc.)
 *  3. Renderizar 3 visões hierárquicas com drill-down completo
 *  4. Farol binário verde/vermelho (≥100% verde, <100% vermelho)
 * ═══════════════════════════════════════════════════════════════════════ */

// ─── State global ────────────────────────────────────────────────────────
const state = {
  baseComparativa: [],   // array de rows normalizadas (base do ano anterior)
  baseAtual:       [],   // base do ano corrente
  persona:         'diretoria',  // diretoria | gerente | ggv | supervisor
  personaEntity:   null,         // código da entidade selecionada
  view:            'V01',        // V01 | V02 | V03
  drillPath:       [],           // [{level, value, label}, ...]
  importMode:      'real',       // 'real' (2 arquivos) | 'sim' (1 arquivo + %)
  simSourceRows:   [],           // rows carregadas no modo simulação
  simPct:          10,           // % de acréscimo/decréscimo aplicado
  // Comparativos temporais (apresentação ao gestor):
  compMode:        'yoy',        // yoy | ytd | mom
  refYear:         null,         // ano do mês de referência (default: mais recente)
  refMonth:        null,         // mês de referência (1-12)
}

// ─── Definição das 3 visões hierárquicas ────────────────────────────────
const HIERARQUIAS = {
  V01: [
    { level: 'cod_fornec',     label: 'Fornecedor',  nameField: 'nome_fornec' },
    { level: 'cod_gerente',    label: 'Gerente',     nameField: 'nome_gerente' },
    { level: 'cod_supervisor', label: 'Supervisor',  nameField: 'nome_supervisor' },
    { level: 'cod_rca',        label: 'RCA',         nameField: 'nome_rca' },
    { level: 'cod_cli',        label: 'Cliente',     nameField: 'nome_cli' },
    { level: 'cod_prod',       label: 'Produto',     nameField: 'nome_prod' },
  ],
  V02: [
    { level: 'cod_supervisor', label: 'Supervisor',  nameField: 'nome_supervisor' },
    { level: 'cod_rca',        label: 'RCA',         nameField: 'nome_rca' },
    { level: 'cod_fornec',     label: 'Fornecedor',  nameField: 'nome_fornec' },
    { level: 'cod_cli',        label: 'Cliente',     nameField: 'nome_cli' },
    { level: 'cod_prod',       label: 'Produto',     nameField: 'nome_prod' },
  ],
  V03: [
    { level: 'cod_fornec',     label: 'Fornecedor',  nameField: 'nome_fornec' },
    { level: 'empresa',        label: 'Empresa',     nameField: 'empresa' },
    { level: 'uf',             label: 'UF',          nameField: 'uf' },
    { level: 'cod_gerente',    label: 'Gerente',     nameField: 'nome_gerente' },
    { level: 'cod_supervisor', label: 'Supervisor',  nameField: 'nome_supervisor' },
    { level: 'cod_rca',        label: 'RCA',         nameField: 'nome_rca' },
    { level: 'cod_cli',        label: 'Cliente',     nameField: 'nome_cli' },
    { level: 'cod_prod',       label: 'Produto',     nameField: 'nome_prod' },
  ],
}

const TRAVA_MINIMA_DEFAULT = 1  // qt mínima para considerar positivado

// ─── Utilidades ──────────────────────────────────────────────────────────
function fmtBRL(v) {
  return v.toLocaleString('pt-BR', { style:'currency', currency:'BRL', minimumFractionDigits:0, maximumFractionDigits:0 })
}
function fmtPct(v) { return v.toFixed(0) + '%' }
function fmtNum(v) { return v.toLocaleString('pt-BR') }

function parseNum(s) {
  if (s == null) return 0
  if (typeof s === 'number') return s
  const cleaned = String(s).replace(/\./g, '').replace(',', '.').trim()
  const n = parseFloat(cleaned)
  return isNaN(n) ? 0 : n
}
function parseInt2(s) {
  if (s == null) return 0
  const n = parseInt(String(s).replace(/\D/g, ''), 10)
  return isNaN(n) ? 0 : n
}

// ─── Parser de período: extrai {ano, mes} de strings variadas ───────────
// Aceita: "2026-05", "2026/05", "05/2026", "Mai/2026", "2026-T1", "2026-S1", "2026"
const MES_ABREV = { JAN:1, FEV:2, MAR:3, ABR:4, MAI:5, JUN:6, JUL:7, AGO:8, SET:9, OUT:10, NOV:11, DEZ:12 }
function parsePeriodo(s) {
  if (!s) return null
  const str = String(s).toUpperCase().trim()
  let m
  // YYYY-MM ou YYYY/MM ou YYYY-MM-DD
  m = str.match(/(\d{4})[\/\-](\d{1,2})\b/)
  if (m && +m[2] >= 1 && +m[2] <= 12) return { ano: +m[1], mes: +m[2] }
  // MM/YYYY
  m = str.match(/^(\d{1,2})[\/\-](\d{4})/)
  if (m && +m[1] >= 1 && +m[1] <= 12) return { ano: +m[2], mes: +m[1] }
  // MMM/YYYY ou MMM-YYYY (Mai/2026)
  m = str.match(/([A-Z]{3})[\/\-](\d{4})/)
  if (m && MES_ABREV[m[1]]) return { ano: +m[2], mes: MES_ABREV[m[1]] }
  // YYYY-TN (trimestral) → primeiro mês do trimestre
  m = str.match(/(\d{4})[\/\-]T(\d)/)
  if (m && +m[2] >= 1 && +m[2] <= 4) return { ano: +m[1], mes: (+m[2] - 1) * 3 + 1 }
  // YYYY-SN (semestral) → primeiro mês do semestre
  m = str.match(/(\d{4})[\/\-]S(\d)/)
  if (m && +m[2] >= 1 && +m[2] <= 2) return { ano: +m[1], mes: (+m[2] - 1) * 6 + 1 }
  // Apenas YYYY
  m = str.match(/^(\d{4})$/)
  if (m) return { ano: +m[1], mes: 1 }
  return null
}

const MES_NOMES = ['', 'Jan','Fev','Mar','Abr','Mai','Jun','Jul','Ago','Set','Out','Nov','Dez']
function fmtPeriodo(ano, mes) {
  if (!ano) return '—'
  if (!mes) return String(ano)
  return `${MES_NOMES[mes]}/${ano}`
}

// Normaliza uma row do CSV. SLIM: só os campos usados pelo painel.
// Campos descartados (não usados ainda): cnpj, cod_ramo, ramo, embalagem,
// qt_unit, qt_unit_cx, ean, qtrca_supervisor, periodo. Adicione de volta
// aqui se forem usados em alguma feature.
//
// Para evitar OOM em arquivos grandes (>50MB), retorna null quando a row
// não tem identificação mínima (cod_fornec/cod_rca/cod_cli vazios) —
// economiza milhões de objetos descartados.
function normalizeRow(raw) {
  // Lookup case-insensitive sem criar objeto intermediário
  const lk = {}
  for (const k in raw) lk[k.trim().toLowerCase()] = raw[k]

  const codFornec = String(lk['codfornec'] || lk['cod_fornec'] || '')
  const codRca    = String(lk['codusur']   || lk['cod_rca']    || '')
  const codCli    = String(lk['codcli']    || lk['cod_cli']    || '')
  // Se nenhum identificador, descarta — economiza memória
  if (!codFornec && !codRca && !codCli) return null

  // Estado: detecta no PERIODO ou no campo ESTADO
  const periodo = lk['periodo'] || ''
  const estadoRaw = lk['estado'] || ''
  const estado = /trans/i.test(periodo) || /trans/i.test(estadoRaw)
    ? 'TRANSMITIDO'
    : 'FATURADO'

  // Ano e mês — para os comparativos temporais (YoY, YTD, MoM)
  const { ano, mes } = parsePeriodo(periodo) ||
                       parsePeriodo(lk['data']) ||
                       parsePeriodo(lk['mes_ref']) ||
                       { ano: null, mes: null }

  return {
    estado,
    ano,
    mes,
    cod_gerente:     String(lk['codgerente']    || lk['cod_gerente']    || ''),
    nome_gerente:    lk['gerente']  || lk['nome_gerente']  || '',
    cod_supervisor:  String(lk['codsupervisor'] || lk['cod_supervisor'] || ''),
    nome_supervisor: lk['supervisor'] || lk['nome_supervisor'] || '',
    cod_rca:         codRca,
    nome_rca:        lk['rca']        || lk['nome_rca']        || '',
    qtcli_rca:       parseInt2(lk['qtcli_rca']),
    cod_fornec:      codFornec,
    nome_fornec:     lk['fornecedor'] || lk['nome_fornec']     || '',
    cod_cli:         codCli,
    nome_cli:        lk['cliente']    || lk['nome_cli']        || '',
    uf:              lk['uf']         || '',
    empresa:         lk['empresa']    || 'EMPRESA',
    cod_prod:        String(lk['codprod'] || lk['cod_prod'] || ''),
    nome_prod:       lk['produto']    || lk['nome_prod']       || '',
    qt:              parseNum(lk['qt']),
    pvenda:          parseNum(lk['pvenda']),
  }
}

// ─── Importação CSV (chunk + worker pra não travar com arquivos grandes) ──
function loadCSV(file, dest, statusEl) {
  return new Promise((resolve, reject) => {
    const rows = []
    const totalBytes = file.size
    Papa.parse(file, {
      header: true,
      delimiter: ';',
      skipEmptyLines: true,
      worker: true,
      chunkSize: 1024 * 1024,  // 1 MB por chunk
      chunk: (results) => {
        for (const raw of results.data) {
          const r = normalizeRow(raw)
          if (r) rows.push(r)
        }
        // Posição real do parser (byte offset). Aproxima de 100% no último chunk.
        const cursor = results.meta?.cursor || 0
        const pct = totalBytes > 0 ? Math.min(99, Math.round((cursor / totalBytes) * 100)) : 0
        if (statusEl) statusEl.textContent = `⏳ Importando... ${pct}% • ${rows.length.toLocaleString('pt-BR')} linhas`
      },
      complete: () => {
        state[dest] = rows
        if (statusEl) statusEl.textContent = `✓ ${rows.length.toLocaleString('pt-BR')} linhas carregadas. Processando painel...`
        // Yield para o browser atualizar o texto antes do render pesado
        setTimeout(() => {
          if (statusEl) statusEl.textContent = `✓ ${rows.length.toLocaleString('pt-BR')} linhas`
          resolve(rows)
        }, 50)
      },
      error: reject,
    })
  })
}

// Carrega exemplos via fetch
async function loadSamples() {
  try {
    const [csv1, csv2] = await Promise.all([
      fetch('sample-data/base-comparativa-exemplo.csv').then(r => r.text()),
      fetch('sample-data/base-atual-exemplo.csv').then(r => r.text()),
    ])
    const parse = (text) => new Promise((res) => {
      Papa.parse(text, {
        header: true, delimiter: ';', skipEmptyLines: true,
        complete: (r) => res(r.data.map(normalizeRow).filter(Boolean)),
      })
    })
    state.baseComparativa = await parse(csv1)
    state.baseAtual = await parse(csv2)
    document.getElementById('statusComparativa').textContent = `✓ ${state.baseComparativa.length} linhas (exemplo)`
    document.getElementById('statusAtual').textContent = `✓ ${state.baseAtual.length} linhas (exemplo)`
    onDataLoaded()
  } catch (err) {
    alert('Erro ao carregar dados de exemplo: ' + err.message)
  }
}

// ─── Lógica de agregação ────────────────────────────────────────────────
// Cache de filtros usando WeakMap chaveado pelo array de origem (slice atual/anterior
// gera array novo a cada render, mas o drillPath só precisa filtrar se for diferente).
// Esta versão simplificada NÃO cacheia entre renders (slice gera arrays novos) mas
// o filtro é O(N) e razoavelmente rápido — ainda é melhor que recalcular indicadores.
function filterByDrill(rows) {
  if (state.drillPath.length === 0) return rows
  return rows.filter(row => state.drillPath.every(({level, value}) => row[level] === value))
}
function invalidateCache() { /* no-op nesta versão */ }

// ─── Comparativos temporais (YoY, YTD, MoM) ──────────────────────────────

// Lista de períodos (YYYY-MM) disponíveis em ambas as bases, ordenada do mais recente
function listAvailablePeriods() {
  const set = new Set()
  const add = r => { if (r.ano && r.mes) set.add(`${r.ano}-${String(r.mes).padStart(2,'0')}`) }
  state.baseAtual.forEach(add)
  state.baseComparativa.forEach(add)
  return Array.from(set).sort().reverse()
}

// Detecta período de referência mais recente nos dados
function autoDetectRefPeriod() {
  const periods = listAvailablePeriods()
  if (periods.length === 0) return
  const [y, m] = periods[0].split('-')
  state.refYear  = +y
  state.refMonth = +m
}

// Calcula slice atual/anterior + label + fator de projeção conforme o modo
function computePeriodSlice() {
  if (state.refYear == null || state.refMonth == null) autoDetectRefPeriod()
  const ry = state.refYear, rm = state.refMonth
  if (!ry || !rm) {
    return { atual: state.baseAtual, anterior: state.baseComparativa, projecaoFator: 1, label: '' }
  }
  const all = state.baseAtual.length + state.baseComparativa.length > 0
    ? state.baseAtual.concat(state.baseComparativa)
    : []
  let atual = [], anterior = [], projecaoFator = 1, label = ''
  if (state.compMode === 'yoy') {
    atual    = all.filter(r => r.ano === ry     && r.mes === rm)
    anterior = all.filter(r => r.ano === ry - 1 && r.mes === rm)
    label    = `${fmtPeriodo(ry, rm)} vs ${fmtPeriodo(ry - 1, rm)}`
  } else if (state.compMode === 'ytd') {
    atual    = all.filter(r => r.ano === ry     && r.mes <= rm)
    anterior = all.filter(r => r.ano === ry - 1)
    projecaoFator = rm > 0 ? 12 / rm : 1
    label    = `Projeção ${ry} (${fmtPeriodo(ry, 1)}–${fmtPeriodo(ry, rm)}) vs Total ${ry - 1}`
  } else if (state.compMode === 'mom') {
    const pm = rm === 1 ? 12 : rm - 1
    const py = rm === 1 ? ry - 1 : ry
    atual    = all.filter(r => r.ano === ry && r.mes === rm)
    anterior = all.filter(r => r.ano === py && r.mes === pm)
    label    = `${fmtPeriodo(ry, rm)} vs ${fmtPeriodo(py, pm)}`
  }
  return { atual, anterior, projecaoFator, label }
}

// Persona scope: limita o que cada persona enxerga
function applyPersonaScope(rows) {
  if (state.persona === 'diretoria' || !state.personaEntity) return rows
  if (state.persona === 'gerente')    return rows.filter(r => r.empresa        === state.personaEntity)
  if (state.persona === 'ggv')        return rows.filter(r => r.cod_gerente    === state.personaEntity)
  if (state.persona === 'supervisor') return rows.filter(r => r.cod_supervisor === state.personaEntity)
  return rows
}

// Extrai a lista de entidades disponíveis para a persona ativa (a partir dos dados carregados)
function listPersonaEntities() {
  const map = new Map()
  if (state.persona === 'gerente') {
    // Gerente vê uma empresa específica do grupo
    state.baseAtual.forEach(r => {
      if (r.empresa) map.set(r.empresa, r.empresa)
    })
  } else if (state.persona === 'ggv') {
    state.baseAtual.forEach(r => {
      if (r.cod_gerente) map.set(r.cod_gerente, r.nome_gerente || r.cod_gerente)
    })
  } else if (state.persona === 'supervisor') {
    state.baseAtual.forEach(r => {
      if (r.cod_supervisor) map.set(r.cod_supervisor, r.nome_supervisor || r.cod_supervisor)
    })
  } else {
    return []
  }
  return Array.from(map.entries())
    .map(([cod, nome]) => ({ cod, nome }))
    .sort((a, b) => a.nome.localeCompare(b.nome, 'pt-BR'))
}

// Mapeamento persona → layout (desktop = notebook, mobile = celular)
const PERSONA_LAYOUT = {
  diretoria:  'desktop',
  gerente:    'desktop',
  ggv:        'mobile',
  supervisor: 'mobile',
}

// Atualiza o segundo seletor e a visibilidade da aba Diretoria + classe de layout
function refreshPersonaUI() {
  const sel = document.getElementById('personaEntitySelect')
  const entities = listPersonaEntities()

  // Aplica classe de layout no <body> (CSS faz o resto)
  document.body.classList.remove('layout-desktop', 'layout-mobile')
  document.body.classList.add('layout-' + (PERSONA_LAYOUT[state.persona] || 'desktop'))

  if (state.persona === 'diretoria') {
    sel.classList.add('hidden')
    state.personaEntity = null
  } else {
    sel.classList.remove('hidden')
    const label =
      state.persona === 'gerente'    ? 'Selecione a empresa' :
      state.persona === 'ggv'        ? 'Selecione o GGV' :
      state.persona === 'supervisor' ? 'Selecione o Supervisor' :
      'Selecione...'
    sel.innerHTML = `<option value="">${label}</option>` +
      entities.map(e => `<option value="${escapeAttr(e.cod)}">${escapeHtml(e.cod)}${e.cod !== e.nome ? ' — ' + escapeHtml(e.nome) : ''}</option>`).join('')
    if (!entities.some(e => e.cod === state.personaEntity)) {
      state.personaEntity = entities[0]?.cod || null
    }
    sel.value = state.personaEntity || ''
  }

  // Visão Diretoria (V03) só para Diretoria + Gerente (personas com vista de grupo/empresa)
  const tabV03 = document.querySelector('.tab-btn[data-view="V03"]')
  if (tabV03) {
    const podeVerDiretoria = state.persona === 'diretoria' || state.persona === 'gerente'
    if (podeVerDiretoria) {
      tabV03.classList.remove('hidden')
    } else {
      tabV03.classList.add('hidden')
      if (state.view === 'V03') state.view = 'V01'
    }
  }
}

// Agrega rows pelo próximo nível da hierarquia (usando slice temporal)
function aggregate() {
  const hierarquia = HIERARQUIAS[state.view]
  const nextIdx = state.drillPath.length
  if (nextIdx >= hierarquia.length) return []

  const nextLevel = hierarquia[nextIdx]
  const slice = computePeriodSlice()
  const atual  = applyPersonaScope(filterByDrill(slice.atual))
  const compar = applyPersonaScope(filterByDrill(slice.anterior))

  const groups = new Map()
  function addToGroup(row, bucket) {
    const key = row[nextLevel.level]
    if (!key) return
    if (!groups.has(key)) {
      groups.set(key, {
        key,
        label: row[nextLevel.nameField] || row[nextLevel.level] || '—',
        atual: [],
        compar: [],
      })
    }
    groups.get(key)[bucket].push(row)
  }
  atual.forEach(r => addToGroup(r, 'atual'))
  compar.forEach(r => addToGroup(r, 'compar'))

  return Array.from(groups.values()).map(g => calcCard(g, slice.projecaoFator))
}

// Calcula tudo em UMA passada O(N). projecaoFator aplica quando modo=YTD (atual = realizado × 12/mes_ref)
function calcCard(g, projecaoFator = 1) {
  let valorAtual = 0, valorAnt = 0
  let faturado = 0, transmitido = 0
  let maxQtcliRca = 0
  const positSet   = new Set()  // clientes que bateram trava mínima
  const cliDistinct = new Set() // todos clientes (inclusive sem venda)
  const cliProds   = new Map()  // cliente → Set<produto> para mix

  for (let i = 0, n = g.atual.length; i < n; i++) {
    const r = g.atual[i]
    const pv = r.pvenda || 0
    valorAtual += pv
    if (r.estado === 'FATURADO')    faturado    += pv
    if (r.estado === 'TRANSMITIDO') transmitido += pv
    if (r.qtcli_rca > maxQtcliRca)  maxQtcliRca = r.qtcli_rca
    if (r.cod_cli) {
      cliDistinct.add(r.cod_cli)
      if (r.qt >= TRAVA_MINIMA_DEFAULT) positSet.add(r.cod_cli)
      if (r.qt > 0) {
        let prods = cliProds.get(r.cod_cli)
        if (!prods) { prods = new Set(); cliProds.set(r.cod_cli, prods) }
        if (r.cod_prod) prods.add(r.cod_prod)
      }
    }
  }
  for (let i = 0, n = g.compar.length; i < n; i++) {
    valorAnt += g.compar[i].pvenda || 0
  }

  // Aplica projeção quando modo YTD: valorAtual realizado é escalado para o ano todo
  const valorAtualProj = valorAtual * projecaoFator
  const pct           = valorAnt > 0 ? (valorAtualProj / valorAnt) * 100 : (valorAtualProj > 0 ? 100 : 0)
  const cor           = pct >= 100 ? 'verde' : 'vermelho'
  const projetado     = projecaoFator > 1.01
  // Sobrescreve valorAtual para refletir a projeção (cards mostram o número usado no cálculo)
  if (projetado) valorAtual = valorAtualProj
  const baseCli       = maxQtcliRca || cliDistinct.size
  const positivados   = positSet.size
  const positPct      = baseCli > 0 ? (positivados / baseCli) * 100 : 0
  let mix = 0
  if (cliProds.size > 0) {
    let total = 0
    cliProds.forEach(set => total += set.size)
    mix = total / cliProds.size
  }
  return { ...g, pct, cor, valorAtual, valorAnt, positivados, baseCli, positPct, mix, faturado, transmitido, projetado }
}

function sum(arr, field) { return arr.reduce((a, r) => a + (r[field] || 0), 0) }
function countDistinct(arr, field) { return new Set(arr.map(r => r[field])).size }
function countDistinctWhere(arr, field, pred) { return new Set(arr.filter(pred).map(r => r[field])).size }
function maxNum(arr, field) { return arr.reduce((a, r) => Math.max(a, r[field] || 0), 0) }
function uniqueBy(arr, field) { return Array.from(new Set(arr.map(r => r[field]))) }

// ─── Renderização ───────────────────────────────────────────────────────
function render() {
  if (state.baseAtual.length === 0 && state.baseComparativa.length === 0) {
    document.getElementById('emptyState').classList.remove('hidden')
    document.getElementById('mainContent').classList.add('hidden')
    return
  }
  document.getElementById('emptyState').classList.add('hidden')
  document.getElementById('mainContent').classList.remove('hidden')

  try {
    renderComparativeBar()
    renderTabs()
    renderBreadcrumb()
    renderKPIs()
    renderCards()
  } catch (err) {
    console.error('Erro ao renderizar painel:', err)
    document.getElementById('cardsGrid').innerHTML = `
      <div class="col-span-full bg-red-50 border border-red-200 rounded-xl p-6 text-red-700">
        <p class="font-semibold mb-2">⚠ Erro ao renderizar</p>
        <p class="text-sm">${escapeHtml(err.message)}</p>
        <pre class="text-xs mt-3 bg-white p-3 rounded border border-red-200 overflow-auto">${escapeHtml(err.stack || '')}</pre>
      </div>
    `
  }
}

function renderComparativeBar() {
  // Botões de modo: marca o ativo
  document.querySelectorAll('.comp-mode-btn').forEach(b => {
    b.classList.toggle('active', b.dataset.comp === state.compMode)
  })
  // Dropdown de período de referência: popula com os meses presentes nas bases
  const sel = document.getElementById('refPeriodoSelect')
  const periods = listAvailablePeriods()
  if (state.refYear == null || state.refMonth == null) autoDetectRefPeriod()
  const currentKey = state.refYear ? `${state.refYear}-${String(state.refMonth).padStart(2,'0')}` : ''
  sel.innerHTML = periods.map(p => {
    const [y, m] = p.split('-')
    const label = `${MES_NOMES[+m]}/${y}`
    return `<option value="${p}"${p === currentKey ? ' selected' : ''}>${label}</option>`
  }).join('') || '<option value="">Sem dados</option>'
  // Label do comparativo
  const slice = computePeriodSlice()
  document.getElementById('compLabel').textContent = slice.label
    ? `Comparando: ${slice.label}`
    : ''
}

function renderTabs() {
  document.querySelectorAll('.tab-btn').forEach(btn => {
    btn.classList.toggle('active', btn.dataset.view === state.view)
  })
}

function renderBreadcrumb() {
  const bc = document.getElementById('breadcrumb')
  const hierarquia = HIERARQUIAS[state.view]
  const viewNames = { V01: 'Por Fornecedor', V02: 'Por RCA', V03: 'Diretoria' }
  const html = []
  // Badge de persona quando filtrada
  if (state.persona !== 'diretoria' && state.personaEntity) {
    const entities = listPersonaEntities()
    const ent = entities.find(e => e.cod === state.personaEntity)
    const label = state.persona === 'ggv' ? '👤 GGV' : '👤 Supervisor'
    html.push(`<span class="breadcrumb-chip" style="background:#fef3c7;color:#92400e">${label}: ${escapeHtml(ent?.nome || state.personaEntity)}</span>`)
    html.push(`<span class="breadcrumb-sep">›</span>`)
  }
  html.push(`<span class="breadcrumb-chip" data-idx="-1">📊 ${viewNames[state.view]}</span>`)
  state.drillPath.forEach((seg, i) => {
    html.push(`<span class="breadcrumb-sep">›</span>`)
    const isLast = i === state.drillPath.length - 1
    html.push(`<span class="breadcrumb-chip ${isLast ? 'current' : ''}" data-idx="${i}">${seg.label}</span>`)
  })
  // Hint do próximo nível
  if (state.drillPath.length < hierarquia.length) {
    const nextLabel = hierarquia[state.drillPath.length].label
    html.push(`<span class="breadcrumb-sep">›</span>`)
    html.push(`<span class="text-xs text-slate-400">↓ ${nextLabel}</span>`)
  }
  bc.innerHTML = html.join('')

  bc.querySelectorAll('.breadcrumb-chip').forEach(chip => {
    chip.addEventListener('click', () => {
      const idx = parseInt(chip.dataset.idx, 10)
      // Truncar drillPath até o índice clicado
      state.drillPath = idx < 0 ? [] : state.drillPath.slice(0, idx + 1)
      render()
    })
  })
}

function renderKPIs() {
  const slice = computePeriodSlice()
  const atual  = applyPersonaScope(filterByDrill(slice.atual))
  const compar = applyPersonaScope(filterByDrill(slice.anterior))

  // Single-pass O(N) sobre atual: acumula tudo de uma vez
  let totAtual = 0, faturado = 0, transmitido = 0, maxQtcliRca = 0
  const positSet = new Set(), cliDistinct = new Set(), cliProds = new Map()
  for (let i = 0, n = atual.length; i < n; i++) {
    const r = atual[i]
    const pv = r.pvenda || 0
    totAtual += pv
    if (r.estado === 'FATURADO')    faturado    += pv
    if (r.estado === 'TRANSMITIDO') transmitido += pv
    if (r.qtcli_rca > maxQtcliRca)  maxQtcliRca = r.qtcli_rca
    if (r.cod_cli) {
      cliDistinct.add(r.cod_cli)
      if (r.qt >= TRAVA_MINIMA_DEFAULT) positSet.add(r.cod_cli)
      if (r.qt > 0) {
        let prods = cliProds.get(r.cod_cli)
        if (!prods) { prods = new Set(); cliProds.set(r.cod_cli, prods) }
        if (r.cod_prod) prods.add(r.cod_prod)
      }
    }
  }
  let totAnt = 0
  for (let i = 0, n = compar.length; i < n; i++) totAnt += compar[i].pvenda || 0

  // Aplica projeção no totAtual quando modo YTD
  const totAtualProj = totAtual * slice.projecaoFator
  const pctTotal    = totAnt > 0 ? (totAtualProj / totAnt) * 100 : (totAtualProj > 0 ? 100 : 0)
  const corTotal    = pctTotal >= 100 ? 'verde' : 'vermelho'
  const projetado   = slice.projecaoFator > 1.01
  const positivados = positSet.size
  const baseCli     = maxQtcliRca || cliDistinct.size
  const positPct    = baseCli > 0 ? (positivados / baseCli) * 100 : 0
  let mix = 0
  if (cliProds.size > 0) {
    let total = 0
    cliProds.forEach(set => total += set.size)
    mix = total / cliProds.size
  }

  document.getElementById('kpiStrip').innerHTML = `
    <div class="kpi-card">
      <p class="kpi-label">Obj. Anterior</p>
      <p class="kpi-value">${fmtBRL(totAnt)}</p>
    </div>
    <div class="kpi-card">
      <p class="kpi-label">${projetado ? 'Projeção ' + state.refYear : 'Obj. Atual'}</p>
      <p class="kpi-value">${fmtBRL(projetado ? totAtualProj : totAtual)}${projetado ? '<span class="text-xs text-slate-400 ml-1">*</span>' : ''}</p>
    </div>
    <div class="kpi-card">
      <p class="kpi-label">Atingimento</p>
      <p class="kpi-value" style="color:${corTotal === 'verde' ? '#16a34a' : '#dc2626'}">${fmtPct(pctTotal)}</p>
    </div>
    <div class="kpi-card highlight">
      <p class="kpi-label">Positivação</p>
      <p class="kpi-value">${positivados}/${baseCli} <span class="text-sm text-orange-400">(${fmtPct(positPct)})</span></p>
    </div>
    <div class="kpi-card">
      <p class="kpi-label">Mix (itens/cli)</p>
      <p class="kpi-value">${mix.toFixed(1)}</p>
    </div>
    <div class="kpi-card">
      <p class="kpi-label">Faturado / Transm.</p>
      <p class="kpi-value text-sm leading-tight">${fmtBRL(faturado)}<br><span class="text-slate-400 font-normal">${fmtBRL(transmitido)}</span></p>
    </div>
  `
}

function renderCards() {
  const grid = document.getElementById('cardsGrid')
  const items = aggregate().sort((a, b) => b.pct - a.pct)  // melhores primeiro

  if (items.length === 0) {
    const hier = HIERARQUIAS[state.view]
    const isLeaf = state.drillPath.length >= hier.length
    grid.innerHTML = `
      <div class="col-span-full bg-white border-2 border-dashed border-slate-200 rounded-xl p-10 text-center text-slate-400">
        ${isLeaf ? '🍃 Você chegou ao último nível da hierarquia.' : '📭 Nenhum dado neste recorte.'}
      </div>
    `
    return
  }

  grid.innerHTML = items.map(it => `
    <button class="entity-card cor-${it.cor}" data-key="${escapeAttr(it.key)}" data-label="${escapeAttr(it.label)}">
      <div class="progress-track">
        <div class="progress-fill" style="width: ${Math.min(it.pct, 100)}%"></div>
      </div>
      <div class="p-4">
        <div class="flex items-start justify-between gap-3 mb-2">
          <div class="flex-1 min-w-0">
            <p class="text-[10px] font-bold text-slate-400 uppercase tracking-wider">${escapeHtml(it.key)}</p>
            <p class="font-semibold text-slate-800 truncate">${escapeHtml(it.label)}</p>
          </div>
          <span class="semaforo cor-${it.cor}">${it.cor === 'verde' ? '✓' : '✕'}</span>
        </div>
        <p class="pct-big mt-2">${fmtPct(it.pct)}</p>
        <div class="grid grid-cols-2 gap-2 mt-3 pt-3 border-t border-slate-100">
          <div>
            <p class="text-[10px] text-slate-400 uppercase tracking-wider">Anterior</p>
            <p class="text-base font-bold text-slate-500 mt-0.5 tabular-nums">${fmtBRL(it.valorAnt)}</p>
          </div>
          <div>
            <p class="text-[10px] text-slate-400 uppercase tracking-wider">Atual</p>
            <p class="text-base font-bold text-slate-900 mt-0.5 tabular-nums">${fmtBRL(it.valorAtual)}</p>
          </div>
        </div>
        <div class="mt-3 flex items-center gap-3 text-xs text-slate-500">
          <span title="Positivação">👥 ${it.positivados}/${it.baseCli} (${fmtPct(it.positPct)})</span>
          <span title="Mix de itens">📦 ${it.mix.toFixed(1)} itens/cli</span>
        </div>
        <div class="mt-2 flex items-center gap-3 text-[11px] text-slate-400">
          <span>Fat. ${fmtBRL(it.faturado)}</span>
          <span>Tr. ${fmtBRL(it.transmitido)}</span>
        </div>
      </div>
    </button>
  `).join('')

  // Click handlers para drill-down
  grid.querySelectorAll('.entity-card').forEach(card => {
    card.addEventListener('click', () => {
      const hier = HIERARQUIAS[state.view]
      const nextLevel = hier[state.drillPath.length]
      if (!nextLevel) return  // já está no leaf
      state.drillPath.push({
        level: nextLevel.level,
        value: card.dataset.key,
        label: `${nextLevel.label}: ${card.dataset.label}`,
      })
      render()
    })
  })
}

function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))
}
function escapeAttr(s) {
  return escapeHtml(s)
}

// ─── Wire up dos controles ──────────────────────────────────────────────
function onDataLoaded() {
  document.getElementById('toggleImport').classList.remove('hidden')
  state.drillPath = []
  invalidateCache()
  refreshPersonaUI()
  render()
}

document.getElementById('fileComparativa').addEventListener('change', async (e) => {
  const f = e.target.files[0]; if (!f) return
  const statusEl = document.getElementById('statusComparativa')
  statusEl.textContent = '⏳ Iniciando...'
  await loadCSV(f, 'baseComparativa', statusEl)
  if (state.baseAtual.length > 0) onDataLoaded()
})
document.getElementById('fileAtual').addEventListener('change', async (e) => {
  const f = e.target.files[0]; if (!f) return
  const statusEl = document.getElementById('statusAtual')
  statusEl.textContent = '⏳ Iniciando...'
  await loadCSV(f, 'baseAtual', statusEl)
  if (state.baseComparativa.length > 0) onDataLoaded()
})
document.getElementById('btnLoadSample').addEventListener('click', loadSamples)
document.getElementById('toggleImport').addEventListener('click', () => {
  const sec = document.getElementById('importSection')
  const btn = document.getElementById('toggleImport')
  if (sec.classList.contains('compact')) {
    sec.classList.remove('compact')
    sec.querySelector('.p-5.grid').style.display = ''
    sec.querySelector('.px-5.pb-4').style.display = ''
    btn.textContent = 'Ocultar'
  } else {
    sec.classList.add('compact')
    sec.querySelector('.p-5.grid').style.display = 'none'
    sec.querySelector('.px-5.pb-4').style.display = 'none'
    btn.textContent = 'Mostrar'
  }
})
document.querySelectorAll('.tab-btn').forEach(btn => {
  btn.addEventListener('click', () => {
    state.view = btn.dataset.view
    state.drillPath = []
    render()
  })
})
document.querySelectorAll('.persona-tab').forEach(btn => {
  btn.addEventListener('click', () => {
    document.querySelectorAll('.persona-tab').forEach(b => b.classList.remove('active'))
    btn.classList.add('active')
    state.persona = btn.dataset.persona
    state.personaEntity = null
    state.drillPath = []
    refreshPersonaUI()
    render()
  })
})
document.getElementById('personaEntitySelect').addEventListener('change', (e) => {
  state.personaEntity = e.target.value || null
  state.drillPath = []
  render()
})

// ─── Modo simulação ──────────────────────────────────────────────────────
function switchImportMode(mode) {
  state.importMode = mode
  document.querySelectorAll('.mode-tab').forEach(b => b.classList.toggle('active', b.dataset.mode === mode))
  document.getElementById('modeReal').classList.toggle('hidden', mode !== 'real')
  document.getElementById('modeSim').classList.toggle('hidden',  mode !== 'sim')
}

function applySimulation() {
  if (state.simSourceRows.length === 0) return
  const fator = 1 + (state.simPct / 100)
  // Base Comparativa reusa o array fonte (sem clone — economia de memória em arquivos grandes)
  state.baseComparativa = state.simSourceRows
  // Base Atual: cria novo array só com qt/pvenda escalados; usa Object.assign pra ser mais rápido que spread
  const n = state.simSourceRows.length
  const atual = new Array(n)
  for (let i = 0; i < n; i++) {
    const r = state.simSourceRows[i]
    atual[i] = Object.assign({}, r, {
      qt:     Math.round(r.qt * fator * 100) / 100,
      pvenda: Math.round(r.pvenda * fator * 100) / 100,
    })
  }
  state.baseAtual = atual
  onDataLoaded()
}

function updateSimPct(v) {
  const n = Math.max(-100, Math.min(500, parseInt(v, 10) || 0))
  state.simPct = n
  document.getElementById('simPct').value   = n
  document.getElementById('simRange').value = Math.max(-100, Math.min(200, n))  // range tem limite menor
  document.getElementById('btnApplySim').disabled = state.simSourceRows.length === 0
}

document.querySelectorAll('.mode-tab').forEach(btn => {
  btn.addEventListener('click', () => switchImportMode(btn.dataset.mode))
})

document.getElementById('fileSim').addEventListener('change', async (e) => {
  const f = e.target.files[0]; if (!f) return
  const statusEl = document.getElementById('statusSim')
  statusEl.textContent = '⏳ Iniciando...'
  const rows = []
  let chunkCount = 0
  const totalBytes = f.size
  await new Promise((res) => Papa.parse(f, {
    header: true, delimiter: ';', skipEmptyLines: true,
    worker: true,
    chunkSize: 1024 * 1024,
    chunk: (r) => {
      for (const raw of r.data) {
        const x = normalizeRow(raw)
        if (x) rows.push(x)
      }
      const cursor = r.meta?.cursor || 0
      const pct = totalBytes > 0 ? Math.min(99, Math.round((cursor / totalBytes) * 100)) : 0
      statusEl.textContent = `⏳ Importando... ${pct}% • ${rows.length.toLocaleString('pt-BR')} linhas`
    },
    complete: () => res(),
  }))
  state.simSourceRows = rows
  statusEl.textContent = `✓ ${rows.length.toLocaleString('pt-BR')} linhas`
  document.getElementById('btnApplySim').disabled = false
})

document.getElementById('simPct').addEventListener('input',   (e) => updateSimPct(e.target.value))
document.getElementById('simRange').addEventListener('input', (e) => updateSimPct(e.target.value))
document.getElementById('simStep').addEventListener('click',   () => updateSimPct(state.simPct - 5))
document.getElementById('simStepUp').addEventListener('click', () => updateSimPct(state.simPct + 5))
document.getElementById('btnApplySim').addEventListener('click', applySimulation)

// Botões de comparativo (YoY / YTD / MoM)
document.querySelectorAll('.comp-mode-btn').forEach(btn => {
  btn.addEventListener('click', () => {
    state.compMode = btn.dataset.comp
    state.drillPath = []
    invalidateCache()
    render()
  })
})
// Dropdown de período de referência
document.getElementById('refPeriodoSelect').addEventListener('change', (e) => {
  const v = e.target.value
  if (!v) return
  const [y, m] = v.split('-')
  state.refYear  = +y
  state.refMonth = +m
  state.drillPath = []
  invalidateCache()
  render()
})

// Inicializa tab e layout
state.view = 'V01'
renderTabs()
updateSimPct(state.simPct)
document.body.classList.add('layout-' + (PERSONA_LAYOUT[state.persona] || 'desktop'))
