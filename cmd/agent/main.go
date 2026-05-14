package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"agent/internal/agent"
	"agent/internal/config"
	agentgit "agent/internal/git"
	"agent/internal/logging"
	"agent/internal/message"
	"agent/internal/pr"
	"agent/internal/provider"
	"agent/internal/provider/claude"
	"agent/internal/provider/ollama"
	"agent/internal/provider/openai"
	"agent/internal/score"
	"agent/internal/skill"
	"agent/internal/task"
	"agent/internal/tool"
	"agent/internal/tool/builtin"
)

func main() {
	if err := logging.Init(os.Getenv("LOG_LEVEL")); err != nil {
		fmt.Fprintf(os.Stderr, "invalid log level: %v\n", err)
		os.Exit(1)
	}

	if err := run(); err != nil {
		logging.Fatalf("Fatal: %v", err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config error: %w", err)
	}
	logging.Infof("Provider: %s | Workspace: %s", cfg.Provider, cfg.WorkspacePath)

	// Git setup: ensure repo exists and create working branch
	branch, err := agentgit.Setup(agentgit.Config{
		Token:      cfg.GitToken,
		WorkDir:    cfg.WorkspacePath,
		RepoURL:    cfg.RepoURL,
		BaseBranch: cfg.BaseBranch,
	})

	if err != nil {
		return fmt.Errorf("git setup: %w", err)
	}

	logging.Infof("Working branch: %s", branch)

	// Load task
	t, err := task.LoadFromFile(cfg.TaskFile)
	if err != nil {
		return fmt.Errorf("task error: %w", err)
	}

	logging.Infof("Task: %s", t.Title)

	// Record branch in session
	t.EnsureSession()
	t.Session.Branch = branch

	// If resuming with answers, record them as decisions
	t.RecordAnswersAsDecisions()

	// Load skills
	skills, err := skill.LoadAll(cfg.SkillsPath)
	if err != nil {
		return fmt.Errorf("skills error: %w", err)
	}
	logSkills(skills)

	// Setup tool registry
	registry := tool.NewRegistry()
	toolTimeout := time.Duration(cfg.ToolTimeout) * time.Second
	tools := builtin.RegisterAll(registry, cfg.WorkspacePath, toolTimeout)
	registry.ApplySkillConfig(skill.MergeToolConfigs(skills))

	// Create provider
	p := createProvider(cfg)

	// Build prompts
	systemPrompt := agent.BuildSystemPrompt(skills, registry)
	taskPrompt := buildTaskPrompt(t)

	// Context with graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Run agent
	ag := agent.New(p, registry, tools, t, cfg.MaxIterations)
	if err := ag.Run(ctx, systemPrompt, taskPrompt); err != nil {
		logging.Errorf("Agent error: %v", err)
	}
	// Populate skills in result
	if t.Result != nil {
		for _, s := range skills {
			t.Result.Skills = append(t.Result.Skills, s.Name)
		}
	}
	// Evaluation + rework Loop (only if agent completed successfully)
	if t.Result != nil && t.Result.Status == "success" {
		evaluateAndRework(ctx, cfg, t, ag, p, registry, tools, skills, systemPrompt)
	}
	// Commit work and push if possible
	finalizeGit(cfg, branch, t)

	// Create PR if score passed and branch was pushed
	if t.Result != nil && t.Result.Status == "success" && t.Result.Score >= t.GetScoreThreshold() && cfg.GitToken != "" {
		createPullRequest(ctx, cfg, t, branch)
	}
	// Save task.json (contains questions, session, result - everything)
	if err := t.SaveToFile(cfg.TaskFile); err != nil {
		logging.Warnf("failed to save task.json: %v", err)
	}
	return exit(t)
}

func buildTaskPrompt(t *task.Task) string {
	prompt := t.ToPrompt()
	if answers := t.AnswersPrompt(); answers != "" {
		prompt += "\n\n" + answers
	}
	if continuation := t.ContinuationPrompt(); continuation != "" {
		prompt += "\n\n" + continuation
	}
	return prompt
}

func evaluateAndRework(ctx context.Context, cfg *config.Config, t *task.Task, ag *agent.Agent, p provider.Provider, registry *tool.Registry, tools *builtin.Tools, skills []skill.Skill, systemPrompt string) {
	threshold := t.GetScoreThreshold()
	evaluator := &score.Evaluator{
		WorkDir:   cfg.WorkspacePath,
		Threshold: threshold,
	}
	for attempt := 0; attempt <= cfg.MaxReworkAttempts; attempt++ {
		// Ask the AI for self-assessment
		selfScore := getSelfAssessment(ctx, p, t)
		logging.Infof("[eval] AI self-assessment: %d/100", selfScore)

		// Run full evaluation
		sc := evaluator.Evaluate(ctx, selfScore)
		t.Result.Score = sc.Total
		t.Result.ScoreDetail = sc.Breakdown

		if sc.PassedCheck {
			logging.Infof("[eval] Score %d >= threshold %d - passing", sc.Total, threshold)
			return
		}
		if attempt >= cfg.MaxReworkAttempts {
			logging.Warnf("[eval] Score %d < threshold %d - max rework attempts reached", sc.Total, threshold)
			return
		}
		// Ask agent to rework
		logging.Warnf("[eval] Score %d < threshold %d - requesting rework (attempt %d/%d)", sc.Total, threshold, attempt+1, cfg.MaxReworkAttempts)
		reworkPrompt := buildReworkPrompt(sc, t)

		agRework := agent.New(p, registry, tools, t, cfg.MaxIterations/2)
		if err := agRework.Run(ctx, systemPrompt, reworkPrompt); err != nil {
			logging.Errorf("[eval] Rework failed: %v", err)
			return
		}
	}
}

func getSelfAssessment(ctx context.Context, p provider.Provider, t *task.Task) int {
	prompt := score.SelfAssessPrompt(t.Title, t.Description)

	// Use a simple message exchange for self-assessment (no system prompt needed)
	conv := message.NewConversation("You are a code quality evaluator. Respond only with JSON.")
	conv.AddUserMessage(prompt)

	resp, err := p.SendMessage(ctx, conv, nil)
	if err != nil {
		logging.Errorf("[eval] Self-assessment failed: %v", err)
		return 50 // default to middle score on failure
	}
	// Parse the JSON response
	type assessResponse struct {
		Score     int    `json:"score"`
		Reasoning string `json:"reasoning"`
	}

	// Try to find JSON in the response
	content := resp.Content
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		content = content[start : end+1]
	}

	var assess assessResponse
	if err := json.Unmarshal([]byte(content), &assess); err != nil {
		logging.Errorf("[eval] Failed to parse self-assessment response: %v", err)
		return 50
	}

	logging.Debugf("[eval] Self-assessment reasoning: %s", assess.Reasoning)
	return assess.Score
}

