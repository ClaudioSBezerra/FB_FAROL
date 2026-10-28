import type * as XLSXType from 'xlsx'

// ─── Importação de Clientes Válidos direto do .xlsx da Unilever ───────────────
//
// O modelo real que a JC manda ("Unico Acompanhamento Ponderadas..."), aba
// "BASE LOJAS", tem as colunas CNPJ/COD PRINC/COD CL/RAZAO/FANTASIA/GGV COD/
// GGV NOME/CRV COD/CRV NOME/RCA COD/RCA NOME — quase o mesmo layout do CSV que
// o backend já aceita (`cnpj;cod_princ;razao;fantasia;cod_ggv;nome_ggv;
// cod_crv;nome_crv;cod_rca;nome_rca`), só que largo (colunas nomeadas
// diferente, sem "COD CL"). Backend continua só CSV (nunca viu um xlsx) — a
// tradução acontece aqui, no navegador, antes do upload.
//
// Cada candidato é tentado nessa ordem — cobre tanto o nome exato do modelo
// real quanto variações razoáveis (ex.: "COD_GGV" num arquivo editado à mão).
export const CLIENTES_XLSX_COLS: { field: string; candidates: string[] }[] = [
  { field: 'cnpj', candidates: ['CNPJ'] },
  { field: 'cod_princ', candidates: ['COD PRINC', 'COD_PRINC', 'CODPRINC'] },
  { field: 'razao', candidates: ['RAZAO', 'RAZÃO', 'RAZAO SOCIAL', 'RAZÃO SOCIAL'] },
  { field: 'fantasia', candidates: ['FANTASIA'] },
  { field: 'cod_ggv', candidates: ['GGV COD', 'COD GGV', 'COD_GGV'] },
  { field: 'nome_ggv', candidates: ['GGV NOME', 'NOME GGV', 'NOME_GGV'] },
  { field: 'cod_crv', candidates: ['CRV COD', 'COD CRV', 'COD_CRV'] },
  { field: 'nome_crv', candidates: ['CRV NOME', 'NOME CRV', 'NOME_CRV'] },
  { field: 'cod_rca', candidates: ['RCA COD', 'COD RCA', 'COD_RCA'] },
  { field: 'nome_rca', candidates: ['RCA NOME', 'NOME RCA', 'NOME_RCA'] },
]

export function normalizaHeaderXlsx(s: unknown): string {
  return String(s ?? '').trim().toUpperCase().replace(/\s+/g, ' ')
}

