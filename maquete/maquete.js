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
  baseComparativa: [],   // array de rows normalizadas
  baseAtual:       [],
  persona:         'diretoria',  // diretoria | ggv | supervisor
  personaEntity:   null,         // código da entidade selecionada (cod_gerente OU cod_supervisor)
  view:            'V01',        // V01 | V02 | V03
  drillPath:       [],           // [{level, value, label}, ...]
  importMode:      'real',       // 'real' (2 arquivos) | 'sim' (1 arquivo + %)
  simSourceRows:   [],           // rows carregadas no modo simulação (fonte do clone)
  simPct:          10,           // % de acréscimo/decréscimo aplicado
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

// Normaliza uma row do CSV: lower-case keys, parsing numérico, descoberta de estado
function normalizeRow(raw) {
  const r = {}
  for (const k of Object.keys(raw)) {
    r[k.trim().toLowerCase()] = raw[k]
  }
  // Possíveis variações de nomes de campos no CSV (flexível)
  const periodo = r['periodo'] || ''
  const isFaturado = /fat/i.test(periodo) || /fat/i.test(r['estado'] || '')
  const isTrans    = /trans/i.test(periodo) || /trans/i.test(r['estado'] || '')

  return {
    periodo:           periodo,
    estado:            isFaturado ? 'FATURADO' : (isTrans ? 'TRANSMITIDO' : (r['estado'] || 'FATURADO')),
    cod_gerente:       String(r['codgerente'] || r['cod_gerente'] || ''),
    nome_gerente:      r['gerente'] || r['nome_gerente'] || '',
    cod_supervisor:    String(r['codsupervisor'] || r['cod_supervisor'] || ''),
    nome_supervisor:   r['supervisor'] || r['nome_supervisor'] || '',
    qtrca_supervisor:  parseInt2(r['qtrca_supervisor']),
    cod_rca:           String(r['codusur'] || r['cod_rca'] || ''),
    nome_rca:          r['rca'] || r['nome_rca'] || '',
    qtcli_rca:         parseInt2(r['qtcli_rca']),
    cod_fornec:        String(r['codfornec'] || r['cod_fornec'] || ''),
    nome_fornec:       r['fornecedor'] || r['nome_fornec'] || '',
    cod_cli:           String(r['codcli'] || r['cod_cli'] || ''),
    nome_cli:          r['cliente'] || r['nome_cli'] || '',
    cnpj:              r['cnpj'] || '',
    cod_ramo:          String(r['codramo'] || r['cod_ramo'] || ''),
    ramo:              r['ramo'] || '',
    uf:                r['uf'] || '',
    empresa:           r['empresa'] || 'EMPRESA',  // pode vir do CSV ou ficar fixo
    cod_prod:          String(r['codprod'] || r['cod_prod'] || ''),
    nome_prod:         r['produto'] || r['nome_prod'] || '',
    embalagem:         r['embalagem'] || '',
    qt_unit:           parseInt2(r['qtunit']),
    qt_unit_cx:        parseInt2(r['qtunitcx']),
    ean:               r['ean'] || '',
    qt:                parseNum(r['qt']),
    pvenda:            parseNum(r['pvenda']),
  }
}

