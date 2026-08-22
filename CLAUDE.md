<!-- GSD:project-start source:PROJECT.md -->
## Project

**FB_FAROL — Reescrita 2026**

Sistema de Farol de Vendas (semáforo de objetivos) para distribuidoras que operam com força de vendas via ION VENDAS (WinThor/PC Sistemas). Oferece visões hierárquicas — Diretoria → GGV → Supervisor → RCA → Cliente → Produto — com indicadores de positivação, mix de itens, faturado vs transmitido, e farol binário (verde/vermelho) de atingimento de meta por indústria. Acessível via web autenticado (para Gerentes, GGVs e Supervisores) e via URL pública para o ION VENDAS (RCAs em campo).

**Core Value:** **Mostrar ao gestor — em segundos — quem está atingindo meta e quem não está, com drill-down até cliente/produto, incluindo clientes sem venda.** Se algum indicador secundário falhar, isso ainda precisa funcionar.

### Constraints

- **Stack:** Go 8087 + React 3087 + Postgres + Coolify — sem mudança de stack
- **Compatibilidade ION VENDAS:** URLs `/m/CNPJ/SUP/cod` e `/m/CNPJ/RCA/cod` devem continuar funcionando para o aplicativo de campo
- **Visual:** padrão "Clean Professional" atual mantido e estendido — com UMA exceção registrada: a tela `/farol/dinheiro-na-mesa` é escura. Decisão do dono da JC em 22/08/2026, que preferiu esse estilo. A justificativa de produto é que ela é tela de campo: chega por WhatsApp, é lida no celular e compete com a atenção de um aplicativo de mensagem. As telas de trabalho seguem claras; qualquer nova exceção precisa de decisão explícita, não de precedente
- **Banco:** continua Postgres com materialized views (sem migração para OLAP / cubo)
- **Migração:** dados atuais descartados (schema limpo) — gestor aceitou esta perda
<!-- GSD:project-end -->

<!-- GSD:stack-start source:STACK.md -->
## Technology Stack

Technology stack not yet documented. Will populate after codebase mapping or first phase.
<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->
## Conventions

Conventions not yet established. Will populate as patterns emerge during development.
<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->
## Architecture

Architecture not yet mapped. Follow existing patterns found in the codebase.
<!-- GSD:architecture-end -->

<!-- GSD:skills-start source:skills/ -->
## Project Skills

