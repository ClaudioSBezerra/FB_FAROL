import { useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import { Upload } from 'lucide-react'
import { toast } from 'sonner'

// ─── Tipos de período ────────────────────────────────────────────────────────

type TipoPeriodo = 'MENSAL' | 'TRIMESTRAL' | 'SEMESTRAL' | 'ANUAL'

const TIPOS: { value: TipoPeriodo; label: string }[] = [
  { value: 'MENSAL',      label: 'Mensal'      },
  { value: 'TRIMESTRAL',  label: 'Trimestral'  },
  { value: 'SEMESTRAL',   label: 'Semestral'   },
  { value: 'ANUAL',       label: 'Anual'       },
]

const MESES = [
  'Janeiro','Fevereiro','Março','Abril','Maio','Junho',
  'Julho','Agosto','Setembro','Outubro','Novembro','Dezembro',
]

const TRIMESTRES = ['T1 (Jan–Mar)', 'T2 (Abr–Jun)', 'T3 (Jul–Set)', 'T4 (Out–Dez)']
const SEMESTRES  = ['S1 (Jan–Jun)', 'S2 (Jul–Dez)']

// ─── Componente ──────────────────────────────────────────────────────────────

export default function ObjetivosImportar() {
  const fileRef = useRef<HTMLInputElement>(null)

  const [tipo,       setTipo]       = useState<TipoPeriodo>('MENSAL')
  const [ano,        setAno]        = useState<string>(String(new Date().getFullYear()))
  const [periodoSeq, setPeriodoSeq] = useState<string>('1')
  const [uploading,  setUploading]  = useState(false)
  const [result,     setResult]     = useState<{ importados: number; atualizados: number; ignorados: number } | null>(null)

  // Ao mudar o tipo, reseta periodo_seq para 1
  function handleTipoChange(v: TipoPeriodo) {
    setTipo(v)
    setPeriodoSeq('1')
  }

  async function handleUpload(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!fileRef.current) return
    fileRef.current.value = ''
    if (!file) return

    const anoNum = parseInt(ano, 10)
    if (!ano || isNaN(anoNum) || anoNum < 2000 || anoNum > 2100) {
      toast.error('Informe um ano válido (ex: 2025)')
      return
    }

    setUploading(true)
    setResult(null)
    try {
      const form = new FormData()
      form.append('file', file)

      const params = new URLSearchParams({
        tipo_periodo: tipo,
        ano:          String(anoNum),
        periodo_seq:  periodoSeq,
      })

      const res = await fetch(`/api/objetivos/upload-csv?${params}`, {
        method: 'POST',
        body:   form,
      })

      const data = await res.json()
      if (!res.ok) {
        toast.error(data.error ?? 'Erro na importação')
        return
      }
      setResult(data)
      toast.success('Importação concluída')
    } catch {
      toast.error('Erro de conexão')
    } finally {
      setUploading(false)
    }
  }

  // Label descritivo do período selecionado
  function periodoLabel(): string {
    const seq = parseInt(periodoSeq, 10)
    if (tipo === 'MENSAL')     return MESES[seq - 1] ?? ''
    if (tipo === 'TRIMESTRAL') return TRIMESTRES[seq - 1] ?? ''
    if (tipo === 'SEMESTRAL')  return SEMESTRES[seq - 1] ?? ''
    return ''
  }

  return (
    <div className="p-6 space-y-6 max-w-xl">
      <div>
        <h1 className="text-xl font-semibold">Importar Objetivos de Vendas</h1>
        <p className="text-sm text-muted-foreground mt-1">
          CSV separado por <code>;</code> com cabeçalho:{' '}
          <span className="font-mono text-xs">
            CODSUPERVISOR; CODUSUR; CODEPTO; DEPARTAMENTO; CODSEC; SECAO; CODFORNEC; FORNECEDOR; CODPROD; CODCLI; VL_ANTERIOR; VL_CORRENTE
          </span>
        </p>
      </div>

      <div className="border rounded-lg p-5 space-y-5 bg-white">
        {/* Tipo de período */}
        <div className="space-y-1.5">
          <Label>Tipo de objetivo</Label>
          <Select value={tipo} onValueChange={v => handleTipoChange(v as TipoPeriodo)}>
            <SelectTrigger className="w-48">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {TIPOS.map(t => (
                <SelectItem key={t.value} value={t.value}>{t.label}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        {/* Seletor de período (dinâmico conforme tipo) */}
        <div className="flex gap-3 items-end flex-wrap">
          {tipo !== 'ANUAL' && (
            <div className="space-y-1.5">
              <Label>
                {tipo === 'MENSAL' && 'Mês'}
                {tipo === 'TRIMESTRAL' && 'Trimestre'}
                {tipo === 'SEMESTRAL' && 'Semestre'}
              </Label>
              <Select value={periodoSeq} onValueChange={setPeriodoSeq}>
                <SelectTrigger className="w-44">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {tipo === 'MENSAL' && MESES.map((m, i) => (
                    <SelectItem key={i + 1} value={String(i + 1)}>{m}</SelectItem>
                  ))}
                  {tipo === 'TRIMESTRAL' && TRIMESTRES.map((t, i) => (
                    <SelectItem key={i + 1} value={String(i + 1)}>{t}</SelectItem>
                  ))}
                  {tipo === 'SEMESTRAL' && SEMESTRES.map((s, i) => (
                    <SelectItem key={i + 1} value={String(i + 1)}>{s}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}

          <div className="space-y-1.5">
            <Label>Ano</Label>
            <Input
              className="w-24"
              type="number"
              min={2000}
              max={2100}
              value={ano}
              onChange={e => setAno(e.target.value)}
            />
          </div>
        </div>

        {/* Resumo do período selecionado */}
        <p className="text-sm text-muted-foreground">
          Período: <strong>
            {tipo === 'ANUAL' ? `Ano ${ano}` : `${periodoLabel()} / ${ano}`}
          </strong>
        </p>

        {/* Upload */}
        <div className="pt-1">
          <input
            ref={fileRef}
            type="file"
            accept=".csv,.txt"
            className="hidden"
            onChange={handleUpload}
          />
          <Button
            onClick={() => fileRef.current?.click()}
            disabled={uploading || !ano}
            className="w-full"
          >
            <Upload className="w-4 h-4 mr-2" />
            {uploading ? 'Importando...' : 'Selecionar arquivo e importar'}
          </Button>
        </div>
      </div>

      {/* Resultado */}
      {result && (
        <div className="flex gap-6 p-4 bg-green-50 border border-green-200 rounded-lg text-sm">
          <span>✅ <strong>{result.importados}</strong> novos</span>
          <span>🔄 <strong>{result.atualizados}</strong> atualizados</span>
          {result.ignorados > 0 && (
            <span>⚠️ <strong>{result.ignorados}</strong> ignorados</span>
          )}
          <button
            className="ml-auto text-muted-foreground hover:text-foreground"
            onClick={() => setResult(null)}
          >✕</button>
        </div>
      )}
    </div>
  )
}