// ─── Importação CSV ─────────────────────────────────────────────────────
function loadCSV(file, dest) {
  return new Promise((resolve, reject) => {
    Papa.parse(file, {
      header: true,
      delimiter: ';',
      skipEmptyLines: true,
      complete: (results) => {
        const rows = results.data.map(normalizeRow).filter(r => r.cod_fornec || r.cod_rca || r.cod_cli)
        state[dest] = rows
        resolve(rows)
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
        complete: (r) => res(r.data.map(normalizeRow).filter(r => r.cod_fornec || r.cod_rca || r.cod_cli)),
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
function filterByDrill(rows) {
  return rows.filter(row =>
    state.drillPath.every(({level, value}) => row[level] === value)
  )
}

// Persona scope: limita o que cada persona enxerga
function applyPersonaScope(rows) {
  if (state.persona === 'diretoria' || !state.personaEntity) return rows
  if (state.persona === 'ggv')        return rows.filter(r => r.cod_gerente    === state.personaEntity)
  if (state.persona === 'supervisor') return rows.filter(r => r.cod_supervisor === state.personaEntity)
  return rows
}

// Extrai a lista de entidades disponíveis para a persona ativa (a partir dos dados carregados)
function listPersonaEntities() {
  if (state.persona === 'ggv') {
    const map = new Map()
    state.baseAtual.forEach(r => {
      if (r.cod_gerente) map.set(r.cod_gerente, r.nome_gerente || r.cod_gerente)
    })
    return Array.from(map.entries())
      .map(([cod, nome]) => ({ cod, nome }))
      .sort((a, b) => a.nome.localeCompare(b.nome, 'pt-BR'))
  }
  if (state.persona === 'supervisor') {
    const map = new Map()
    state.baseAtual.forEach(r => {
      if (r.cod_supervisor) map.set(r.cod_supervisor, r.nome_supervisor || r.cod_supervisor)
    })
    return Array.from(map.entries())
      .map(([cod, nome]) => ({ cod, nome }))
      .sort((a, b) => a.nome.localeCompare(b.nome, 'pt-BR'))
  }
  return []
}

// Atualiza o segundo seletor (GGV/Supervisor específico) e a visibilidade da aba Diretoria
function refreshPersonaUI() {
  const sel = document.getElementById('personaEntitySelect')
  const entities = listPersonaEntities()

  if (state.persona === 'diretoria') {
    sel.classList.add('hidden')
    state.personaEntity = null
  } else {
    sel.classList.remove('hidden')
    const label = state.persona === 'ggv' ? 'Selecione o GGV' : 'Selecione o Supervisor'
    sel.innerHTML = `<option value="">${label}</option>` +
      entities.map(e => `<option value="${escapeAttr(e.cod)}">${escapeHtml(e.cod)} — ${escapeHtml(e.nome)}</option>`).join('')
    // Mantém seleção atual se ainda válida; senão escolhe a primeira entidade
    if (!entities.some(e => e.cod === state.personaEntity)) {
      state.personaEntity = entities[0]?.cod || null
    }
    sel.value = state.personaEntity || ''
  }

  // Diretoria (V03) só faz sentido para a persona Diretoria
  const tabV03 = document.querySelector('.tab-btn[data-view="V03"]')
  if (tabV03) {
    if (state.persona === 'diretoria') {
      tabV03.classList.remove('hidden')
    } else {
      tabV03.classList.add('hidden')
      if (state.view === 'V03') state.view = 'V01'
    }
  }
}

// Agrega rows pelo próximo nível da hierarquia
function aggregate() {
  const hierarquia = HIERARQUIAS[state.view]
  const nextIdx = state.drillPath.length
  if (nextIdx >= hierarquia.length) return []  // chegou no fim

  const nextLevel = hierarquia[nextIdx]
  const atual = applyPersonaScope(filterByDrill(state.baseAtual))
  const compar = applyPersonaScope(filterByDrill(state.baseComparativa))

  const groups = new Map()

  function addToGroup(row, bucket) {
    const key = row[nextLevel.level]
    if (!key) return  // pula rows sem identificação no nível
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

  return Array.from(groups.values()).map(calcCard)
}

function calcCard(g) {
  const valorAtual    = sum(g.atual, 'pvenda')
  const valorAnt      = sum(g.compar, 'pvenda')
  const pct           = valorAnt > 0 ? (valorAtual / valorAnt) * 100 : (valorAtual > 0 ? 100 : 0)
  const cor           = pct >= 100 ? 'verde' : 'vermelho'

  // Positivação: clientes distintos com qt >= TRAVA, sobre qtcli_rca (declarado) ou clientes distintos
  const positivados = countDistinctWhere(g.atual, 'cod_cli', r => r.qt >= TRAVA_MINIMA_DEFAULT)
  const baseCli = maxNum(g.atual, 'qtcli_rca') || countDistinct(g.atual, 'cod_cli')
  const positPct = baseCli > 0 ? (positivados / baseCli) * 100 : 0

  // Mix: média de produtos distintos por cliente ativo
  const ativos = uniqueBy(g.atual.filter(r => r.qt > 0), 'cod_cli')
  let mix = 0
  if (ativos.length > 0) {
    const totProds = ativos.reduce((acc, cli) => {
      const prodSet = new Set(g.atual.filter(r => r.cod_cli === cli && r.qt > 0).map(r => r.cod_prod))
      return acc + prodSet.size
    }, 0)
    mix = totProds / ativos.length
  }

  const faturado    = sum(g.atual.filter(r => r.estado === 'FATURADO'),    'pvenda')
  const transmitido = sum(g.atual.filter(r => r.estado === 'TRANSMITIDO'), 'pvenda')

  return { ...g, pct, cor, valorAtual, valorAnt, positivados, baseCli, positPct, mix, faturado, transmitido }
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

  renderTabs()
  renderBreadcrumb()
  renderKPIs()
  renderCards()
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
  const atual = applyPersonaScope(filterByDrill(state.baseAtual))
  const compar = applyPersonaScope(filterByDrill(state.baseComparativa))

  const totAtual    = sum(atual, 'pvenda')
  const totAnt      = sum(compar, 'pvenda')
  const pctTotal    = totAnt > 0 ? (totAtual / totAnt) * 100 : (totAtual > 0 ? 100 : 0)
  const corTotal    = pctTotal >= 100 ? 'verde' : 'vermelho'

  const positivados = countDistinctWhere(atual, 'cod_cli', r => r.qt >= TRAVA_MINIMA_DEFAULT)
  const baseCli     = maxNum(atual, 'qtcli_rca') || countDistinct(atual, 'cod_cli')
  const positPct    = baseCli > 0 ? (positivados / baseCli) * 100 : 0

  const ativos = uniqueBy(atual.filter(r => r.qt > 0), 'cod_cli')
  let mix = 0
  if (ativos.length > 0) {
    const totProds = ativos.reduce((acc, cli) => {
      const prodSet = new Set(atual.filter(r => r.cod_cli === cli && r.qt > 0).map(r => r.cod_prod))
      return acc + prodSet.size
    }, 0)
    mix = totProds / ativos.length
  }

  const faturado    = sum(atual.filter(r => r.estado === 'FATURADO'),    'pvenda')
  const transmitido = sum(atual.filter(r => r.estado === 'TRANSMITIDO'), 'pvenda')

  document.getElementById('kpiStrip').innerHTML = `
    <div class="kpi-card">
      <p class="kpi-label">Obj. Anterior</p>
      <p class="kpi-value">${fmtBRL(totAnt)}</p>
    </div>
    <div class="kpi-card">
      <p class="kpi-label">Obj. Atual</p>
      <p class="kpi-value">${fmtBRL(totAtual)}</p>
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
  refreshPersonaUI()
  render()
}

document.getElementById('fileComparativa').addEventListener('change', async (e) => {
  const f = e.target.files[0]; if (!f) return
  document.getElementById('statusComparativa').textContent = '⏳ Importando...'
  await loadCSV(f, 'baseComparativa')
  document.getElementById('statusComparativa').textContent = `✓ ${state.baseComparativa.length} linhas`
  if (state.baseAtual.length > 0) onDataLoaded()
})
document.getElementById('fileAtual').addEventListener('change', async (e) => {
  const f = e.target.files[0]; if (!f) return
  document.getElementById('statusAtual').textContent = '⏳ Importando...'
  await loadCSV(f, 'baseAtual')
  document.getElementById('statusAtual').textContent = `✓ ${state.baseAtual.length} linhas`
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
  // Base Comparativa = arquivo original (sem mudança)
  state.baseComparativa = state.simSourceRows.map(r => ({ ...r }))
  // Base Atual = clone com qt e pvenda multiplicados pelo fator
  state.baseAtual = state.simSourceRows.map(r => ({
    ...r,
    qt:     Math.round(r.qt * fator * 100) / 100,
    pvenda: Math.round(r.pvenda * fator * 100) / 100,
  }))
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
  document.getElementById('statusSim').textContent = '⏳ Importando...'
  await new Promise((res) => Papa.parse(f, {
    header: true, delimiter: ';', skipEmptyLines: true,
    complete: (r) => { state.simSourceRows = r.data.map(normalizeRow).filter(x => x.cod_fornec || x.cod_rca || x.cod_cli); res() },
  }))
  document.getElementById('statusSim').textContent = `✓ ${state.simSourceRows.length} linhas`
  document.getElementById('btnApplySim').disabled = false
})

document.getElementById('simPct').addEventListener('input',   (e) => updateSimPct(e.target.value))
document.getElementById('simRange').addEventListener('input', (e) => updateSimPct(e.target.value))
document.getElementById('simStep').addEventListener('click',   () => updateSimPct(state.simPct - 5))
document.getElementById('simStepUp').addEventListener('click', () => updateSimPct(state.simPct + 5))
document.getElementById('btnApplySim').addEventListener('click', applySimulation)

// Inicializa tab
state.view = 'V01'
renderTabs()
updateSimPct(state.simPct)