// csvEscape — só entra em aspas quando o valor tem ';', '"' ou quebra de
// linha (regra padrão de CSV); dobra aspas internas.
export function csvEscape(v: string): string {
  if (/[;"\n\r]/.test(v)) return '"' + v.replace(/"/g, '""') + '"'
  return v
}

// acharAbaBaseLojas — o arquivo real tem várias abas ("Resumo Redes",
// "BASE EANS", etc.); procura pela que tem "LOJA" no nome (cobre "BASE
// LOJAS" e "BASE DE LOJAS", as duas variações já vistas).
export function acharAbaBaseLojas(wb: XLSXType.WorkBook): string | null {
  const porNome = wb.SheetNames.find(n => normalizaHeaderXlsx(n).includes('LOJA'))
  return porNome ?? wb.SheetNames[0] ?? null
}

// parseClientesValidosXlsx — lê o .xlsx no navegador e devolve o mesmo CSV
// (';') que o endpoint de importação já espera. CNPJ lido como NÚMERO (perde
// zero à esquerda no Excel) é preenchido de volta pra 14 dígitos; como texto,
// só limpa não-dígitos — o backend valida os 14 dígitos de qualquer jeito.
//
// import() dinâmico de propósito: a lib xlsx (~430kB) já é lazy-loaded pro
// resto do app (SpDashboard/exportToExcel) — import estático aqui empurraria
// ela pro bundle principal de novo (visto no build: +435kB no chunk main).
export async function parseClientesValidosXlsx(file: File): Promise<string> {
  const XLSX = await import('xlsx')
  const buf = await file.arrayBuffer()
  const wb = XLSX.read(buf, { type: 'array', cellText: false, cellDates: false })
  const abaNome = acharAbaBaseLojas(wb)
  if (!abaNome) throw new Error('Planilha sem nenhuma aba — arquivo .xlsx inválido')
  const sheet = wb.Sheets[abaNome]
  const linhas = XLSX.utils.sheet_to_json<unknown[]>(sheet, { header: 1, raw: true, defval: '' })
  if (linhas.length === 0) throw new Error(`Aba "${abaNome}" está vazia`)

  const headerRow = linhas[0].map(normalizaHeaderXlsx)
  const idxPorCampo: Record<string, number> = {}
  const faltando: string[] = []
  for (const { field, candidates } of CLIENTES_XLSX_COLS) {
    const idx = headerRow.findIndex(h => candidates.includes(h))
    if (idx === -1) {
      // razao/fantasia/nome_* são só rótulo — o backend aceita vazio.
      if (field.startsWith('nome_') || field === 'razao' || field === 'fantasia') continue
      faltando.push(candidates[0])
      continue
    }
    idxPorCampo[field] = idx
  }
  if (faltando.length > 0) {
    throw new Error(`Aba "${abaNome}" sem a(s) coluna(s): ${faltando.join(', ')}`)
  }

  const header = CLIENTES_XLSX_COLS.map(c => c.field)
  const out = [header.join(';')]
  for (let i = 1; i < linhas.length; i++) {
    const row = linhas[i]
    if (!row || row.length === 0) continue
    const valores = header.map(field => {
      const idx = idxPorCampo[field]
      if (idx === undefined) return ''
      const raw = row[idx]
      if (raw === undefined || raw === null) return ''
      if (field === 'cnpj' && typeof raw === 'number') {
        return String(Math.trunc(raw)).padStart(14, '0')
      }
      return csvEscape(String(raw).trim())
    })
    // Pula linha totalmente vazia (aba costuma ter linhas sobrando no fim).
    if (valores.every(v => v === '')) continue
    out.push(valores.join(';'))
  }
  return out.join('\r\n')
}

// ─── Numérica: Clientes (aba BASE LOJAS) e PPAs (aba BASE PPA's) ──────────────
// Mesma conversão de backend/tools/numerica_xlsx_convert.go, agora no
// navegador — o administrador solta o .xlsx da JC e a tela envia o CSV que os
// importadores já aceitam. Cada indústria tem seu layout; novos layouts entram
// em PARSERS_POR_FORMULA (fim do arquivo) sem mexer na tela.

function cnpj14(v: unknown): string {
  const d = String(v ?? '').replace(/\D/g, '')
  return d === '' ? '' : d.padStart(14, '0')
}

function acharColuna(header: string[], candidatos: string[]): number {
  for (const c of candidatos) {
    const i = header.indexOf(normalizaHeaderXlsx(c))
    if (i !== -1) return i
  }
  return -1
}

async function lerAba(file: File, achar: (n: string) => boolean, rotulo: string) {
  const XLSX = await import('xlsx')
  const wb = XLSX.read(await file.arrayBuffer(), { type: 'array', cellText: false, cellDates: false })
  const nome = wb.SheetNames.find(n => achar(normalizaHeaderXlsx(n)))
  if (!nome) throw new Error(`Arquivo sem a aba ${rotulo}`)
  const linhas = XLSX.utils.sheet_to_json<unknown[]>(wb.Sheets[nome], { header: 1, raw: true, defval: '' })
  if (linhas.length < 2) throw new Error(`Aba "${nome}" está vazia`)
  return { nome, linhas, header: linhas[0].map(normalizaHeaderXlsx) }
}

export async function parseClientesNumericasXlsx(file: File): Promise<string> {
  const { nome, linhas, header } = await lerAba(file, n => n.includes('BASE LOJAS'), '"BASE LOJAS"')
  const campos: { f: string; c: string[]; obrig: boolean }[] = [
    { f: 'cnpj', c: ['CNPJ'], obrig: true },
    { f: 'cod_cl', c: ['CODCL', 'COD CL'], obrig: false },
    { f: 'classificacao_pdv', c: ['Classificação PDV', 'CLASSIFICACAO PDV'], obrig: true },
    { f: 'razao', c: ['RAZAO', 'RAZÃO'], obrig: false },
    { f: 'fantasia', c: ['FANTASIA'], obrig: false },
    { f: 'cod_ggv', c: ['GGV COD'], obrig: false },
    { f: 'nome_ggv', c: ['GGV NOME'], obrig: false },
    { f: 'cod_crv', c: ['CRV COD'], obrig: false },
    { f: 'nome_crv', c: ['CRV NOME'], obrig: false },
    { f: 'cod_rca', c: ['RCA COD'], obrig: false },
    { f: 'nome_rca', c: ['RCA NOME'], obrig: false },
  ]
  const idx = campos.map(x => acharColuna(header, x.c))
  const faltando = campos.filter((x, i) => x.obrig && idx[i] === -1).map(x => x.c[0])
  if (faltando.length) throw new Error(`Aba "${nome}" sem a(s) coluna(s): ${faltando.join(', ')}`)
  const out = [campos.map(x => x.f).join(';')]
  for (let r = 1; r < linhas.length; r++) {
    const row = linhas[r]
    if (!row || cnpj14(row[idx[0]]) === '') continue
    out.push(campos.map((x, i) => {
      if (idx[i] === -1) return ''
      const raw = row[idx[i]]
      if (x.f === 'cnpj') return cnpj14(raw)
      return csvEscape(String(raw ?? '').trim())
    }).join(';'))
  }
  return out.join('\r\n')
}

export async function parsePPAsXlsx(file: File): Promise<string> {
  const { nome, linhas, header } = await lerAba(file, n => n.includes('BASE PPA'), '"BASE PPA\'s"')
  const campos: { f: string; c: string[] }[] = [
    { f: 'cod_prod', c: ['Cod JC', 'COD JC'] },
    { f: 'ppa_nome', c: ['PPA', 'PPA / FAMILIA', 'PPA/FAMILIA'] },
    { f: 'ean', c: ['EAN Regular', 'EAN'] },
    { f: 'embalagem', c: ['Embalagem'] },
    { f: 'regiao', c: ['Região do Sortimento'] },
    { f: 'ae', c: ['AE'] },
    { f: 'bu', c: ['BU'] },
  ]
  const idx = campos.map(x => acharColuna(header, x.c))
  if (idx[0] === -1 || idx[1] === -1) throw new Error(`Aba "${nome}" precisa das colunas "Cod JC" e "PPA"`)
  const out = [campos.map(x => x.f).join(';')]
  let semCodigo = 0
  for (let r = 1; r < linhas.length; r++) {
    const row = linhas[r]
    if (!row) continue
    const ppa = String(row[idx[1]] ?? '').trim()
    if (ppa === '') continue
    const cod = String(row[idx[0]] ?? '').trim().replace(/\.0+$/, '')
    // "0" é o placeholder da JC pra produto ainda não mapeado ao código interno.
    if (cod === '' || cod === '0') { semCodigo++; continue }
    out.push(campos.map((_, i) => {
      if (idx[i] === -1) return ''
      const raw = i === 0 ? cod : String(row[idx[i]] ?? '').trim()
      return csvEscape(String(raw))
    }).join(';'))
  }
  if (out.length === 1) throw new Error(`Aba "${nome}" sem nenhum PPA com Cod JC preenchido`)
  void semCodigo
  return out.join('\r\n')
}

// ─── Segmento Foco (Reckitt): grupos de produtos e objetivos por UF ───────────

// Lista de SKUs válidos ("SKUs Validos segmento foco.xlsx"): abas COD MATRIZ
// (caixa) e COD PA (unidade), colunas EAN, COD JC, PRODUTO, EMBALAGEM,
// CATEGORIA (= grupo). Um mesmo cod_prod vale uma vez só.
export async function parseSegmentoFocoGruposXlsx(file: File): Promise<string> {
  const XLSX = await import('xlsx')
  const wb = XLSX.read(await file.arrayBuffer(), { type: 'array', cellText: false, cellDates: false })
  const out = ['grupo;cod_prod;produto;ean;embalagem']
  const vistos = new Set<string>()
  let abasLidas = 0
  for (const nome of wb.SheetNames) {
    const linhas = XLSX.utils.sheet_to_json<unknown[]>(wb.Sheets[nome], { header: 1, raw: true, defval: '' })
    if (linhas.length < 2) continue
    const header = linhas[0].map(normalizaHeaderXlsx)
    const iCod = acharColuna(header, ['COD JC', 'COD_JC', 'CODPROD'])
    const iGrupo = acharColuna(header, ['CATEGORIA', 'GRUPO'])
    if (iCod === -1 || iGrupo === -1) continue
    const iProd = acharColuna(header, ['PRODUTO', 'DESCRICAO'])
    const iEan = acharColuna(header, ['EAN'])
    const iEmb = acharColuna(header, ['EMBALAGEM'])
    abasLidas++
    for (let r = 1; r < linhas.length; r++) {
      const row = linhas[r]
      const cod = String(row[iCod] ?? '').trim().replace(/\.0+$/, '')
      const grupo = String(row[iGrupo] ?? '').replace(/\s+/g, ' ').trim()
      if (!cod || cod === '0' || !grupo || vistos.has(cod)) continue
      vistos.add(cod)
      const v = (i: number) => (i === -1 ? '' : csvEscape(String(row[i] ?? '').trim()))
      out.push([csvEscape(grupo), cod, v(iProd), v(iEan), v(iEmb)].join(';'))
    }
  }
  if (abasLidas === 0) throw new Error('Nenhuma aba com as colunas "COD JC" e "CATEGORIA" — é o arquivo de SKUs válidos do Segmento Foco?')
  if (out.length === 1) throw new Error('Nenhum produto com grupo e código preenchidos')
  return out.join('\r\n')
}

// Hierarquia RCA ("HIERARQUIA RCA.xlsx"): 1ª coluna (sem título) = "GGV - 3",
// EQP = código da equipe (CRV), SUPERVISOR = nome da equipe, RCA = código do RCA.
// A UF do programa vem do GGV (o backend traduz: 3 e 4 = GO, 2 = TO, 347 = MS, 5 = PA).
export async function parseSegmentoFocoHierarquiaXlsx(file: File): Promise<string> {
  const XLSX = await import('xlsx')
  const wb = XLSX.read(await file.arrayBuffer(), { type: 'array', cellText: false, cellDates: false })
  const out = ['rca;ggv;crv;nome_crv;nome_ggv']
  const vistos = new Set<string>()
  for (const nome of wb.SheetNames) {
    const linhas = XLSX.utils.sheet_to_json<unknown[]>(wb.Sheets[nome], { header: 1, raw: true, defval: '' })
    if (linhas.length < 2) continue
    const header = linhas[0].map(normalizaHeaderXlsx)
    const iRca = acharColuna(header, ['RCA', 'COD RCA'])
    const iEqp = acharColuna(header, ['EQP', 'CRV'])
    const iSup = acharColuna(header, ['SUPERVISOR'])
    if (iRca === -1 || iEqp === -1) continue
    for (let r = 1; r < linhas.length; r++) {
      const row = linhas[r]
      const rca = String(row[iRca] ?? '').trim().replace(/\.0+$/, '')
      const ggvTxt = String(row[0] ?? '').trim()
      const ggv = (ggvTxt.match(/\d+/) ?? [''])[0]
      if (!rca || !/^\d+$/.test(rca) || !ggv || vistos.has(rca)) continue
      vistos.add(rca)
      const crv = String(row[iEqp] ?? '').trim().replace(/\.0+$/, '')
      out.push([rca, ggv, csvEscape(crv), csvEscape(iSup === -1 ? '' : String(row[iSup] ?? '').trim()), csvEscape(ggvTxt)].join(';'))
    }
    if (out.length > 1) break
  }
  if (out.length === 1) throw new Error('Nenhuma linha com GGV e RCA — é o arquivo HIERARQUIA RCA?')
  return out.join('\r\n')
}

// A planilha chama o grupo de "Limpeza Pesada" de VLP no painel.
const ALIAS_GRUPO: Record<string, string> = { 'VEJA VLP': 'VEJA LIMPEZA PESADA' }

const MESES_ABREV = ['JAN', 'FEV', 'MAR', 'ABR', 'MAI', 'JUN', 'JUL', 'AGO', 'SET', 'OUT', 'NOV', 'DEZ']

export interface ObjetivosSegmentoFoco {
  // mês (1-12) → CSV "uf;grupo;objetivo"
  porMes: Record<number, string>
}

// Objetivos: aba "PAINEL FORMULA" — um bloco por UF (rótulo GO, MS, TO, PA...),
// com os meses do trimestre lado a lado (JUL/AGO/SET, cada um com OBJETIVO,
// REALIZADO, GAP) e uma linha "POSITIVAÇÃO - <grupo>" por grupo. Só as 3
// primeiras colunas de mês entram (a planilha tem um bloco extra à direita).
export async function parseSegmentoFocoObjetivosXlsx(file: File): Promise<ObjetivosSegmentoFoco> {
  const XLSX = await import('xlsx')
  const wb = XLSX.read(await file.arrayBuffer(), { type: 'array', cellText: false, cellDates: false })
  const nome = wb.SheetNames.find(n => normalizaHeaderXlsx(n).includes('PAINEL'))
  if (!nome) throw new Error('Arquivo sem a aba "PAINEL FORMULA"')
  const linhas = XLSX.utils.sheet_to_json<unknown[]>(wb.Sheets[nome], { header: 1, raw: true, defval: '' })

  const porMes: Record<number, string[]> = {}
  let uf = ''
  let colunasMes: Array<{ col: number; mes: number }> = []
  for (const row of linhas) {
    const cel = (i: number) => String(row[i] ?? '').replace(/\s+/g, ' ').trim()
    // rótulo de UF: texto curto sozinho na coluna E (índice 4)
    const c4 = cel(4)
    if (c4 && /^[A-Z]{2}$/.test(c4) && !row.slice(5).some(x => x !== '') && !row.slice(0, 4).some(x => x !== '')) {
      uf = c4; colunasMes = []; continue
    }
    // cabeçalho de meses: JUL/AGO/SET em colunas ≤ 9
    const meses = row.map((x, col) => ({ col, mes: MESES_ABREV.indexOf(String(x).trim().toUpperCase().slice(0, 3)) }))
      .filter(m => m.mes >= 0 && m.col <= 9 && String(row[m.col]).trim().length <= 4)
    if (meses.length >= 2 && !cel(0).toUpperCase().startsWith('POSITIVA')) { colunasMes = meses; continue }
    const kpi = cel(0).toUpperCase()
    if (!uf || colunasMes.length === 0 || !kpi.startsWith('POSITIVA')) continue
    let grupo = kpi.replace(/^POSITIVA[ÇC][ÃA]O\s*[-–]\s*/, '').trim()
    grupo = ALIAS_GRUPO[grupo] ?? grupo
    for (const { col, mes } of colunasMes) {
      const v = row[col]
      const n = typeof v === 'number' ? v : Number(String(v).replace(',', '.'))
      if (!Number.isFinite(n)) continue
      ;(porMes[mes + 1] ??= []).push(`${uf};${grupo};${Math.round(n * 10000) / 10000}`)
    }
  }
  const meses = Object.keys(porMes)
  if (meses.length === 0) throw new Error('Nenhum objetivo encontrado — esperado blocos por UF com linhas "POSITIVAÇÃO - <grupo>"')
  const out: Record<number, string> = {}
  for (const m of meses) out[Number(m)] = ['uf;grupo;objetivo', ...porMes[Number(m)]].join('\r\n')
  return { porMes: out }
}
