// exportToPdf — PDF de uma tabela do painel (pedido do Heverton, 06/11/2026):
// sai o que está na tela, na aba aberta, com os mesmos filtros. A biblioteca
// só é baixada no clique (import dinâmico), pra não pesar o painel.

export interface ColunaPdf {
  header: string
  align?: 'left' | 'center' | 'right'
  // largura relativa; sem isso a coluna divide o que sobra
  largura?: number
}

// Célula: texto puro ou texto com cor (verde = atingiu, vermelho = abaixo).
export type CelulaPdf = string | number | { texto: string | number; cor?: 'verde' | 'vermelho'; negrito?: boolean }

export interface TabelaPdf {
  titulo: string
  // linhas de contexto abaixo do título (indústria, período, filtros...)
  contexto: string[]
  // resumo em destaque (ex.: "Cobertura: 76 / 134 redes"), uma linha cada
  resumo?: string[]
  colunas: ColunaPdf[]
  linhas: CelulaPdf[][]
  // linha final em negrito (TOTAL)
  total?: CelulaPdf[]
  arquivo: string // sem extensão
  // token do usuário: busca a logo da empresa (a mesma do menu lateral)
  token?: string | null
}

interface LogoPdf { dataUrl: string; w: number; h: number; fmt: 'PNG' | 'JPEG' }

async function carregarLogo(token?: string | null): Promise<LogoPdf | null> {
  const tentar = async (url: string, auth: boolean): Promise<LogoPdf | null> => {
    try {
      const res = await fetch(url, { headers: auth && token ? { Authorization: `Bearer ${token}` } : undefined, cache: 'no-store' })
      if (!res.ok) return null
      const blob = await res.blob()
      if (!blob.type.startsWith('image/') || blob.type.includes('svg')) return null
      const dataUrl = await new Promise<string>((ok, err) => {
        const r = new FileReader()
        r.onload = () => ok(String(r.result))
        r.onerror = () => err(r.error)
        r.readAsDataURL(blob)
      })
      const dim = await new Promise<{ w: number; h: number }>((ok, err) => {
        const im = new Image()
        im.onload = () => ok({ w: im.naturalWidth, h: im.naturalHeight })
        im.onerror = () => err(new Error('img'))
        im.src = dataUrl
      })
      if (!dim.w || !dim.h) return null
      return { dataUrl, ...dim, fmt: blob.type.includes('jpeg') || blob.type.includes('jpg') ? 'JPEG' : 'PNG' }
    } catch {
      return null
    }
  }
  return (token ? await tentar('/api/config/empresa/logo', true) : null) ?? (await tentar('/logo-fb.png', false))
}

const VERDE: [number, number, number] = [21, 128, 61]
const VERMELHO: [number, number, number] = [185, 28, 28]

export async function exportToPdf(t: TabelaPdf) {
  const [{ jsPDF }, autoTableMod] = await Promise.all([import('jspdf'), import('jspdf-autotable')])
  const autoTable = autoTableMod.default
  const doc = new jsPDF({ orientation: 'landscape', unit: 'pt', format: 'a4' })
  const margem = 28
  let y = 34

  // Logo no canto superior direito (máx. 46pt de altura, 130pt de largura)
  const logo = await carregarLogo(t.token)
  if (logo) {
    const esc = Math.min(46 / logo.h, 130 / logo.w)
    const w = logo.w * esc
    const h = logo.h * esc
    try {
      doc.addImage(logo.dataUrl, logo.fmt, doc.internal.pageSize.getWidth() - margem - w, 22, w, h)
    } catch { /* logo inválida: segue sem */ }
  }

  doc.setFont('helvetica', 'bold')
  doc.setFontSize(14)
  doc.text(t.titulo, margem, y)
  y += 16
  doc.setFont('helvetica', 'normal')
  doc.setFontSize(8.5)
  doc.setTextColor(80)
  for (const l of t.contexto) {
    doc.text(l, margem, y)
    y += 11
  }
  if (t.resumo?.length) {
    y += 2
    doc.setTextColor(0)
    doc.setFont('helvetica', 'bold')
    doc.setFontSize(9.5)
    for (const l of t.resumo) {
      doc.text(l, margem, y)
      y += 12
    }
  }
  doc.setTextColor(0)

  const celula = (c: CelulaPdf) => {
    if (typeof c === 'object') {
      return {
        content: String(c.texto),
        styles: { textColor: c.cor === 'verde' ? VERDE : c.cor === 'vermelho' ? VERMELHO : 0, fontStyle: c.negrito ? 'bold' as const : 'normal' as const },
      }
    }
    return { content: String(c) }
  }

  const columnStyles: Record<number, { halign: 'left' | 'center' | 'right'; cellWidth?: number }> = {}
  t.colunas.forEach((c, i) => {
    columnStyles[i] = { halign: c.align ?? 'left', ...(c.largura ? { cellWidth: c.largura } : {}) }
  })

  autoTable(doc, {
    startY: y + 4,
    margin: { left: margem, right: margem, bottom: 28 },
    head: [t.colunas.map(c => ({ content: c.header, styles: { halign: c.align ?? 'left' as const } }))],
    body: [
      ...t.linhas.map(l => l.map(celula)),
      ...(t.total ? [t.total.map(c => {
        const x = celula(c)
        return { ...x, styles: { ...(x.styles ?? {}), fontStyle: 'bold' as const, fillColor: [241, 245, 249] as [number, number, number] } }
      })] : []),
    ],
    columnStyles,
    styles: { font: 'helvetica', fontSize: 7, cellPadding: 3, overflow: 'linebreak', lineColor: [226, 232, 240], lineWidth: 0.3 },
    headStyles: { fillColor: [51, 65, 85], textColor: 255, fontSize: 7, fontStyle: 'bold' },
    alternateRowStyles: { fillColor: [250, 250, 252] },
    showHead: 'everyPage',
  })

  const total = doc.getNumberOfPages()
  for (let i = 1; i <= total; i++) {
    doc.setPage(i)
    doc.setFontSize(7)
    doc.setTextColor(120)
    doc.text('FB Farol', margem, doc.internal.pageSize.getHeight() - 14)
    doc.text(`Página ${i} de ${total}`, doc.internal.pageSize.getWidth() - margem, doc.internal.pageSize.getHeight() - 14, { align: 'right' })
  }

  doc.save(`${t.arquivo}.pdf`)
}
