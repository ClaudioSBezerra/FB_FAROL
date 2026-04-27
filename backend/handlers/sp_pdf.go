package handlers

// sp_pdf.go — Geração de PDF Operacional de Calibragem
//
// GET /api/sp/pdf/calibracao?job_id=UUID  ou  ?cd_id=INT
//
// Layout:
//   - Página por RUA (subheader "RUA X — N itens", nova página a cada troca de RUA)
//   - Colunas: Curva | Cód. | Produto | Prédio | Apto | Cap.Atual | Nova Cap. | Ação
//   - Linha Obs. em branco após cada produto (anotação manual → Winthor)

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/page"
	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"
)

// ─── DTO interno ──────────────────────────────────────────────────────────────

type pdfProposta struct {
	Codprod         int
	Produto         string
	Rua, Predio, Apto *int
	ClasseVenda     string
	CapacidadeAtual *int
	NovaCapacidade  int
	Delta           int
	Justificativa   string
	AprovadoEm      string
}

// ─── Handler ──────────────────────────────────────────────────────────────────

func SpPDFCalibracaoHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		jobIDStr := r.URL.Query().Get("job_id")
		cdIDStr  := r.URL.Query().Get("cd_id")
		if jobIDStr == "" && cdIDStr == "" {
			http.Error(w, "job_id ou cd_id obrigatório", http.StatusBadRequest)
			return
		}

		var cdNome, filialNome, jobFilename string
		var cdID int

		if jobIDStr != "" {
			err := db.QueryRow(`
				SELECT cd.nome, f.nome, j.filename, j.cd_id
				FROM farol.sp_csv_jobs j
				JOIN farol.sp_centros_dist cd ON cd.id = j.cd_id
				JOIN farol.sp_filiais f ON f.id = j.filial_id
				WHERE j.id = $1 AND j.empresa_id = $2
			`, jobIDStr, spCtx.EmpresaID).Scan(&cdNome, &filialNome, &jobFilename, &cdID)
			if err == sql.ErrNoRows {
				http.Error(w, "Job não encontrado", http.StatusNotFound)
				return
			}
		} else {
			err := db.QueryRow(`
				SELECT cd.nome, f.nome, cd.id
				FROM farol.sp_centros_dist cd
				JOIN farol.sp_filiais f ON f.id = cd.filial_id
				WHERE cd.id = $1 AND cd.empresa_id = $2
			`, cdIDStr, spCtx.EmpresaID).Scan(&cdNome, &filialNome, &cdID)
			if err == sql.ErrNoRows {
				http.Error(w, "CD não encontrado", http.StatusNotFound)
				return
			}
		}

		filter := "WHERE p.empresa_id = $1 AND p.status = 'aprovada'"
		args   := []any{spCtx.EmpresaID}
		idx    := 2

		if jobIDStr != "" {
			filter += fmt.Sprintf(" AND p.job_id = $%d", idx)
			args = append(args, jobIDStr)
		} else {
			filter += fmt.Sprintf(" AND p.cd_id = $%d", idx)
			args = append(args, cdIDStr)
		}

		// Filtro de rua (opcional) — lista separada por vírgula: "1,2,5"
		ruaStr := r.URL.Query().Get("rua")
		if ruaStr != "" {
			var placeholders []string
			for _, part := range strings.Split(ruaStr, ",") {
				if rua, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
					placeholders = append(placeholders, fmt.Sprintf("$%d", idx))
					args = append(args, rua)
					idx++
				}
			}
			if len(placeholders) > 0 {
				filter += " AND p.rua IN (" + strings.Join(placeholders, ",") + ")"
			}
		}

		// Ordenado por RUA → PREDIO → APTO → CURVA (operador percorre fisicamente)
		query := fmt.Sprintf(`
			SELECT p.codprod,
			       COALESCE(p.produto,''),
			       p.rua, p.predio, p.apto,
			       COALESCE(p.classe_venda,'C'),
			       p.capacidade_atual,
			       COALESCE(p.sugestao_editada, p.sugestao_calibragem),
			       p.delta,
			       COALESCE(p.justificativa,''),
			       TO_CHAR(p.aprovado_em,'DD/MM/YYYY HH24:MI')
			FROM farol.sp_propostas p
			%s
			ORDER BY p.rua NULLS LAST, p.predio NULLS LAST, p.apto NULLS LAST, p.classe_venda
		`, filter)

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, "Erro ao carregar propostas: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var propostas []pdfProposta
		for rows.Next() {
			var p pdfProposta
			var aprovadoEm *string
			if err := rows.Scan(
				&p.Codprod, &p.Produto,
				&p.Rua, &p.Predio, &p.Apto,
				&p.ClasseVenda,
				&p.CapacidadeAtual,
				&p.NovaCapacidade,
				&p.Delta,
				&p.Justificativa,
				&aprovadoEm,
			); err != nil {
				continue
			}
			if aprovadoEm != nil {
				p.AprovadoEm = *aprovadoEm
			}
			propostas = append(propostas, p)
		}

		if len(propostas) == 0 {
			http.Error(w, "Nenhuma proposta aprovada encontrada", http.StatusNotFound)
			return
		}

		pdfBytes, err := buildPDF(cdNome, filialNome, jobFilename, propostas)
		if err != nil {
			http.Error(w, "Erro ao gerar PDF: "+err.Error(), http.StatusInternalServerError)
			return
		}

		ruaLabel := ""
		if ruaStr != "" {
			ruaLabel = "_rua" + strings.ReplaceAll(ruaStr, ",", "-")
		}
		filename := fmt.Sprintf("calibracao_%s%s_%s.pdf",
			sanitizeFilename(cdNome),
			ruaLabel,
			time.Now().Format("20060102"))

		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(pdfBytes)))
		w.Write(pdfBytes)
	}
}