| Skill | Description | Path |
|-------|-------------|------|
| bmad-advanced-elicitation | 'Push the LLM to reconsider, refine, and improve its recent output. Use when user asks for deeper critique or mentions a known deeper critique method, e.g. socratic, first principles, pre-mortem, red team.' | `.claude/skills/bmad-advanced-elicitation/SKILL.md` |
| bmad-agent-analyst | Strategic business analyst and requirements expert. Use when the user asks to talk to Mary or requests the business analyst. | `.claude/skills/bmad-agent-analyst/SKILL.md` |
| bmad-agent-architect | System architect and technical design leader. Use when the user asks to talk to Winston or requests the architect. | `.claude/skills/bmad-agent-architect/SKILL.md` |
| bmad-agent-builder | Builds, edits or analyzes Agent Skills through conversational discovery. Use when the user requests to "Create an Agent", "Analyze an Agent" or "Edit an Agent". | `.claude/skills/bmad-agent-builder/SKILL.md` |
| bmad-agent-dev | Senior software engineer for story execution and code implementation. Use when the user asks to talk to Amelia or requests the developer agent. | `.claude/skills/bmad-agent-dev/SKILL.md` |
| bmad-agent-pm | Product manager for PRD creation and requirements discovery. Use when the user asks to talk to John or requests the product manager. | `.claude/skills/bmad-agent-pm/SKILL.md` |
| bmad-agent-tech-writer | Technical documentation specialist and knowledge curator. Use when the user asks to talk to Paige or requests the tech writer. | `.claude/skills/bmad-agent-tech-writer/SKILL.md` |
| bmad-agent-ux-designer | UX designer and UI specialist. Use when the user asks to talk to Sally or requests the UX designer. | `.claude/skills/bmad-agent-ux-designer/SKILL.md` |
| bmad-bmb-setup | Sets up BMad Builder module in a project. Use when the user requests to 'install bmb module', 'configure BMad Builder', or 'setup BMad Builder'. | `.claude/skills/bmad-bmb-setup/SKILL.md` |
| bmad-brainstorming | 'Facilitate interactive brainstorming sessions using diverse creative techniques and ideation methods. Use when the user says help me brainstorm or help me ideate.' | `.claude/skills/bmad-brainstorming/SKILL.md` |
| bmad-check-implementation-readiness | 'Validate PRD, UX, Architecture and Epics specs are complete. Use when the user says "check implementation readiness".' | `.claude/skills/bmad-check-implementation-readiness/SKILL.md` |
| bmad-checkpoint-preview | 'LLM-assisted human-in-the-loop review. Make sense of a change, focus attention where it matters, test. Use when the user says "checkpoint", "human review", or "walk me through this change".' | `.claude/skills/bmad-checkpoint-preview/SKILL.md` |
| bmad-cis-agent-brainstorming-coach | Elite brainstorming specialist for facilitated ideation sessions. Use when the user asks to talk to Carson or requests the Brainstorming Specialist. | `.claude/skills/bmad-cis-agent-brainstorming-coach/SKILL.md` |
| bmad-cis-agent-creative-problem-solver | Master problem solver for systematic problem-solving methodologies. Use when the user asks to talk to Dr. Quinn or requests the Master Problem Solver. | `.claude/skills/bmad-cis-agent-creative-problem-solver/SKILL.md` |
| bmad-cis-agent-design-thinking-coach | Design thinking maestro for human-centered design processes. Use when the user asks to talk to Maya or requests the Design Thinking Maestro. | `.claude/skills/bmad-cis-agent-design-thinking-coach/SKILL.md` |
| bmad-cis-agent-innovation-strategist | Disruptive innovation oracle for business model innovation and strategic disruption. Use when the user asks to talk to Victor or requests the Disruptive Innovation Oracle. | `.claude/skills/bmad-cis-agent-innovation-strategist/SKILL.md` |
| bmad-cis-agent-presentation-master | Visual communication and presentation expert for slide decks, pitch decks, and visual storytelling. Use when the user asks to talk to Caravaggio or requests the Presentation Expert. | `.claude/skills/bmad-cis-agent-presentation-master/SKILL.md` |
| bmad-cis-agent-storyteller | Master storyteller for compelling narratives using proven frameworks. Use when the user asks to talk to Sophia or requests the Master Storyteller. | `.claude/skills/bmad-cis-agent-storyteller/SKILL.md` |
| bmad-cis-design-thinking | 'Guide human-centered design processes using empathy-driven methodologies. Use when the user says "lets run design thinking" or "I want to apply design thinking"' | `.claude/skills/bmad-cis-design-thinking/SKILL.md` |
| bmad-cis-innovation-strategy | 'Identify disruption opportunities and architect business model innovation. Use when the user says "lets create an innovation strategy" or "I want to find disruption opportunities"' | `.claude/skills/bmad-cis-innovation-strategy/SKILL.md` |
| bmad-cis-problem-solving | 'Apply systematic problem-solving methodologies to complex challenges. Use when the user says "guide me through structured problem solving" or "I want to crack this challenge with guided problem solving techniques"' | `.claude/skills/bmad-cis-problem-solving/SKILL.md` |
| bmad-cis-storytelling | 'Craft compelling narratives using story frameworks. Use when the user says "help me with storytelling" or "I want to create a narrative through storytelling"' | `.claude/skills/bmad-cis-storytelling/SKILL.md` |
| bmad-code-review | 'Review code changes adversarially using parallel review layers (Blind Hunter, Edge Case Hunter, Acceptance Auditor) with structured triage into actionable categories. Use when the user says "run code review" or "review this code"' | `.claude/skills/bmad-code-review/SKILL.md` |
| bmad-correct-course | 'Manage significant changes during sprint execution. Use when the user says "correct course" or "propose sprint change"' | `.claude/skills/bmad-correct-course/SKILL.md` |
| bmad-create-architecture | 'Create architecture solution design decisions for AI agent consistency. Use when the user says "lets create architecture" or "create technical architecture" or "create a solution design"' | `.claude/skills/bmad-create-architecture/SKILL.md` |
| bmad-create-epics-and-stories | 'Break requirements into epics and user stories. Use when the user says "create the epics and stories list"' | `.claude/skills/bmad-create-epics-and-stories/SKILL.md` |
| bmad-create-prd | 'Create a PRD from scratch. Use when the user says "lets create a product requirements document" or "I want to create a new PRD"' | `.claude/skills/bmad-create-prd/SKILL.md` |
| bmad-create-story | 'Creates a dedicated story file with all the context the agent will need to implement it later. Use when the user says "create the next story" or "create story [story identifier]"' | `.claude/skills/bmad-create-story/SKILL.md` |
| bmad-create-ux-design | 'Plan UX patterns and design specifications. Use when the user says "lets create UX design" or "create UX specifications" or "help me plan the UX"' | `.claude/skills/bmad-create-ux-design/SKILL.md` |
| bmad-dev-story | 'Execute story implementation following a context filled story spec file. Use when the user says "dev this story [story file]" or "implement the next story in the sprint plan"' | `.claude/skills/bmad-dev-story/SKILL.md` |
| bmad-distillator | Lossless LLM-optimized compression of source documents. Use when the user requests to 'distill documents' or 'create a distillate'. | `.claude/skills/bmad-distillator/SKILL.md` |
| bmad-document-project | 'Document brownfield projects for AI context. Use when the user says "document this project" or "generate project docs"' | `.claude/skills/bmad-document-project/SKILL.md` |
| bmad-domain-research | 'Conduct domain and industry research. Use when the user says wants to do domain research for a topic or industry' | `.claude/skills/bmad-domain-research/SKILL.md` |
| bmad-edit-prd | 'Edit an existing PRD. Use when the user says "edit this PRD".' | `.claude/skills/bmad-edit-prd/SKILL.md` |
| bmad-editorial-review-prose | 'Clinical copy-editor that reviews text for communication issues. Use when user says review for prose or improve the prose' | `.claude/skills/bmad-editorial-review-prose/SKILL.md` |
| bmad-editorial-review-structure | 'Structural editor that proposes cuts, reorganization, and simplification while preserving comprehension. Use when user requests structural review or editorial review of structure' | `.claude/skills/bmad-editorial-review-structure/SKILL.md` |
| bmad-generate-project-context | 'Create project-context.md with AI rules. Use when the user says "generate project context" or "create project context"' | `.claude/skills/bmad-generate-project-context/SKILL.md` |
| bmad-help | 'Analyzes current state and user query to answer BMad questions or recommend the next skill(s) to use. Use when user asks for help, bmad help, what to do next, or what to start with in BMad.' | `.claude/skills/bmad-help/SKILL.md` |
| bmad-index-docs | 'Generates or updates an index.md to reference all docs in the folder. Use if user requests to create or update an index of all files in a specific folder' | `.claude/skills/bmad-index-docs/SKILL.md` |
| bmad-market-research | 'Conduct market research on competition and customers. Use when the user says they need market research' | `.claude/skills/bmad-market-research/SKILL.md` |
| bmad-module-builder | Plans, creates, and validates BMad modules. Use when the user requests to 'ideate module', 'plan a module', 'create module', 'build a module', or 'validate module'. | `.claude/skills/bmad-module-builder/SKILL.md` |
| bmad-party-mode | 'Orchestrates group discussions between installed BMAD agents, enabling natural multi-agent conversations where each agent is a real subagent with independent thinking. Use when user requests party mode, wants multiple agent perspectives, group discussion, roundtable, or multi-agent conversation about their project.' | `.claude/skills/bmad-party-mode/SKILL.md` |
| bmad-prfaq | Working Backwards PRFAQ challenge to forge product concepts. Use when the user requests to 'create a PRFAQ', 'work backwards', or 'run the PRFAQ challenge'. | `.claude/skills/bmad-prfaq/SKILL.md` |
| bmad-product-brief | Create or update product briefs through guided or autonomous discovery. Use when the user requests to create or update a Product Brief. | `.claude/skills/bmad-product-brief/SKILL.md` |
| bmad-qa-generate-e2e-tests | 'Generate end to end automated tests for existing features. Use when the user says "create qa automated tests for [feature]"' | `.claude/skills/bmad-qa-generate-e2e-tests/SKILL.md` |
| bmad-quick-dev | 'Implements any user intent, requirement, story, bug fix or change request by producing clean working code artifacts that follow the project''s existing architecture, patterns and conventions. Use when the user wants to build, fix, tweak, refactor, add or modify any code, component or feature.' | `.claude/skills/bmad-quick-dev/SKILL.md` |
| bmad-retrospective | 'Post-epic review to extract lessons and assess success. Use when the user says "run a retrospective" or "lets retro the epic [epic]"' | `.claude/skills/bmad-retrospective/SKILL.md` |
| bmad-review-adversarial-general | 'Perform a Cynical Review and produce a findings report. Use when the user requests a critical review of something' | `.claude/skills/bmad-review-adversarial-general/SKILL.md` |
| bmad-review-edge-case-hunter | 'Walk every branching path and boundary condition in content, report only unhandled edge cases. Orthogonal to adversarial review - method-driven not attitude-driven. Use when you need exhaustive edge-case analysis of code, specs, or diffs.' | `.claude/skills/bmad-review-edge-case-hunter/SKILL.md` |
| bmad-shard-doc | 'Splits large markdown documents into smaller, organized files based on level 2 (default) sections. Use if the user says perform shard document' | `.claude/skills/bmad-shard-doc/SKILL.md` |
| bmad-sprint-planning | 'Generate sprint status tracking from epics. Use when the user says "run sprint planning" or "generate sprint plan"' | `.claude/skills/bmad-sprint-planning/SKILL.md` |
| bmad-sprint-status | 'Summarize sprint status and surface risks. Use when the user says "check sprint status" or "show sprint status"' | `.claude/skills/bmad-sprint-status/SKILL.md` |
| bmad-tea | Master Test Architect and Quality Advisor. Use when the user asks to talk to Murat or requests the Test Architect. | `.claude/skills/bmad-tea/SKILL.md` |
| bmad-teach-me-testing | 'Teach testing progressively through structured sessions. Use when user says "lets learn testing" or "I want to study test practices"' | `.claude/skills/bmad-teach-me-testing/SKILL.md` |
| bmad-technical-research | 'Conduct technical research on technologies and architecture. Use when the user says they would like to do or produce a technical research report' | `.claude/skills/bmad-technical-research/SKILL.md` |
| bmad-testarch-atdd | 'Generate red-phase acceptance test scaffolds using the TDD cycle. Use when the user says "lets write acceptance tests" or "I want to do ATDD"' | `.claude/skills/bmad-testarch-atdd/SKILL.md` |
| bmad-testarch-automate | 'Expand test automation coverage for codebase. Use when user says "lets expand test coverage" or "I want to automate tests"' | `.claude/skills/bmad-testarch-automate/SKILL.md` |
| bmad-testarch-ci | 'Scaffold CI/CD quality pipeline with test execution. Use when the user says "lets setup CI pipeline" or "I want to create quality gates"' | `.claude/skills/bmad-testarch-ci/SKILL.md` |
| bmad-testarch-framework | 'Initialize test framework with Playwright or Cypress. Use when the user says "lets setup test framework" or "I want to initialize testing framework"' | `.claude/skills/bmad-testarch-framework/SKILL.md` |
| bmad-testarch-nfr | 'Assess NFRs like performance security and reliability. Use when the user says "lets assess NFRs" or "I want to evaluate non-functional requirements"' | `.claude/skills/bmad-testarch-nfr/SKILL.md` |
| bmad-testarch-test-design | 'Create system-level or epic-level test plans. Use when the user says "lets design test plan" or "I want to create test strategy"' | `.claude/skills/bmad-testarch-test-design/SKILL.md` |
| bmad-testarch-test-review | 'Review test quality using best practices validation. Use when user says "lets review tests" or "I want to evaluate test quality"' | `.claude/skills/bmad-testarch-test-review/SKILL.md` |
| bmad-testarch-trace | 'Generate traceability matrix and quality gate decision. Use when the user says "lets create traceability matrix" or "I want to analyze test coverage"' | `.claude/skills/bmad-testarch-trace/SKILL.md` |
| bmad-validate-prd | 'Validate a PRD against standards. Use when the user says "validate this PRD" or "run PRD validation"' | `.claude/skills/bmad-validate-prd/SKILL.md` |
| bmad-workflow-builder | Builds, converts, and analyzes workflows and skills. Use when the user requests to "build a workflow", "modify a workflow", "quality check workflow", "analyze skill", or "convert a skill". | `.claude/skills/bmad-workflow-builder/SKILL.md` |
<!-- GSD:skills-end -->

<!-- GSD:workflow-start source:GSD defaults -->
## GSD Workflow Enforcement

Before using Edit, Write, or other file-changing tools, start work through a GSD command so planning artifacts and execution context stay in sync.

Use these entry points:
- `/gsd-quick` for small fixes, doc updates, and ad-hoc tasks
- `/gsd-debug` for investigation and bug fixing
- `/gsd-execute-phase` for planned phase work

Do not make direct repo edits outside a GSD workflow unless the user explicitly asks to bypass it.
<!-- GSD:workflow-end -->



<!-- GSD:profile-start -->
## Developer Profile

> Profile not yet configured. Run `/gsd-profile-user` to generate your developer profile.
> This section is managed by `generate-claude-profile` -- do not edit manually.
<!-- GSD:profile-end -->
