// AssistenteChart — gráfico automático sobre o resultado do Assistente.
//
// A tabela continua abaixo e é a fonte precisa: o gráfico mostra a forma, a
// tabela mostra o número. Essa divisão também é o que cobre o aviso de
// contraste da paleta (cores claras exigem rótulo visível ou tabela).
//
// Paleta: slots 1–4 do tema categórico de referência, validados para superfície
// clara — CVD ΔE 9,1 no pior par adjacente, visão normal 22,9.
// Nunca há dois eixos Y. Duas medidas de escalas diferentes viram dois
// gráficos (o seletor troca qual está em tela), jamais um eixo secundário.

import {
  BarChart, Bar, LineChart, Line,
  XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer, LabelList,
} from 'recharts'
import { fmtCell, fmtEixo, classifyCol, parseNum, type ChartSpec } from '@/lib/farolAiFormat'

const SERIE = ['#2a78d6', '#eb6834', '#1baf7a', '#eda100']

const EIXO   = '#64748b'   // slate-500 — recessivo, não compete com a marca
const GRADE  = '#e2e8f0'   // slate-200

const MAX_BARRAS = 15      // além disso o eixo vira lista ilegível
const MAX_LABEL  = 28

function encurta(s: string) {
  return s.length > MAX_LABEL ? s.slice(0, MAX_LABEL - 1) + '…' : s
}

export default function AssistenteChart({
  spec, rows, medida, onMedida,
}: {
  spec: ChartSpec
  rows: Record<string, unknown>[]
  medida: string
  onMedida: (m: string) => void
}) {
  const kind = classifyCol(medida)

  // Série temporal preserva a ordem cronológica; ranking corta no topo N.
  const dados = (spec.forma === 'linha' ? rows : rows.slice(0, MAX_BARRAS)).map(r => {
    const p: Record<string, unknown> = {
      __label: spec.forma === 'linha' && spec.periodo?.ano
        ? `${String(r[spec.periodo.ano] ?? '')}/${String(r[spec.labelCol] ?? '').padStart(2, '0')}`
        : String(r[spec.labelCol] ?? ''),
    }
    for (const m of spec.medidas) p[m] = parseNum(r[m]) ?? 0
    return p
  })

  const cortou = spec.forma === 'barra' && rows.length > MAX_BARRAS
  // Rótulo direto só quando há poucas barras — número em toda barra polui.
  const rotularDireto = spec.forma === 'barra' && dados.length <= 8

  const tooltip = (
    <Tooltip
      cursor={{ fill: 'rgba(100,116,139,0.06)' }}
      contentStyle={{
        borderRadius: 8, border: '1px solid #e2e8f0', fontSize: 12,
        boxShadow: '0 4px 12px rgba(15,23,42,0.08)',
      }}
      labelStyle={{ color: '#334155', fontWeight: 600, marginBottom: 4 }}
      /* Tipos inferidos do contexto: no recharts 3 o value chega como
         number | undefined, e anotar `number` à mão quebra a atribuição. */
      formatter={(v, name) => [fmtCell(v, String(name)), String(name)]}
    />
  )

  return (
    <div className="px-4 pt-4 pb-2">
      {spec.medidas.length > 1 && (
        <div className="flex flex-wrap items-center gap-1.5 mb-3">
          <span className="text-[11px] uppercase tracking-wider text-slate-400 font-semibold mr-1">
            Medida
          </span>
          {spec.medidas.map(m => (
            <button
              key={m}
              onClick={() => onMedida(m)}
              className={`px-2.5 py-1 rounded-md text-xs font-medium transition-colors ${
                m === medida
                  ? 'bg-slate-800 text-white'
                  : 'bg-slate-100 text-slate-600 hover:bg-slate-200'
              }`}
            >
              {m}
            </button>
          ))}
        </div>
      )}

      <ResponsiveContainer width="100%" height={Math.max(240, dados.length * 30 + 60)}>
        {spec.forma === 'barra' ? (
          <BarChart data={dados} layout="vertical" margin={{ top: 4, right: rotularDireto ? 76 : 16, bottom: 4, left: 4 }}>
            <CartesianGrid horizontal={false} stroke={GRADE} />
            <XAxis
              type="number" tickFormatter={(v: number) => fmtEixo(v, kind)}
              tick={{ fontSize: 11, fill: EIXO }} axisLine={false} tickLine={false}
            />
            <YAxis
              type="category" dataKey="__label" width={170}
              tick={{ fontSize: 11, fill: EIXO }} axisLine={false} tickLine={false}
              tickFormatter={encurta}
            />
            {tooltip}
            <Bar dataKey={medida} fill={SERIE[0]} radius={[0, 4, 4, 0]} maxBarSize={22}>
              {rotularDireto && (
                <LabelList
                  dataKey={medida} position="right"
                  formatter={v => fmtEixo(Number(v), kind)}
                  style={{ fontSize: 11, fill: EIXO, fontWeight: 600 }}
                />
              )}
            </Bar>
          </BarChart>
        ) : (
          <LineChart data={dados} margin={{ top: 8, right: 16, bottom: 4, left: 4 }}>
            <CartesianGrid vertical={false} stroke={GRADE} />
            <XAxis dataKey="__label" tick={{ fontSize: 11, fill: EIXO }} axisLine={false} tickLine={false} />
            <YAxis
              tickFormatter={(v: number) => fmtEixo(v, kind)}
              tick={{ fontSize: 11, fill: EIXO }} axisLine={false} tickLine={false}
            />
            {tooltip}
            {/* Legenda só a partir de 2 séries — com uma, o título já a nomeia. */}
            {spec.medidas.length > 1 && <Legend wrapperStyle={{ fontSize: 11 }} />}
            {(spec.medidas.length > 1 ? spec.medidas : [medida]).map((m, i) => (
              <Line
                key={m} type="monotone" dataKey={m}
                stroke={SERIE[i % SERIE.length]} strokeWidth={2}
                dot={{ r: 3 }} activeDot={{ r: 5, strokeWidth: 2, stroke: '#fff' }}
              />
            ))}
          </LineChart>
        )}
      </ResponsiveContainer>

      {cortou && (
        <p className="text-[11px] text-slate-400 mt-1">
          Gráfico mostra os {MAX_BARRAS} primeiros de {rows.length}. A tabela abaixo traz todos.
        </p>
      )}
    </div>
  )
}