func buildReworkPrompt(sc *score.Score, t *task.Task) string {
	var b strings.Builder
	b.WriteString("## Rework Required\n\n")
	b.WriteString(fmt.Sprintf("Your previous work scored %d/100 which is below the threshold.\n\n", sc.Total))

	if len(sc.Details) > 0 {
		b.WriteString("### Issues to fix:\n")
		for _, d := range sc.Details {
			b.WriteString(fmt.Sprintf("- %s\n", d))
		}
		b.WriteString("\n")
	}

	b.WriteString("### Score breakdown:\n")
	for component, pts := range sc.Breakdown {
		b.WriteString(fmt.Sprintf("- %s: %d\n", component, pts))
	}
	b.WriteString("\n")

	b.WriteString("Fix the issues above. The original task was:\n\n")
	b.WriteString(t.ToPrompt())
	return b.String()
}

func createPullRequest(ctx context.Context, cfg *config.Config, t *task.Task, branch string) {
	remoteURL := pr.GetRemoteURL(cfg.WorkspacePath)
	if remoteURL == "" && cfg.RepoURL != "" {
		remoteURL = cfg.RepoURL
	}
	if remoteURL == "" {
		logging.Warn("[pr] No remote URL available - skipping PR creation")
		return
	}

	owner, repo := pr.ParseRemoteURL(remoteURL)
	if owner == "" || repo == "" {
		logging.Warnf("[pr] Unable to parse owner/repo from: %s", remoteURL)
		return
	}
	baseBranch := cfg.BaseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}
	prCfg := &pr.Config{
		Platform:   pr.Platform(cfg.GitPlatform),
		Token:      cfg.GitToken,
		RepoOwner:  owner,
		RepoName:   repo,
		BaseBranch: baseBranch,
		HeadBranch: branch,
		RemoteURL:  remoteURL,
	}

	// Build PR body
	sc := &score.Score{
		Total:     t.Result.Score,
		Breakdown: t.Result.ScoreDetail,
	}
	filesChanged := pr.GetChangedFiles(cfg.WorkspacePath, baseBranch)
	body := pr.BuildPRBody(t.Description, t.Result.Summary, sc, filesChanged)

	result, err := pr.CreatePR(ctx, prCfg, t.Title, body)
	if err != nil {
		logging.Errorf("[pr] Failed to create PR: %v", err)
		return
	}

	t.Result.PRURL = result.URL
	logging.Infof("[pr] Created PR #%d: %s", result.Number, result.URL)
}

func finalizeGit(cfg *config.Config, branch string, t *task.Task) {
	// Comit any remaining changes
	msg := fmt.Sprintf("agent: %s", t.Title)
	if err := agentgit.CommitAll(cfg.WorkspacePath, msg); err != nil {
		logging.Errorf("Git commit: %v", err)
	}
	// Push branch if token is available
	if cfg.GitToken != "" {
		if err := agentgit.Push(cfg.WorkspacePath, branch, cfg.GitToken); err != nil {
			logging.Warnf("Git push: %v (non-fatal)", err)
		} else {
			logging.Infof("Pushed branch %s to origin", branch)
		}
	}
}

func logSkills(skills []skill.Skill) {
	if len(skills) == 0 {
		logging.Info("No skills loaded (generic mode)")
		return
	}
	names := make([]string, len(skills))
	for i, s := range skills {
		names[i] = s.Name
	}
	logging.Infof("Skills: %v", names)
}

func createProvider(cfg *config.Config) provider.Provider {
	switch cfg.Provider {
	case config.ProviderClaude:
		return claude.New(cfg.AnthropicKey, cfg.ClaudeModel)
	case config.ProviderOpenAI:
		return openai.New(cfg.OpenAIKey, cfg.OpenAIModel)
	case config.ProviderOllama:
		return ollama.New(cfg.OllamaURL, cfg.OllamaModel)
	default:
		return claude.New(cfg.AnthropicKey, cfg.ClaudeModel)
	}
}

func exit(t *task.Task) error {
	if t.Result == nil {
		os.Exit(2)
	}
	switch t.Result.Status {
	case "success":
		fmt.Println(t.Result.Summary)
		os.Exit(0)
	case "paused":
		fmt.Println("Agent paused - questions written to task. json. Answer them and re-run.")
		os.Exit(3)
	case "failure":
		fmt.Println(t.Result.Summary)
		os.Exit(1)
	default:
		os.Exit(2)
	}
	return nil
}
