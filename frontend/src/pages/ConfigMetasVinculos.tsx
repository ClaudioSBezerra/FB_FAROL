import { useMemo, useRef, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel,
  AlertDialogContent, AlertDialogDescription, AlertDialogFooter,
  AlertDialogHeader, AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { toast } from 'sonner'
import { Plus, Pencil, Trash2, CalendarClock, Lock, X, Upload } from 'lucide-react'
import { useAuth } from '@/contexts/AuthContext'
import type * as XLSXType from 'xlsx'

// ─── Types ────────────────────────────────────────────────────────────────────

interface ParametroSchema {
  key: string
  label: string
  type: string
}

interface Industria { id: number; nome: string }

interface TipoMetrica {
  id: number
  nome: string
  nivel_agregacao: string
  parametros_schema: ParametroSchema[]
}

interface Faixa {
  faixa: number
  valor_meta: number
}

interface Vigencia {
  id: number
  vinculo_id: number
  data_inicio: string
  data_fim: string
  status: 'aberta' | 'fechada'
  faixas: Faixa[]
}

interface MetaVinculo {
  id: number
  industria_id: number
  industria_nome: string
  tipo_metrica_id: number
  tipo_metrica_nome: string
  parametros_schema: ParametroSchema[]
  parametros_valores: Record<string, unknown>
  ativo: boolean
  recorte_uf: string
  recorte_ggvs: string[]
  tipos_venda_validos: string[]
}

const EMPTY_FORM = {
  industria_id: '',
  tipo_metrica_id: '',
  ativo: true,
  parametros_valores: {} as Record<string, string>,
  recorte_uf: '',
  recorte_ggvs: '', // texto separado por vírgula na UI, vira array só no submit
  tipos_venda_validos: '', // idem — vazio = usa o "Líquido" padrão do Farol
}

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
const CLIENTES_XLSX_COLS: { field: string; candidates: string[] }[] = [
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

function normalizaHeaderXlsx(s: unknown): string {
  return String(s ?? '').trim().toUpperCase().replace(/\s+/g, ' ')
}

// csvEscape — só entra em aspas quando o valor tem ';', '"' ou quebra de
// linha (regra padrão de CSV); dobra aspas internas.
function csvEscape(v: string): string {
  if (/[;"\n\r]/.test(v)) return '"' + v.replace(/"/g, '""') + '"'
  return v
}

// acharAbaBaseLojas — o arquivo real tem várias abas ("Resumo Redes",
// "BASE EANS", etc.); procura pela que tem "LOJA" no nome (cobre "BASE
// LOJAS" e "BASE DE LOJAS", as duas variações já vistas).
function acharAbaBaseLojas(wb: XLSXType.WorkBook): string | null {
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
async function parseClientesValidosXlsx(file: File): Promise<string> {
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

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function ConfigMetasVinculos() {
  const { token } = useAuth()
  const qc = useQueryClient()
  const headers = useMemo(() => ({ Authorization: `Bearer ${token}` }), [token])

  const [editTarget, setEditTarget] = useState<MetaVinculo | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<MetaVinculo | null>(null)
  const [showDialog, setShowDialog] = useState(false)
  const [form, setForm] = useState(EMPTY_FORM)
  const [vigenciasTarget, setVigenciasTarget] = useState<MetaVinculo | null>(null)

  const { data: vinculos = [], isLoading } = useQuery<MetaVinculo[]>({
    queryKey: ['farol-metas-vinculos'],
    queryFn: async () => {
      const r = await fetch('/api/farol/metas-vinculos', { headers })
      if (!r.ok) throw new Error('Erro ao carregar vínculos')
      return r.json()
    },
  })

  const { data: industrias = [] } = useQuery<Industria[]>({
    queryKey: ['farol-industrias'],
    queryFn: async () => {
      const r = await fetch('/api/farol/industrias', { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
  })

  const { data: tiposMetrica = [] } = useQuery<TipoMetrica[]>({
    queryKey: ['farol-tipos-metrica'],
    queryFn: async () => {
      const r = await fetch('/api/farol/tipos-metrica', { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
  })

  const tipoSelecionado = tiposMetrica.find(t => String(t.id) === form.tipo_metrica_id)

  const save = useMutation({
    mutationFn: async () => {
      const url = editTarget ? `/api/farol/metas-vinculos/${editTarget.id}` : '/api/farol/metas-vinculos'
      const parametrosValores: Record<string, unknown> = {}
      for (const [k, v] of Object.entries(form.parametros_valores)) {
        const schemaParam = tipoSelecionado?.parametros_schema.find(p => p.key === k)
        parametrosValores[k] = schemaParam?.type === 'number' || schemaParam?.type === 'integer' ? Number(v) : v
      }
      const r = await fetch(url, {
        method: editTarget ? 'PUT' : 'POST',
        headers: { ...headers, 'Content-Type': 'application/json' },
        body: JSON.stringify({
          industria_id: Number(form.industria_id),
          tipo_metrica_id: Number(form.tipo_metrica_id),
          ativo: form.ativo,
          parametros_valores: parametrosValores,
          recorte_uf: form.recorte_uf.trim().toUpperCase(),
          recorte_ggvs: form.recorte_ggvs.split(',').map(s => s.trim()).filter(Boolean),
          tipos_venda_validos: form.tipos_venda_validos.split(',').map(s => s.trim()).filter(Boolean),
        }),
      })
      if (!r.ok) throw new Error((await r.text()) || 'Erro ao salvar vínculo')
    },
    onSuccess: () => {
      toast.success(editTarget ? 'Vínculo atualizado' : 'Vínculo criado')
      qc.invalidateQueries({ queryKey: ['farol-metas-vinculos'] })
      setShowDialog(false)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const remove = useMutation({
    mutationFn: async (id: number) => {
      const r = await fetch(`/api/farol/metas-vinculos/${id}`, { method: 'DELETE', headers })
      if (!r.ok) throw new Error((await r.text()) || 'Erro ao remover vínculo')
    },
    onSuccess: () => {
      toast.success('Vínculo removido')
      qc.invalidateQueries({ queryKey: ['farol-metas-vinculos'] })
      setDeleteTarget(null)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const fileInputRef = useRef<HTMLInputElement>(null)

  const importarCSV = useMutation({
    mutationFn: async (file: File) => {
      const body = new FormData()
      body.append('file', file)
      const r = await fetch('/api/farol/metas-vinculos-importar-csv', { method: 'POST', headers, body })
      const data = await r.json()
      if (!r.ok) {
        const erros = Array.isArray(data?.erros)
          ? data.erros.map((e: { linha: number; erro: string }) => `Linha ${e.linha || '-'}: ${e.erro}`).join('\n')
          : (data?.error ?? 'Erro ao importar CSV')
        throw new Error(erros)
      }
      return data as { vigencias_criadas: number; linhas_processadas: number }
    },
    onSuccess: data => {
      toast.success(`${data.vigencias_criadas} vigência(s) criada(s) a partir de ${data.linhas_processadas} linha(s)`)
      qc.invalidateQueries({ queryKey: ['farol-metas-vigencias'] })
    },
    onError: (e: Error) => toast.error(e.message, { style: { whiteSpace: 'pre-line' } }),
  })

  function onCSVSelected(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (file) importarCSV.mutate(file)
    e.target.value = ''
  }

  function openCreate() {
    setEditTarget(null)
    setForm(EMPTY_FORM)
    setShowDialog(true)
  }

  function openEdit(v: MetaVinculo) {
    setEditTarget(v)
    const valores: Record<string, string> = {}
    for (const p of v.parametros_schema) {
      valores[p.key] = String(v.parametros_valores[p.key] ?? '')
    }
    setForm({
      industria_id: String(v.industria_id),
      tipo_metrica_id: String(v.tipo_metrica_id),
      ativo: v.ativo,
      parametros_valores: valores,
      recorte_uf: v.recorte_uf ?? '',
      recorte_ggvs: (v.recorte_ggvs ?? []).join(', '),
      tipos_venda_validos: (v.tipos_venda_validos ?? []).join(', '),
    })
    setShowDialog(true)
  }

  function onTipoChange(tipoId: string) {
    const tipo = tiposMetrica.find(t => String(t.id) === tipoId)
    const valores: Record<string, string> = {}
    for (const p of tipo?.parametros_schema ?? []) {
      valores[p.key] = ''
    }
    setForm(f => ({ ...f, tipo_metrica_id: tipoId, parametros_valores: valores }))
  }

  return (
    <div className="p-6 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">Configuração de Metas por Indústria</h1>
          <p className="text-sm text-muted-foreground">
            Vincula uma Indústria a um Tipo de Métrica, com os valores de parâmetro específicos dela (ex: limiar de Cobertura).
            Valores de meta por faixa/vigência ficam em outra tela.
          </p>
        </div>
        <div className="flex gap-2">
          <input ref={fileInputRef} type="file" accept=".csv" className="hidden" onChange={onCSVSelected} />
          <Button variant="outline" size="sm" disabled={importarCSV.isPending} onClick={() => fileInputRef.current?.click()}>
            <Upload className="w-4 h-4 mr-1" /> {importarCSV.isPending ? 'Importando...' : 'Importar Metas (CSV)'}
          </Button>
          <Button onClick={openCreate} size="sm" disabled={industrias.length === 0 || tiposMetrica.length === 0}>
            <Plus className="w-4 h-4 mr-1" /> Novo Vínculo
          </Button>
        </div>
      </div>

      <div className="border rounded-lg overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Indústria</TableHead>
              <TableHead>Tipo de Métrica</TableHead>
              <TableHead>Parâmetros</TableHead>
              <TableHead>Recorte</TableHead>
              <TableHead className="w-20 text-center">Status</TableHead>
              <TableHead className="w-20" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading && (
              <TableRow><TableCell colSpan={6} className="text-center py-8 text-muted-foreground">Carregando...</TableCell></TableRow>
            )}
            {!isLoading && vinculos.length === 0 && (
              <TableRow><TableCell colSpan={6} className="text-center py-8 text-muted-foreground">Nenhum vínculo cadastrado</TableCell></TableRow>
            )}
            {vinculos.map(v => (
              <TableRow key={v.id}>
                <TableCell className="font-medium">{v.industria_nome}</TableCell>
                <TableCell>{v.tipo_metrica_nome}</TableCell>
                <TableCell>
                  <div className="flex flex-wrap gap-1">
                    {v.parametros_schema.map(p => (
                      <span key={p.key} className="inline-flex items-center gap-1 font-mono text-xs bg-slate-100 px-1.5 py-0.5 rounded">
                        {p.label}: {String(v.parametros_valores[p.key] ?? '—')}
                      </span>
                    ))}
                  </div>
                </TableCell>
                <TableCell className="text-xs text-muted-foreground">
                  {v.recorte_uf || v.recorte_ggvs.length > 0
                    ? [v.recorte_uf, ...v.recorte_ggvs].filter(Boolean).join(' · ')
                    : 'Empresa toda'}
                </TableCell>
                <TableCell className="text-center">
                  <Badge variant={v.ativo ? 'default' : 'secondary'}>{v.ativo ? 'Ativo' : 'Inativo'}</Badge>
                </TableCell>
                <TableCell>
                  <div className="flex gap-1 justify-end">
                    <Button variant="ghost" size="icon" title="Vigências e metas" onClick={() => setVigenciasTarget(v)}>
                      <CalendarClock className="w-4 h-4" />
                    </Button>
                    <Button variant="ghost" size="icon" onClick={() => openEdit(v)}>
                      <Pencil className="w-4 h-4" />
                    </Button>
                    <Button variant="ghost" size="icon" className="text-destructive" onClick={() => setDeleteTarget(v)}>
                      <Trash2 className="w-4 h-4" />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      {/* Modal CRUD */}
      <Dialog open={showDialog} onOpenChange={setShowDialog}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>{editTarget ? 'Editar Vínculo' : 'Novo Vínculo'}</DialogTitle>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-1">
              <Label>Indústria</Label>
              <Select value={form.industria_id} onValueChange={v => setForm(f => ({ ...f, industria_id: v }))}>
                <SelectTrigger><SelectValue placeholder="Selecione" /></SelectTrigger>
                <SelectContent>
                  {industrias.map(i => (
                    <SelectItem key={i.id} value={String(i.id)}>{i.nome}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <Label>Tipo de Métrica</Label>
              <Select value={form.tipo_metrica_id} onValueChange={onTipoChange}>
                <SelectTrigger><SelectValue placeholder="Selecione" /></SelectTrigger>
                <SelectContent>
                  {tiposMetrica.map(t => (
                    <SelectItem key={t.id} value={String(t.id)}>{t.nome}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {tipoSelecionado && tipoSelecionado.parametros_schema.length > 0 && (
              <div className="space-y-2 border-t pt-3">
                <Label className="text-xs text-muted-foreground">Parâmetros de "{tipoSelecionado.nome}"</Label>
                {tipoSelecionado.parametros_schema.map(p => (
                  <div key={p.key} className="space-y-1">
                    <Label className="text-sm">{p.label}</Label>
                    <Input
                      type={p.type === 'number' || p.type === 'integer' ? 'number' : 'text'}
                      value={form.parametros_valores[p.key] ?? ''}
                      onChange={e => setForm(f => ({
                        ...f,
                        parametros_valores: { ...f.parametros_valores, [p.key]: e.target.value },
                      }))}
                    />
                  </div>
                ))}
              </div>
            )}

            <div className="space-y-2 border-t pt-3">
              <Label className="text-xs text-muted-foreground">Recorte organizacional (opcional — vazio = empresa toda)</Label>
              <div className="flex gap-2">
                <div className="space-y-1 w-24">
                  <Label className="text-sm">UF</Label>
                  <Input
                    value={form.recorte_uf}
                    onChange={e => setForm(f => ({ ...f, recorte_uf: e.target.value }))}
                    placeholder="GO"
                    maxLength={2}
                  />
                </div>
                <div className="space-y-1 flex-1">
                  <Label className="text-sm">GGVs (separados por vírgula)</Label>
                  <Input
                    value={form.recorte_ggvs}
                    onChange={e => setForm(f => ({ ...f, recorte_ggvs: e.target.value }))}
                    placeholder="GO, GO FOOD, V7, DF"
                  />
                </div>
              </div>
            </div>

            <div className="space-y-1">
              <Label className="text-sm">Tipos de venda válidos (opcional — vazio = "Líquido" padrão do Farol)</Label>
              <Input
                value={form.tipos_venda_validos}
                onChange={e => setForm(f => ({ ...f, tipos_venda_validos: e.target.value }))}
                placeholder="1, 9"
              />
            </div>

            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                id="ativo-vinculo"
                checked={form.ativo}
                onChange={e => setForm(f => ({ ...f, ativo: e.target.checked }))}
                className="h-4 w-4"
              />
              <Label htmlFor="ativo-vinculo">Ativo</Label>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowDialog(false)}>Cancelar</Button>
            <Button
              onClick={() => save.mutate()}
              disabled={!form.industria_id || !form.tipo_metrica_id || save.isPending}
            >
              {save.isPending ? 'Salvando...' : 'Salvar'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Confirmar exclusão */}
      <AlertDialog open={!!deleteTarget} onOpenChange={v => !v && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Remover vínculo?</AlertDialogTitle>
            <AlertDialogDescription>
              O vínculo entre <strong>{deleteTarget?.industria_nome}</strong> e <strong>{deleteTarget?.tipo_metrica_nome}</strong> será removido.
              Esta ação não pode ser desfeita.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancelar</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground"
              disabled={remove.isPending}
              onClick={() => deleteTarget && remove.mutate(deleteTarget.id)}
            >
              Remover
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      {/* Vigências e faixas do vínculo selecionado */}
      {vigenciasTarget && (
        <VigenciasDialog
          vinculo={vigenciasTarget}
          headers={headers}
          onClose={() => setVigenciasTarget(null)}
        />
      )}
    </div>
  )
}

// ─── VigenciasDialog — gestão de vigências e faixas de um vínculo ─────────────

const EMPTY_VIGENCIA_FORM = {
  data_inicio: '',
  data_fim: '',
  faixas: [{ faixa: 1, valor_meta: '' }] as { faixa: number; valor_meta: string }[],
}

function VigenciasDialog({ vinculo, headers, onClose }: {
  vinculo: MetaVinculo
  headers: Record<string, string>
  onClose: () => void
}) {
  const qc = useQueryClient()
  const [form, setForm] = useState(EMPTY_VIGENCIA_FORM)
  const [showForm, setShowForm] = useState(false)
  const clientesFileInputRef = useRef<HTMLInputElement>(null)
  const [clientesUploadTarget, setClientesUploadTarget] = useState<number | null>(null)

  const importarClientes = useMutation({
    mutationFn: async ({ vigenciaId, file }: { vigenciaId: number; file: File }) => {
      const body = new FormData()
      body.append('file', file)
      const r = await fetch(`/api/farol/metas-clientes-validos-importar-csv?vinculo_id=${vinculo.id}&vigencia_id=${vigenciaId}`, {
        method: 'POST', headers, body,
      })
      const data = await r.json()
      if (!r.ok) {
        const erros = Array.isArray(data?.erros)
          ? data.erros.map((e: { linha: number; erro: string }) => `Linha ${e.linha || '-'}: ${e.erro}`).join('\n')
          : (data?.error ?? 'Erro ao importar Clientes Válidos')
        throw new Error(erros)
      }
      return data as { clientes_importados: number; avisos?: { linha: number; erro: string }[] }
    },
    onSuccess: data => {
      toast.success(`${data.clientes_importados} cliente(s) válido(s) importado(s)`)
      // Importação PARCIAL (08/09/2026) — linha com problema não trava mais
      // o resto do arquivo, só vira aviso. Mostra separado do toast de
      // sucesso pra não escrever por cima dele.
      if (data.avisos && data.avisos.length > 0) {
        const linhas = data.avisos.map(a => `Linha ${a.linha || '-'}: ${a.erro}`).join('\n')
        toast.warning(`${data.avisos.length} linha(s) ignorada(s) (não entraram):`, {
          description: linhas, style: { whiteSpace: 'pre-line' }, duration: 15000,
        })
      }
    },
    onError: (e: Error) => toast.error(e.message, { style: { whiteSpace: 'pre-line' } }),
  })

  async function onClientesCSVSelected(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    const vigenciaId = clientesUploadTarget
    e.target.value = ''
    if (!file || !vigenciaId) return

    const ehXlsx = /\.xlsx?$/i.test(file.name)
    if (!ehXlsx) {
      importarClientes.mutate({ vigenciaId, file })
      return
    }
    // .xlsx/.xls: converte pro mesmo CSV que o backend já aceita, no
    // navegador — o endpoint nunca viu um xlsx, só recebe o resultado.
    try {
      const csvText = await parseClientesValidosXlsx(file)
      const csvFile = new File([csvText], file.name.replace(/\.xlsx?$/i, '.csv'), { type: 'text/csv' })
      importarClientes.mutate({ vigenciaId, file: csvFile })
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Erro ao ler o .xlsx')
    }
  }

  const itensFileInputRef = useRef<HTMLInputElement>(null)
  const [itensUploadTarget, setItensUploadTarget] = useState<number | null>(null)

  const importarItens = useMutation({
    mutationFn: async ({ vigenciaId, file }: { vigenciaId: number; file: File }) => {
      const body = new FormData()
      body.append('file', file)
      const r = await fetch(`/api/farol/metas-itens-validos-importar-csv?vinculo_id=${vinculo.id}&vigencia_id=${vigenciaId}`, {
        method: 'POST', headers, body,
      })
      const data = await r.json()
      if (!r.ok) {
        const erros = Array.isArray(data?.erros)
          ? data.erros.map((e: { linha: number; erro: string }) => `Linha ${e.linha || '-'}: ${e.erro}`).join('\n')
          : (data?.error ?? 'Erro ao importar Itens Válidos')
        throw new Error(erros)
      }
      return data as { itens_importados: number }
    },
    onSuccess: data => toast.success(`${data.itens_importados} item(ns) válido(s) importado(s)`),
    onError: (e: Error) => toast.error(e.message, { style: { whiteSpace: 'pre-line' } }),
  })

  function onItensCSVSelected(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (file && itensUploadTarget) importarItens.mutate({ vigenciaId: itensUploadTarget, file })
    e.target.value = ''
  }

  const { data: vigencias = [], isLoading } = useQuery<Vigencia[]>({
    queryKey: ['farol-metas-vigencias', vinculo.id],
    queryFn: async () => {
      const r = await fetch(`/api/farol/metas-vigencias?vinculo_id=${vinculo.id}`, { headers })
      if (!r.ok) throw new Error('Erro ao carregar vigências')
      return r.json()
    },
  })

  const criar = useMutation({
    mutationFn: async () => {
      const r = await fetch('/api/farol/metas-vigencias', {
        method: 'POST',
        headers: { ...headers, 'Content-Type': 'application/json' },
        body: JSON.stringify({
          vinculo_id: vinculo.id,
          data_inicio: form.data_inicio,
          data_fim: form.data_fim,
          faixas: form.faixas
            .filter(f => f.valor_meta !== '')
            .map(f => ({ faixa: f.faixa, valor_meta: Number(f.valor_meta) })),
        }),
      })
      if (!r.ok) throw new Error((await r.text()) || 'Erro ao criar vigência')
    },
    onSuccess: () => {
      toast.success('Vigência criada')
      qc.invalidateQueries({ queryKey: ['farol-metas-vigencias', vinculo.id] })
      setForm(EMPTY_VIGENCIA_FORM)
      setShowForm(false)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const fechar = useMutation({
    mutationFn: async (id: number) => {
      const r = await fetch(`/api/farol/metas-vigencias/${id}/fechar`, { method: 'POST', headers })
      if (!r.ok) throw new Error((await r.text()) || 'Erro ao fechar vigência')
    },
    onSuccess: () => {
      toast.success('Vigência fechada — congelada, só reprocessamento manual altera o resultado dela')
      qc.invalidateQueries({ queryKey: ['farol-metas-vigencias', vinculo.id] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  function addFaixaRow() {
    const proximaFaixa = Math.max(0, ...form.faixas.map(f => f.faixa)) + 1
    setForm(f => ({ ...f, faixas: [...f.faixas, { faixa: proximaFaixa, valor_meta: '' }] }))
  }

  function removeFaixaRow(idx: number) {
    setForm(f => ({ ...f, faixas: f.faixas.filter((_, i) => i !== idx) }))
  }

  return (
    <Dialog open onOpenChange={v => !v && onClose()}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Vigências — {vinculo.industria_nome} / {vinculo.tipo_metrica_nome}</DialogTitle>
        </DialogHeader>

        <div className="space-y-3">
          {isLoading && <p className="text-sm text-muted-foreground">Carregando...</p>}
          {!isLoading && vigencias.length === 0 && (
            <p className="text-sm text-muted-foreground">Nenhuma vigência cadastrada ainda.</p>
          )}
          {vigencias.map(v => (
            <div key={v.id} className="border rounded-lg p-3 flex items-start justify-between gap-3">
              <div>
                <div className="flex items-center gap-2">
                  <span className="font-medium text-sm">{v.data_inicio} — {v.data_fim}</span>
                  <Badge variant={v.status === 'aberta' ? 'default' : 'secondary'}>
                    {v.status === 'aberta' ? 'Aberta' : 'Fechada'}
                  </Badge>
                </div>
                <div className="flex flex-wrap gap-1 mt-1">
                  {v.faixas.map(f => (
                    <span key={f.faixa} className="font-mono text-xs bg-slate-100 px-1.5 py-0.5 rounded">
                      Faixa {f.faixa}: {f.valor_meta}
                    </span>
                  ))}
                </div>
              </div>
              {v.status === 'aberta' && (
                <div className="flex gap-1.5">
                  <Button
                    variant="outline" size="sm"
                    disabled={importarClientes.isPending}
                    onClick={() => { setClientesUploadTarget(v.id); clientesFileInputRef.current?.click() }}
                    title="Importar Clientes Válidos — aceita .xlsx direto do modelo da JC (aba com 'LOJA' no nome) ou CSV: cnpj;cod_princ;razao;fantasia;cod_ggv;nome_ggv;cod_crv;nome_crv;cod_rca;nome_rca (obrigatórios: cnpj, cod_princ, cod_ggv, cod_crv, cod_rca)"
                  >
                    <Upload className="w-3.5 h-3.5 mr-1" /> Clientes
                  </Button>
                  <Button
                    variant="outline" size="sm"
                    disabled={importarItens.isPending}
                    onClick={() => { setItensUploadTarget(v.id); itensFileInputRef.current?.click() }}
                    title="Importar Itens Válidos (CSV): ean;cod_prod (embalagem/qt_unit_cx vêm do cadastro de produto, não deste arquivo)"
                  >
                    <Upload className="w-3.5 h-3.5 mr-1" /> Itens
                  </Button>
                  <Button
                    variant="outline" size="sm"
                    disabled={fechar.isPending}
                    onClick={() => fechar.mutate(v.id)}
                  >
                    <Lock className="w-3.5 h-3.5 mr-1" /> Fechar
                  </Button>
                </div>
              )}
            </div>
          ))}
          <input ref={clientesFileInputRef} type="file" accept=".csv,.xlsx,.xls" className="hidden" onChange={onClientesCSVSelected} />
          <input ref={itensFileInputRef} type="file" accept=".csv" className="hidden" onChange={onItensCSVSelected} />

          {!showForm && (
            <Button variant="outline" size="sm" onClick={() => setShowForm(true)}>
              <Plus className="w-4 h-4 mr-1" /> Nova Vigência
            </Button>
          )}

          {showForm && (
            <div className="border rounded-lg p-3 space-y-3">
              <div className="flex gap-3">
                <div className="space-y-1 flex-1">
                  <Label className="text-xs">Início</Label>
                  <Input type="date" value={form.data_inicio} onChange={e => setForm(f => ({ ...f, data_inicio: e.target.value }))} />
                </div>
                <div className="space-y-1 flex-1">
                  <Label className="text-xs">Fim</Label>
                  <Input type="date" value={form.data_fim} onChange={e => setForm(f => ({ ...f, data_fim: e.target.value }))} />
                </div>
              </div>
              <div className="space-y-1.5">
                <Label className="text-xs">Faixas de meta</Label>
                {form.faixas.map((row, idx) => (
                  <div key={idx} className="flex gap-2 items-center">
                    <Input
                      className="w-20"
                      type="number"
                      value={row.faixa}
                      onChange={e => setForm(f => ({
                        ...f,
                        faixas: f.faixas.map((r, i) => i === idx ? { ...r, faixa: Number(e.target.value) } : r),
                      }))}
                      placeholder="Faixa"
                    />
                    <Input
                      value={row.valor_meta}
                      type="number"
                      onChange={e => setForm(f => ({
                        ...f,
                        faixas: f.faixas.map((r, i) => i === idx ? { ...r, valor_meta: e.target.value } : r),
                      }))}
                      placeholder="Valor da meta"
                    />
                    <Button variant="ghost" size="icon" disabled={form.faixas.length === 1} onClick={() => removeFaixaRow(idx)}>
                      <X className="w-4 h-4" />
                    </Button>
                  </div>
                ))}
                <Button variant="outline" size="sm" onClick={addFaixaRow}>
                  <Plus className="w-4 h-4 mr-1" /> Adicionar faixa
                </Button>
              </div>
              <div className="flex justify-end gap-2">
                <Button variant="outline" onClick={() => { setShowForm(false); setForm(EMPTY_VIGENCIA_FORM) }}>Cancelar</Button>
                <Button
                  onClick={() => criar.mutate()}
                  disabled={!form.data_inicio || !form.data_fim || criar.isPending}
                >
                  {criar.isPending ? 'Salvando...' : 'Salvar Vigência'}
                </Button>
              </div>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>Fechar</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
