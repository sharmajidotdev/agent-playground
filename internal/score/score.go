package score

import (
	"agent/internal/logging"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type Score struct {
	Total       int            `json:"total"`        // 0-100 overall score
	Breakdown   map[string]int `json:"breakdown"`    // component scores
	Details     []string       `json:"details"`      // human-readable notes
	PassedCheck bool           `json:"passed_check"` // whether it meets threshold
}

// Weights for scoring components (out of 100 total)
const (
	WeightBuild      = 25
	WeightTests      = 25
	WeightLint       = 10
	WeightSelfAssess = 40 // AI judges task completion
)

// Evaluator runs the scoring pipeline
type Evaluator struct {
	WorkDir   string
	Threshold int
}

// Evaluate runs all checks and returns a composite score
func (e *Evaluator) Evaluate(ctx context.Context, selfAssessScore int) *Score {
	s := &Score{
		Breakdown: make(map[string]int),
	}

	// 1. Build Check
	buildScore := e.checkBuild(ctx)
	s.Breakdown["build"] = buildScore
	logging.Infof("[score] Build: %d/%d", buildScore, WeightBuild)

	// 2. Test Check
	testScore := e.checkTests(ctx)
	s.Breakdown["tests"] = testScore
	logging.Infof("[score] Tests: %d/%d", testScore, WeightTests)

	// 3. Lint Check
	lintScore := e.checkLint(ctx)
	s.Breakdown["lint"] = lintScore
	logging.Infof("[score] Lint: %d/%d", lintScore, WeightLint)

	// 4. Self-Assessment (AI judgment)
	aiScore := scaleScore(selfAssessScore, WeightSelfAssess)
	s.Breakdown["self_assessment"] = aiScore
	logging.Infof("[score] Self-Assessment: %d/%d", aiScore, WeightSelfAssess)

	// Calculate total score
	s.Total = buildScore + testScore + lintScore + aiScore
	s.PassedCheck = s.Total >= e.Threshold

	//Generate details
	s.Details = e.generateDetails(s)
	logging.Infof("[score] Total: %d/100 (threshold: %d, passed: %v)", s.Total, e.Threshold, s.PassedCheck)
	return s
}

func (e *Evaluator) checkBuild(ctx context.Context) int {
	// Try common build commands in order of detection
	cmds := e.detectBuildCommands()
	if len(cmds) == 0 {
		return WeightBuild // No build system detected - give full marks
	}

	for _, cmd := range cmds {
		if err := runCmd(ctx, e.WorkDir, cmd); err != nil {
			return 0
		}
	}
	return WeightBuild
}

func (e *Evaluator) checkTests(ctx context.Context) int {
	cmd := e.detectTestCommand()
	if cmd == "" {
		return WeightTests / 2 // No tests detected - half marks (not penalizing)
	}

	if err := runCmd(ctx, e.WorkDir, cmd); err != nil {
		// Tests ran but failed
		return 0
	}

	return WeightTests
}

func (e *Evaluator) checkLint(ctx context.Context) int {
	cmd := e.detectLintCommand()
	if cmd == "" {
		return WeightLint // No linter - give full marks
	}

	if err := runCmd(ctx, e.WorkDir, cmd); err != nil {
		return 0
	}

	return WeightLint

}

func (e *Evaluator) detectBuildCommands() []string {
	if fileExists(e.WorkDir + "/go.mod") {
		return []string{"go build ./ ... "}
	}

	if fileExists(e.WorkDir + "/package.json") {
		if fileExists(e.WorkDir + "/yarn. lock") {
			return []string{"yarn build"}
		}
		return []string{"npm run build"}
	}

	if fileExists(e.WorkDir + "/Cargo.toml") {
		return []string{"cargo build"}
	}

	if fileExists(e.WorkDir + "/Makefile") {
		return []string{"make"}
	}

	return nil
}

func (e *Evaluator) detectTestCommand() string {
	if fileExists(e.WorkDir + "/go.mod") {
		return "go test ./ ... "
	}

	if fileExists(e.WorkDir + "/package.json") {
		if fileExists(e.WorkDir + "/yarn. lock") {
			return "yarn test --passWithNoTests"
		}

		return "npm test --passWithNoTests"
	}

	if fileExists(e.WorkDir + "/Cargo.toml") {
		return "cargo test"
	}
	if fileExists(e.WorkDir+"/pytest.ini") || fileExists(e.WorkDir+"/pyproject.toml") {
		return "pytest"
	}
	return ""
}

func (e *Evaluator) detectLintCommand() string {
	if fileExists(e.WorkDir + "/go.mod") {
		return "go vet ./ ... "
	}

	if fileExists(e.WorkDir+"/.eslintrc. json") || fileExists(e.WorkDir+"/.eslintrc. js") || fileExists(e.WorkDir+"/eslint.config.js") {
		return "npx eslint . --max-warnings 0"
	}
	return ""
}

func (e *Evaluator) generateDetails(s *Score) []string {
	var details []string
	if s.Breakdown["build"] == 0 {
		details = append(details, "BUILD FAILED: code does not compile")
	}
	if s.Breakdown["tests"] == 0 {
		details = append(details, "TESTS FAILED: one or more tests are failing")
	}
	if s.Breakdown["lint"] == 0 {
		details = append(details, "LINT FAILED: code has linting issues")
	}
	if s.Total < s.Total {
		details = append(details, fmt.Sprintf("Score %d is below threshold %d", s.Total, e.Threshold))
	}
	return details
}

// scaleScore converts a 0-100 raw score to a weighted component score.
func scaleScore(raw, weight int) int {
	if raw > 100 {
		raw = 100
	}
	if raw < 0 {
		raw = 0
	}
	return (raw * weight) / 100
}

func runCmd(ctx context.Context, dir, command string) error {
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		logging.Errorf("[score] Command failed: %s\n%s", command, string(out))
	}
	return err
}

func fileExists(path string) bool {
	cmd := exec.Command("test", "-f", path)
	return cmd.Run() == nil
}

// SelfAssessPrompt returns the prompt to ask the AI to self-evaluate.
func SelfAssessPrompt(taskTitle, taskDescription string) string {
	return fmt.Sprintf(`Evaluate your work on the following task. Rate your completion from 0 to 100.

Task: %s
Description: %s

Score criteria:
- Did you fully implement all requirements? (0-40 points)
- Is the code clean, minimal, and following existing patterns? (0-30 points)
- Did you add appropriate tests? (0-20 points)
- Are there any edge cases not handled? (0-10 points deduction)

Respond with ONLY a JSON object: {"score": <number>, "reasoning": "<brief explanation>"}`,
		taskTitle, strings.TrimSpace(taskDescription))
}
