package handlers

// sp_motor.go — Motor de Calibragem Farol
//
// Story 4.5 — Motor de Calibragem
//
// POST /api/sp/motor/calibrar → executa o motor para um job processado
//
// Lógica do motor:
//   Para cada endereço em sp_enderecos do job:
//     1. Lê os parâmetros do CD (sp_motor_params)
//     2. Determina a média de vendas diária relevante (cx ou unidade)
//     3. Calcula sugestão = ceil(med_venda * dias_max_estoque_da_curva * fator_seguranca)
//     4. Aplica regra curva_a_nunca_reduz: Curva A → sugestão ≥ capacidade_atual
//     5. Aplica min_capacidade
//     6. Gera justificativa textual
//     7. Insere em sp_propostas (status = 'pendente')
//
// Idempotente: se já existem propostas para o job, retorna 409 Conflict.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
)

// ─── Tipos internos do motor ──────────────────────────────────────────────────

type motorParams struct {
	DiasAnalise      int
	CurvaAMaxEst     int
	CurvaBMaxEst     int
	CurvaCMaxEst     int
	FatorSeguranca   float64
	CurvaANuncaReduz bool
	MinCapacidade    int
}

type enderecoDB struct {
	ID              int64
	CodFilial       int
	CodProd         int
	Produto         string
	Rua             *int
	Predio          *int
	Apto            *int
	ClasseVenda     string
	ClasseVendaDias *int     // CLASSEVENDA_DIAS do CSV — usado diretamente na fórmula
	Capacidade      *int
	NormaPalete     *int     // NORMA_PALETE — arredonda sugestão para múltiplo de palete
	MedVendaCx      *float64
	MedVendaDias    *float64
	MedDiasEstoque  *float64
	MedVendaCxAA    *float64
	UnidadeMaster   *int
	QtAcesso90      *int     // QTACESSO_PICKING_PERIODO_90 — acessos ao picking em 90 dias
	QtDias          *int     // QT_DIAS — dias do período de análise
}

// ─── Handler ──────────────────────────────────────────────────────────────────

func nilIfEmptyStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// SpMotorCalibrarHandler executa o motor de calibragem para um job.
// POST /api/sp/motor/calibrar
// Body: { "job_id": "uuid" }
//
// Fórmula aplicada: sugestão = ceil( ceil(giro/master) × diasClasse × fator ) → múltiplo de norma_palete
// Giro primário: QTACESSO_PICKING_PERIODO_90 / QT_DIAS (Curva ABC de Acesso ao Picking)
// Fallbacks:     MED_VENDA_DIAS → MED_VENDA_DIAS_CX×master → MED_VENDA_CX_AA×master
func SpMotorCalibrarHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		spCtx := GetSpContext(r)
		if spCtx == nil || !spCtx.CanApprove() {
			http.Error(w, "Forbidden: gestor_geral+ para executar o motor", http.StatusForbidden)
			return
		}

		var body struct {
			JobID string `json:"job_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.JobID == "" {
			http.Error(w, "job_id obrigatório", http.StatusBadRequest)
			return
		}

		// Verifica que o job pertence à empresa e está concluído
		var cdID, filialID int
		var jobStatus string
		err := db.QueryRow(`
			SELECT cd_id, filial_id, status
			FROM farol.sp_csv_jobs
			WHERE id = $1 AND empresa_id = $2
		`, body.JobID, spCtx.EmpresaID).Scan(&cdID, &filialID, &jobStatus)
		if err == sql.ErrNoRows {
			http.Error(w, "Job não encontrado", http.StatusNotFound)
			return
		}
		if jobStatus != "done" {
			http.Error(w, "Job ainda não concluído (status: "+jobStatus+")", http.StatusUnprocessableEntity)
			return
		}

		// Bloqueia nova calibração se o CD já tem propostas pendentes (calibragem em andamento)
		var temPendente bool
		db.QueryRow(`
			SELECT EXISTS(
				SELECT 1 FROM farol.sp_propostas
				WHERE cd_id = $1 AND empresa_id = $2 AND status = 'pendente'
			)
		`, cdID, spCtx.EmpresaID).Scan(&temPendente)
		if temPendente {
			http.Error(w, "Calibragem em andamento para este CD. Finalize as propostas pendentes antes de iniciar uma nova calibração.", http.StatusConflict)
			return
		}

		// Idempotência: verifica se já foram geradas propostas para este job
		var jaExiste bool
		db.QueryRow(`SELECT EXISTS(SELECT 1 FROM farol.sp_propostas WHERE job_id = $1 AND empresa_id = $2)`, body.JobID, spCtx.EmpresaID).Scan(&jaExiste)
		if jaExiste {
			http.Error(w, "Propostas já geradas para este job", http.StatusConflict)
			return
		}

		// Carrega parâmetros do motor para o CD
		params, err := loadMotorParams(db, cdID)
		if err != nil {
			http.Error(w, "Parâmetros do motor não encontrados para o CD", http.StatusUnprocessableEntity)
			return
		}

		// Cria registro de histórico antes de lançar o motor
		historicoID := CriarHistorico(db, body.JobID, spCtx.EmpresaID, spCtx.UserID, cdID)

		// Executa o motor em background para não bloquear o request
		go func() {
			gerado, erros := executarMotor(db, body.JobID, cdID, spCtx.EmpresaID, params)
			log.Printf("[Motor] job %s: %d propostas geradas, %d erros", body.JobID, gerado, erros)
			FecharHistoricoAuto(db, historicoID, body.JobID)
		}()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Motor iniciado em background",
			"job_id":  body.JobID,
		})
	}
}

// ─── Lógica do motor ──────────────────────────────────────────────────────────

func loadMotorParams(db *sql.DB, cdID int) (*motorParams, error) {
	var p motorParams
	err := db.QueryRow(`
		SELECT dias_analise, curva_a_max_est, curva_b_max_est, curva_c_max_est,
		       fator_seguranca, curva_a_nunca_reduz, min_capacidade
		FROM farol.sp_motor_params
		WHERE cd_id = $1
	`, cdID).Scan(&p.DiasAnalise, &p.CurvaAMaxEst, &p.CurvaBMaxEst, &p.CurvaCMaxEst,
		&p.FatorSeguranca, &p.CurvaANuncaReduz, &p.MinCapacidade)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func executarMotor(db *sql.DB, jobID string, cdID int, empresaID string, params *motorParams) (gerado, erros int) {
	// Carrega produtos ignorados para este CD (skip sem gerar proposta)
	ignorados := carregarIgnorados(db, empresaID, cdID)

	rows, err := db.Query(`
		SELECT id, cod_filial, codprod, COALESCE(produto,''), rua, predio, apto,
		       COALESCE(classe_venda,''), classe_venda_dias, capacidade, norma_palete,
		       med_venda_cx, med_venda_dias, med_dias_estoque, med_venda_cx_aa, unidade_master,
		       qt_acesso_90, qt_dias
		FROM farol.sp_enderecos
		WHERE job_id = $1
	`, jobID)
	if err != nil {
		log.Printf("[Motor] erro ao carregar endereços: %v", err)
		return 0, 1
	}
	defer rows.Close()

	const batchSize = 200
	type proposta struct {
		EnderecoID        int64
		CodFilial         int
		CodProd           int
		Produto           string
		Rua, Predio, Apto *int
		ClasseVenda       string
		CapacidadeAtual   *int
		Sugestao          int
		Justificativa     string
		Status            string // 'pendente' | 'calibrado'
	}

	var batch []proposta

	flush := func() {
		if len(batch) == 0 {
			return
		}
		tx, err := db.Begin()
		if err != nil {
			erros += len(batch)
			batch = batch[:0]
			return
		}
		defer tx.Rollback()

		stmt, err := tx.Prepare(`
			INSERT INTO farol.sp_propostas
			  (job_id, endereco_id, empresa_id, cd_id,
			   cod_filial, codprod, produto, rua, predio, apto,
			   classe_venda, capacidade_atual, sugestao_calibragem, justificativa, status)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		`)
		if err != nil {
			erros += len(batch)
			batch = batch[:0]
			return
		}
		defer stmt.Close()

		for _, p := range batch {
			_, execErr := stmt.Exec(
				jobID, p.EnderecoID, empresaID, cdID,
				p.CodFilial, p.CodProd, p.Produto, p.Rua, p.Predio, p.Apto,
				nilIfEmptyStr(p.ClasseVenda), p.CapacidadeAtual, p.Sugestao, p.Justificativa, p.Status,
			)
			if execErr != nil {
				log.Printf("[Motor] erro ao inserir proposta: %v", execErr)
				erros++
			} else {
				gerado++
			}
		}
		tx.Commit()
		batch = batch[:0]
	}

	for rows.Next() {
		var e enderecoDB
		if err := rows.Scan(
			&e.ID, &e.CodFilial, &e.CodProd, &e.Produto,
			&e.Rua, &e.Predio, &e.Apto, &e.ClasseVenda,
			&e.ClasseVendaDias, &e.Capacidade, &e.NormaPalete,
			&e.MedVendaCx, &e.MedVendaDias,
			&e.MedDiasEstoque, &e.MedVendaCxAA, &e.UnidadeMaster,
			&e.QtAcesso90, &e.QtDias,
		); err != nil {
			erros++
			continue
		}

		// Produto ignorado: não gera proposta neste ciclo
		if ignorados[fmt.Sprintf("%d:%d", e.CodProd, e.CodFilial)] {
			continue
		}

		sugestao, justificativa := calcularSugestao(e, params)

		// Determina status: calibrado se dentro de 5% da capacidade atual (≥95% assertividade)
		status := "pendente"
		if e.Capacidade != nil && *e.Capacidade > 0 {
			diff := math.Abs(float64(sugestao-*e.Capacidade)) / float64(*e.Capacidade)
			if diff <= 0.05 {
				status = "calibrado"
			}
		}

		batch = append(batch, proposta{
			EnderecoID:      e.ID,
			CodFilial:       e.CodFilial,
			CodProd:         e.CodProd,
			Produto:         e.Produto,
			Rua:             e.Rua,
			Predio:          e.Predio,
			Apto:            e.Apto,
			ClasseVenda:     e.ClasseVenda,
			CapacidadeAtual: e.Capacidade,
			Sugestao:        sugestao,
			Justificativa:   justificativa,
			Status:          status,
		})

		if len(batch) >= batchSize {
			flush()
		}
	}
	flush()
	return
}

// carregarIgnorados retorna um set de "codprod:cod_filial" ignorados para o CD.
func carregarIgnorados(db *sql.DB, empresaID string, cdID int) map[string]bool {
	rows, err := db.Query(`
		SELECT codprod, cod_filial FROM farol.sp_ignorados
		WHERE empresa_id = $1 AND cd_id = $2
	`, empresaID, cdID)
	if err != nil {
		log.Printf("[Motor] aviso: não foi possível carregar ignorados: %v", err)
		return map[string]bool{}
	}
	defer rows.Close()
	m := map[string]bool{}
	for rows.Next() {
		var cod, filial int
		if rows.Scan(&cod, &filial) == nil {
			m[fmt.Sprintf("%d:%d", cod, filial)] = true
		}
	}
	return m
}

// calcularSugestao aplica a fórmula WMS de calibragem e retorna (sugestão, justificativa).
//
// Giro (prioridade):
//   1. QTACESSO_PICKING_PERIODO_90 / QT_DIAS  ← Curva ABC de Acesso ao Picking (JC)
//   2. MED_VENDA_DIAS                          ← média de vendas diária em unidades
//   3. MED_VENDA_DIAS_CX × unidadeMaster       ← fallback caixas
//   4. MED_VENDA_DIAS_CX_ANOANT_MESSEG × master ← fallback ano anterior
//
// Fórmula: sugestão = ceil( ceil(giro / master) × diasClasse × fator )
//   depois: arredonda para múltiplo de norma_palete (se norma_palete > 1)
//   depois: aplica mínimo absoluto e regra Curva A nunca reduz
func calcularSugestao(e enderecoDB, p *motorParams) (int, string) {
	curva := strings.ToUpper(e.ClasseVenda)

	unidadeMaster := 1
	if e.UnidadeMaster != nil && *e.UnidadeMaster > 1 {
		unidadeMaster = *e.UnidadeMaster
	}

	// ── 1. Giro diário ────────────────────────────────────────────────────────
	var giroDia float64
	var fonteGiro string

	switch {
	case e.QtAcesso90 != nil && *e.QtAcesso90 > 0 && e.QtDias != nil && *e.QtDias > 0:
		giroDia = float64(*e.QtAcesso90) / float64(*e.QtDias)
		fonteGiro = "ACESSO_PICKING/DIA"
	case e.MedVendaDias != nil && *e.MedVendaDias > 0:
		giroDia = *e.MedVendaDias
		fonteGiro = "MED_VENDA_DIAS"
	case e.MedVendaCx != nil && *e.MedVendaCx > 0:
		giroDia = *e.MedVendaCx * float64(unidadeMaster)
		fonteGiro = "MED_VENDA_DIAS_CX×master"
	case e.MedVendaCxAA != nil && *e.MedVendaCxAA > 0:
		giroDia = *e.MedVendaCxAA * float64(unidadeMaster)
		fonteGiro = "MED_VENDA_CX_AA×master"
	}

	// ── 2. Dias da classe ─────────────────────────────────────────────────────
	var diasClasse int
	var fonteDias string

	if e.ClasseVendaDias != nil && *e.ClasseVendaDias > 0 {
		diasClasse = *e.ClasseVendaDias
		fonteDias = "CSV"
	} else {
		switch curva {
		case "A":
			diasClasse = p.CurvaAMaxEst
		case "B":
			diasClasse = p.CurvaBMaxEst
		default:
			diasClasse = p.CurvaCMaxEst
		}
		fonteDias = "params"
	}

	// ── 3. Fórmula base ───────────────────────────────────────────────────────
	caixasGiro := int(math.Ceil(giroDia / float64(unidadeMaster)))
	formulaBase := int(math.Ceil(float64(caixasGiro*diasClasse) * p.FatorSeguranca))
	sugestao := formulaBase

	// ── 4. Mínimo absoluto ────────────────────────────────────────────────────
	if sugestao < p.MinCapacidade {
		sugestao = p.MinCapacidade
	}

	// ── 5. Norma Palete: arredonda para múltiplo de norma_palete ─────────────
	var notaNorma string
	if e.NormaPalete != nil && *e.NormaPalete > 1 {
		np := *e.NormaPalete
		if sugestao%np != 0 {
			sugestao = ((sugestao / np) + 1) * np
			notaNorma = fmt.Sprintf(" →↑%dcx/palete=%d", np, sugestao)
		}
	}

	// ── 6. Curva A: nunca reduz ───────────────────────────────────────────────
	capAtual := 0
	if e.Capacidade != nil {
		capAtual = *e.Capacidade
	}
	mantidaCurvaA := curva == "A" && p.CurvaANuncaReduz && sugestao < capAtual
	if mantidaCurvaA {
		sugestao = capAtual
	}

	// ── 7. Justificativa ──────────────────────────────────────────────────────
	base := fmt.Sprintf(
		"Curva %s: ceil(%s=%.2f / master=%d)=%d × %d dias(%s) × %.2f(seg) = %d cx%s",
		curva, fonteGiro, giroDia, unidadeMaster, caixasGiro, diasClasse, fonteDias, p.FatorSeguranca, formulaBase, notaNorma,
	)
	var justificativa string
	if mantidaCurvaA {
		justificativa = base + fmt.Sprintf(" → mantida em %d cx [Curva A nunca reduz]", sugestao)
	} else {
		justificativa = base
	}

	return sugestao, justificativa
}
