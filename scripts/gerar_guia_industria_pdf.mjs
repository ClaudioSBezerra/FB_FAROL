// Gera o PDF do guia "Indústria" a partir de frontend/src/content/guiaIndustria.ts.
// Uso: node scripts/gerar_guia_industria_pdf.mjs <saida.pdf>
import { build } from '../frontend/node_modules/esbuild/lib/main.js'
import { writeFileSync, mkdtempSync, readdirSync } from 'node:fs'
import { tmpdir, homedir } from 'node:os'
import { join } from 'node:path'
import { execFileSync } from 'node:child_process'
import { pathToFileURL } from 'node:url'

const saida = process.argv[2] ?? 'guia-industria-farol.pdf'
const dir = mkdtempSync(join(tmpdir(), 'guia-'))
const js = join(dir, 'conteudo.mjs')
await build({ entryPoints: ['frontend/src/content/guiaIndustria.ts'], outfile: js, bundle: true, format: 'esm', logLevel: 'silent' })
const C = await import(pathToFileURL(js).href)
const esc = s => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;')
const cor = { ok: '#047857', parcial: '#b45309', diferente: '#0369a1', pendente: '#be123c' }
const grupos = new Map()
for (const t of C.TOPICOS) grupos.set(t.grupo, [...(grupos.get(t.grupo) ?? []), t])
const cont = s => C.TOPICOS.filter(t => t.status === s).length

const html = `<!doctype html><html lang="pt-BR"><meta charset="utf-8"><style>
@page{size:A4;margin:16mm 14mm}
body{font-family:Helvetica,Arial,sans-serif;color:#1e293b;font-size:10.5pt;line-height:1.4}
h1{font-size:20pt;margin:0 0 4px}h2{font-size:13pt;margin:20px 0 8px;border-bottom:2px solid #7c3aed;padding-bottom:3px;break-after:avoid}
.sub{color:#64748b;margin-bottom:12px}.placar{display:flex;gap:8px;margin:10px 0}
.placar div{flex:1;border:1px solid #e2e8f0;border-radius:6px;padding:6px 8px}.placar b{font-size:16pt;display:block}
.t{border:1px solid #e2e8f0;border-radius:6px;margin:0 0 8px;padding:8px 10px;break-inside:avoid}
.t h3{margin:0 0 4px;font-size:11pt}.tag{font-size:8pt;font-weight:bold;margin-left:6px}
.cols{display:flex;gap:12px}.cols div{flex:1}.l{font-size:7.5pt;text-transform:uppercase;color:#94a3b8;font-weight:bold}
.nota{margin-top:5px;background:#fffbeb;border:1px solid #fde68a;border-radius:4px;padding:3px 7px;font-size:9pt;color:#92400e}
.q{border:1px solid #c4b5fd;border-radius:6px;padding:8px 10px;margin-bottom:8px;break-inside:avoid}
.q h3{margin:0 0 4px;font-size:11pt}.res{margin-top:6px;border-top:1px dashed #cbd5e1;padding-top:4px;color:#94a3b8;font-size:9pt}
.g{font-size:9pt;font-weight:bold;text-transform:uppercase;color:#64748b;margin:12px 0 5px;break-after:avoid}
</style><body>
<h1>${esc(C.INTRO.titulo)}</h1><div class="sub">FB_FAROL · Documento para validação com a JC · Outubro/2026</div>
<p>${esc(C.INTRO.resumo)}</p><p>${esc(C.INTRO.comoLer)}</p>
<div class="placar">${Object.keys(C.STATUS_ROTULO).map(s => `<div style="color:${cor[s]}"><b>${cont(s)}</b>${esc(C.STATUS_ROTULO[s])}</div>`).join('')}</div>
<h2>1. Era para ser × O que ficou</h2>
${[...grupos].map(([g, ts]) => `<div class="g">${esc(g)}</div>` + ts.map(t => `<div class="t"><h3>${esc(t.titulo)}<span class="tag" style="color:${cor[t.status]}">● ${esc(C.STATUS_ROTULO[t.status])}</span><span class="tag" style="color:#64748b">[${esc(C.ESCOPO_ROTULO[t.escopo])}]</span></h3>
<div class="cols"><div><div class="l">Era para ser</div>${esc(t.previsto)}</div><div><div class="l">O que ficou</div>${esc(t.ficou)}</div></div>${t.nota ? `<div class="nota">${esc(t.nota)}</div>` : ''}</div>`).join('')).join('')}
<h2 style="break-before:page">2. Perguntas para decidirmos na reunião (${C.PERGUNTAS.length})</h2>
${C.PERGUNTAS.map(p => `<div class="q"><h3>${p.n}. ${esc(p.titulo)} <span class="tag" style="color:#64748b">[${esc(C.ESCOPO_ROTULO[p.escopo])}]</span></h3>
<div>${esc(p.contexto)}</div><div><b>Hoje:</b> ${esc(p.hoje)}</div><div><b>Precisamos decidir:</b> ${esc(p.decidir)}</div>${p.decisao ? `<div class="nota" style="background:#ecfdf5;border-color:#a7f3d0;color:#047857"><b>Decisão:</b> ${esc(p.decisao)}</div>` : '<div class="res">Decisão: ______________________________________________</div>'}</div>`).join('')}
</body></html>`
const arq = join(dir, 'guia.html'); writeFileSync(arq, html)
const base = join(homedir(), '.cache/ms-playwright')
const sh = readdirSync(base).find(d => d.startsWith('chromium_headless_shell'))
const bin = join(base, sh, 'chrome-headless-shell-linux64', 'chrome-headless-shell')
execFileSync(bin, ['--no-sandbox', '--no-pdf-header-footer', `--print-to-pdf=${saida}`, pathToFileURL(arq).href], { stdio: 'inherit' })
console.log('PDF:', saida)
