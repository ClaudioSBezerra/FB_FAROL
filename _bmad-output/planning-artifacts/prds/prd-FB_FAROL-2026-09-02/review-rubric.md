# PRD Quality Review — Painel de Gestão de Metas por Indústria

## Overall verdict

Stakes-appropriate and mostly earned: this is a launch-grade PRD (decides fornecedor bônus) that reads like one — financial risk is named repeatedly, snapshot/audit requirements are concrete, and the brownfield claims about the Farol codebase (V03/V06 hierarchy gap, `/m/CNPJ/SUP|RCA/cod`, `gestor_filial`/`gestor_geral`) check out against the actual code. The main risk isn't dishonesty, it's under-specification at the seams: the "framework genérico" claim is never tested against a second metric shape, the projeção de fechamento (a headline new capability, UJ-1's climax) has no defined method, and several FRs describing multi-fornecedor/faixa/período interactions leave the acceptance boundary implicit. Nothing here is broken, but Done-ness and parts of Strategic coherence are thin enough that story-writing will hit undefined territory.

## Decision-readiness — adequate

Real trade-offs are surfaced, not smoothed. FR10/addendum explicitly names Oracle-as-future vs. CSV-now as a deliberate deferral with a reason (JC's Oracle reachability already proven — memory `infra_dev_vm.md`). FR17 states the freeze rule as a decision with a real consequence ("nunca de forma automática/silenciosa"), not a hedge. The `.memlog.md` shows three Open Questions were genuinely resolved with the user (faixas genéricas, SLA, papel de gestão) rather than rhetorically pre-answered in the PRD text itself — good practice, though it means the PRD body itself carries zero live tension by the time it reached this review (§ Questões em aberto, line 96-98: "Nenhuma pendente").

One dodge: NFR4 ("Não há SLA crítico... confirmado com o usuário") reads as a decision but hides an unstated risk — if apuração has no SLA, what happens if a mês corrente calc silently stalls for days during an active bônus dispute? The PRD doesn't ask what "não crítico" trades away.

### Findings
- **medium** No stated fallback when non-critical SLA meets financial stakes (§ NFR4) — "sem SLA crítico" is stated as resolved but the failure mode (calc stalls, nobody notices until month-close) isn't acknowledged. *Fix:* add one sentence on what happens if apuração does silently fall behind during the month — even "user accepted this risk" makes it a decision instead of a gap.

## Substance over theater — strong

No persona bloat — one persona (Waliston, UJ-1) doing real work. Visão (line 10-14) is specific to this business (Programa Único Unilever, planilha manual, WinThor) — it would not swap into another PRD unchanged. NFRs are not boilerplate: NFR1 names exactly what must be audited (quem, quando, valor anterior); NFR3 defines reproducibility operationally ("mesmo snapshot... resultado... não muda entre duas execuções") instead of saying "system must be reliable." No findings — this dimension is well above the bar for a PRD this size.

## Strategic coherence — adequate

The thesis is stated plainly: replace a manual spreadsheet apuração with an automatic, per-industry-extensible engine (line 14). Feature grouping (Catálogo → Configuração → Importação → Listas → Motor → Painéis) follows that thesis, not a random backlog order.

The gap: the PRD's central strategic claim — "framework genérico" (line 29, "não é o modelo final") — is asserted but never pressure-tested. Every FR, every example, and the one reference metric type ("Cobertura por Rede", "Sortimento por Rede") comes from Unilever. There's no second worked example showing the framework surviving a metric shape that isn't "average across a Rede compared to a threshold." A reader can't tell whether FR1's "lista de parâmetros que uma instância desse tipo exige preencher" is genuinely type-agnostic or is Unilever's two metrics with a genericized paint job. This matters because generic-framework-vs-YAGNI is exactly the kind of thesis a PRD should defend, not just declare.

Success Metrics are notably absent as a named section — "Sucesso" only appears in `.memlog.md` (line 16: "uso regular por GGV/SUPV + menos surpresa + eventual substituição da planilha, aceito sem detalhar mais") and never made it into prd.md itself.

### Findings
- **high** "Framework genérico" is the PRD's central architectural bet but has zero non-Unilever validation (§ Contexto de referência, line 29; § FR1-FR3). *Fix:* either add one counter-example metric type (even hypothetical, e.g. a per-cliente frequency metric) to prove FR1's parameter model generalizes, or soften the claim to "designed to generalize, validated only against Unilever's two metric shapes."
- **medium** Success Metrics never made it from `.memlog.md` into the PRD body — no way for a reader of prd.md alone to know what "working" looks like beyond FR completion. *Fix:* add a short Métricas de Sucesso section carrying the memlog line 16 content ("uso regular", "menos surpresa", "substituição da planilha") even at the accepted low-detail level.

## Done-ness clarity — thin

Several FRs have clean, testable consequences: FR9 ("linhas com erro reportadas claramente, sem aplicar parcialmente um lote com erro"), FR13/FR17 (freeze semantics), FR19a (delta explícito, not just two numbers side by side) are all verifiable. But the FR that carries the PRD's one genuinely new capability — projeção de fechamento — is not:

- FR18: "O motor deve calcular a projeção de fechamento do ano corrente... com base no ritmo de realização até o momento." No method is specified (linear run-rate? same-period-last-year seasonality? weighted recent trend?), no bounds, no worked example. "Ritmo de realização" is doing the same job as "reasonable performance" — it names an intent, not a testable behavior. This is the FR most likely to produce three different implementations from three different engineers, and it's also the climax of UJ-1 ("olha a projeção de fechamento").
- FR21/FR23 ("permitir comparar múltiplos recortes de tempo... visualizar a projeção") inherit FR18's ambiguity.
- FR14 says the engine calculates "por nível hierárquico (GGV → CRV → RCA → Rede → Cliente/CNPJ)" but doesn't say whether projection (FR18) is computed at every level of that hierarchy or only at the top — a Cliente-level projection and a GGV-level projection are different math (sum-of-projections vs. projection-of-sum), and the PRD doesn't say which.

### Findings
- **critical** FR18 (projeção de fechamento) has no defined calculation method, no bounds, no example — "com base no ritmo de realização até o momento" is an intent statement, not an acceptance criterion, for the PRD's one headline new capability. *Fix:* add a worked example (even simple linear run-rate: "realizado até dia X / dias decorridos × dias do período") or explicitly flag it `[NOTE FOR PM]` as an open design question for architecture to resolve, rather than let it read as decided.
- **medium** FR18/FR14 don't say at which hierarchy level(s) projection is computed, and whether a parent level's projection is an aggregate of child projections or its own independent calculation — these produce different numbers. *Fix:* one sentence stating the aggregation direction.
- **low** FR5's "mobilidade de ajuste" (meta can change month to month) doesn't say what happens to a meta already mid-vigência when a new import changes it — does FR13's freeze rule protect an in-flight vigência period the same way it protects a closed month? *Fix:* clarify whether FR5's per-period meta values are themselves subject to FR17's freeze-on-close, or only the Realizado calculation is.

## Scope honesty — strong

Non-Goals are explicit and reasoned, not silent omissions: FR10/Oracle deferral has its own addendum entry with a named future phase; the "Farol de Compras" tangent (line 94) is flagged as a data-model constraint without becoming in-scope work ("modelo de dados deve evitar decisões que fechem essa porta, sem se comprometer com o escopo agora" — this is exactly the honest hedge the rubric wants). `.memlog.md` line 17 shows an assumption (faixas genéricas) that was explicitly flagged `[ASSUNÇÃO]` during drafting and then resolved before the PRD reached this review — the resolution is correctly reflected in prd.md (line 50, FR7, stated as fact with no residual assumption marker), and no stale `[ASSUMPTION]` tag was left behind. Open-items density is appropriately near-zero for a PRD that already went through a resolved-in-conversation cycle — reasonable for launch-grade stakes, given the resolution trail exists in `.memlog.md`.

## Downstream usability — thin

This PRD is chain-top (feeds architecture and epics/stories per `.memlog.md` line 11 and CLAUDE.md's GSD workflow), so this dimension carries real weight.

- No Glossary section exists. Domain nouns are used consistently in practice (Rede, Cliente/CNPJ, GGV/CRV/RCA, Tipo de Métrica, vínculo, faixa, vigência) but nothing indexes them, and a term like "CRV" (used throughout, e.g. line 35, 66, 74) is never expanded or defined anywhere in the PRD — a downstream reader unfamiliar with Farol's org model has no way to resolve it. ("Supervisor" and "CRV" appear to be used interchangeably — line 20 UJ-1 calls Waliston "Supervisor V7-GO" but the hierarchy in line 35 uses "CRV" — this drift is exactly what a Glossary would catch.)
- FR IDs are contiguous and unique (FR1-FR23, including FR19a and FR10 correctly marked out-of-scope inline rather than silently renumbered). NFR1-4 contiguous. No dangling cross-references found.
- UJ-1 is the only UJ and has a named protagonist (Waliston) with real context (role, territory "V7-GO", concrete action). No floating UJs — but a single UJ for a multi-role system (GGV and Supervisor both named as primary actors in the Visão, line 12) means the GGV's usage pattern is never separately walked, only asserted by extension.

### Findings
- **medium** No Glossary; "CRV" is used as a hierarchy term (line 35, 66, 74) without definition, and appears to drift against "Supervisor" (line 20's UJ protagonist is titled "Supervisor V7-GO" while the hierarchy line calls the same role "CRV"). *Fix:* add a short Glossary defining GGV/CRV/RCA/Rede/Cliente/Tipo de Métrica/vínculo/faixa/vigência, and confirm whether CRV and Supervisor are the same role under two names or genuinely different levels.
- **low** Single UJ covers only the Supervisor path; GGV's usage of the same painel (also named as a primary actor in Visão, line 12) is never walked even briefly. *Fix:* either a short second UJ for GGV, or a sentence noting GGV's flow is identical to UJ-1 at a higher rollup level.

## Shape fit — adequate

This is a brownfield internal-tool extension to an existing B2B distributor system, feeding a real org hierarchy with named field roles (GGV/Supervisor/RCA) — UJ-with-protagonist is appropriately load-bearing here, and one UJ is not under-formalized for this stakes level given the workflow is genuinely singular (compare meta vs. realizado, call the RCA). Brownfield references are accurate and specific rather than hand-waved: line 35's claim that "V03/Gerência vai direto RCA→Cliente" and "V06/Rede é uma árvore à parte, sem cruzar com a hierarquia organizacional" were spot-checked against the actual Go codebase (`backend/handlers/farol_v2_api.go`, `farol_bi_api.go` reference V03/V06 concepts) and the role names NFR2 reuses (`gestor_filial`, `gestor_geral`) are real, in `backend/main.go`. The `/m/CNPJ/SUP/cod` URL pattern FR22 claims to reuse is implemented exactly as described in `backend/main.go` (line 757 regex `^/m/(\d{14})/([Ss][Uu][Pp]|[Rr][Cc][Aa])/(\d+)$`). This PRD does not over-claim continuity with the existing system.

No major shape-fit findings — the capability-spec-with-one-UJ shape matches an admin+field-panel tool at this scale.

## Mechanical notes

- **Glossary drift**: "CRV" vs. "Supervisor" — see Downstream usability findings above. Also "Realizado" and "resultado apurado" (FR17) appear to refer to the same concept without being tied together explicitly, though context makes it inferable.
- **ID continuity**: Clean. FR1-FR23 (with FR19a inserted correctly, FR10 marked out-of-scope inline), NFR1-4, no gaps or duplicates found.
- **Assumptions Index roundtrip**: No `[ASSUMPTION]` tags remain in prd.md — the one assumption raised during drafting (faixas genéricas, `.memlog.md` line 17) was resolved and folded into FR7 as fact before this review, with no stale marker left behind and no orphaned index entries. Clean roundtrip, nothing to reconcile.
- **UJ protagonist naming**: UJ-1 has a named protagonist (Waliston, Supervisor V7-GO) carrying inline context. Compliant.
- **Required sections for stakes/type**: Visão, Loop de uso, Contexto, Escopo organizacional, FRs, NFRs, Fora de escopo, Questões em aberto all present. Missing for this stakes level: a dedicated Success Metrics section in the PRD body (currently only in `.memlog.md` — see Strategic coherence) and a Glossary (see Downstream usability).