// ─── Agrupamento por RUA ──────────────────────────────────────────────────────

type ruaGroup struct {
	rua   *int
	items []pdfProposta
}

func groupByRua(propostas []pdfProposta) []ruaGroup {
	var groups []ruaGroup
	for _, p := range propostas {
		if len(groups) == 0 || !intPtrEq(groups[len(groups)-1].rua, p.Rua) {
			groups = append(groups, ruaGroup{rua: p.Rua})
		}
		groups[len(groups)-1].items = append(groups[len(groups)-1].items, p)
	}
	return groups
}

func intPtrEq(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// ─── Construção do PDF ────────────────────────────────────────────────────────

func buildPDF(cdNome, filialNome, jobFilename string, propostas []pdfProposta) ([]byte, error) {
	cfg := config.NewBuilder().
		WithPageNumber(props.PageNumber{
			Pattern: "Pág. {current}/{total}",
			Place:   props.RightBottom,
		}).
		WithLeftMargin(10).
		WithRightMargin(10).
		WithTopMargin(12).
		Build()

	mrt := maroto.New(cfg)

	groups := groupByRua(propostas)

	// Resumo para a primeira página
	countA, countB, countC := 0, 0, 0
	for _, p := range propostas {
		switch p.ClasseVenda {
		case "A":
			countA++
		case "B":
			countB++
		default:
			countC++
		}
	}

	for i, g := range groups {
		pg := page.New()

		// ── Cabeçalho do documento (apenas primeira página) ──────────────
		if i == 0 {
			pg.Add(
				row.New(10).Add(
					col.New(12).Add(
						text.New("Relatório de Calibragem de Picking", props.Text{
							Size:  13,
							Style: fontstyle.Bold,
							Align: align.Center,
						}),
					),
				),
				row.New(6).Add(
					col.New(7).Add(
						text.New(fmt.Sprintf("CD: %s  —  Filial: %s", cdNome, filialNome), props.Text{
							Size: 8,
						}),
					),
					col.New(5).Add(
						text.New(fmt.Sprintf("Gerado em: %s", time.Now().Format("02/01/2006 15:04")), props.Text{
							Size:  8,
							Align: align.Right,
						}),
					),
				),
			)
			if jobFilename != "" {
				pg.Add(
					row.New(5).Add(
						col.New(12).Add(
							text.New(fmt.Sprintf("Importação: %s", jobFilename), props.Text{
								Size:  7,
								Style: fontstyle.Italic,
							}),
						),
					),
				)
			}
			pg.Add(
				row.New(5).Add(
					col.New(12).Add(
						text.New(fmt.Sprintf(
							"Total aprovado: %d itens  (Curva A: %d | B: %d | C: %d)  —  %d ruas",
							len(propostas), countA, countB, countC, len(groups),
						), props.Text{Size: 7, Style: fontstyle.Italic}),
					),
				),
				row.New(3).Add(col.New(12)), // espaço
			)
		}

		// ── Subheader da RUA ─────────────────────────────────────────────
		ruaLabel := "—"
		if g.rua != nil {
			ruaLabel = fmt.Sprintf("%d", *g.rua)
		}
		pg.Add(
			row.New(7).Add(
				col.New(12).Add(
					text.New(
						fmt.Sprintf("RUA %s  —  %d item(s)", ruaLabel, len(g.items)),
						props.Text{
							Size:  9,
							Style: fontstyle.Bold,
						},
					),
				),
			),
		)

		// ── Cabeçalho da tabela ──────────────────────────────────────────
		pg.Add(tableHeaderRow())

		// ── Linhas de dados ──────────────────────────────────────────────
		for _, p := range g.items {
			pg.Add(tableDataRow(p)...)
		}

		mrt.AddPages(pg)
	}

	doc, err := mrt.Generate()
	if err != nil {
		return nil, err
	}
	return doc.GetBytes(), nil
}

// ─── Helpers de tabela ────────────────────────────────────────────────────────

// Colunas: Curva(1) | Cód(1) | Produto(4) | Prédio(1) | Apto(1) | Cap.Atual(1) | Nova Cap(1) | Ação(2)
func tableHeaderRow() core.Row {
	h := props.Text{Size: 7, Style: fontstyle.Bold, Align: align.Center}
	return row.New(6).Add(
		col.New(1).Add(text.New("Curva",    h)),
		col.New(1).Add(text.New("Cód.",     h)),
		col.New(4).Add(text.New("Produto",  h)),
		col.New(1).Add(text.New("Prédio",   h)),
		col.New(1).Add(text.New("Apto",     h)),
		col.New(1).Add(text.New("Cap.At.",  h)),
		col.New(1).Add(text.New("Nova Cap", h)),
		col.New(2).Add(text.New("Ação",     h)),
	)
}

func tableDataRow(p pdfProposta) []core.Row {
	d := props.Text{Size: 7, Align: align.Center}
	l := props.Text{Size: 7, Align: align.Left}

	predioStr := "—"
	if p.Predio != nil {
		predioStr = fmt.Sprintf("%d", *p.Predio)
	}
	aptoStr := "—"
	if p.Apto != nil {
		aptoStr = fmt.Sprintf("%d", *p.Apto)
	}
	capAtual := "—"
	if p.CapacidadeAtual != nil {
		capAtual = fmt.Sprintf("%d cx", *p.CapacidadeAtual)
	}

	var acaoStr string
	switch {
	case p.Delta > 0:
		acaoStr = fmt.Sprintf("+%d cx", p.Delta)
	case p.Delta < 0:
		acaoStr = fmt.Sprintf("%d cx", p.Delta)
	default:
		acaoStr = "OK"
	}

	produto := p.Produto
	if len(produto) > 40 {
		produto = produto[:38] + "…"
	}

	dataRow := row.New(5).Add(
		col.New(1).Add(text.New(p.ClasseVenda,                         d)),
		col.New(1).Add(text.New(fmt.Sprintf("%d", p.Codprod),          d)),
		col.New(4).Add(text.New(produto,                               l)),
		col.New(1).Add(text.New(predioStr,                             d)),
		col.New(1).Add(text.New(aptoStr,                               d)),
		col.New(1).Add(text.New(capAtual,                              d)),
		col.New(1).Add(text.New(fmt.Sprintf("%d cx", p.NovaCapacidade), d)),
		col.New(2).Add(text.New(acaoStr,                               d)),
	)

	noteRow := row.New(5).Add(
		col.New(1),
		col.New(11).Add(text.New(
			"Obs: _______________________________________________",
			props.Text{
				Size:  6,
				Align: align.Left,
				Color: &props.Color{Red: 180, Green: 180, Blue: 180},
			},
		)),
	)

	return []core.Row{dataRow, noteRow}
}

// ─── Utilitários ─────────────────────────────────────────────────────────────

func formatEndereco(rua, predio, apto *int) string {
	parts := []string{}
	if rua    != nil { parts = append(parts, fmt.Sprintf("%d", *rua)) }
	if predio != nil { parts = append(parts, fmt.Sprintf("%d", *predio)) }
	if apto   != nil { parts = append(parts, fmt.Sprintf("%d", *apto)) }
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, "-")
}

func sanitizeFilename(s string) string {
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			result = append(result, c)
		} else {
			result = append(result, '_')
		}
	}
	return string(result)
}
