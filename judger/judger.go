package judger

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Result 评判结果
type Result struct {
	Score       int               `json:"score"`
	Passed      bool              `json:"passed"`
	Feedback    string            `json:"feedback"`
	Details     map[string]interface{} `json:"details"`
	JudgedAt    time.Time         `json:"judged_at"`
	Duration    time.Duration     `json:"duration"`
}

// Criteria 评判标准
type Criteria struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Weight      float64 `json:"weight"`
	MaxScore    int     `json:"max_score"`
}

// Request 评判请求
type Request struct {
	TaskID      string                 `json:"task_id"`
	UserID      string                 `json:"user_id"`
	Input       string                 `json:"input"`
	Expected    string                 `json:"expected"`
	Actual      string                 `json:"actual"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// Judger 评判器接口
type Judger interface {
	Judge(ctx context.Context, req *Request) (*Result, error)
	Validate(ctx context.Context, req *Request) error
}

// BaseJudger 基础评判器
type BaseJudger struct {
	criteria []Criteria
}

// NewBaseJudger 创建基础评判器
func NewBaseJudger() *BaseJudger {
	return &BaseJudger{
		criteria: make([]Criteria, 0),
	}
}

// AddCriteria 添加评判标准
func (j *BaseJudger) AddCriteria(c Criteria) {
	j.criteria = append(j.criteria, c)
}

// GetCriteria 获取所有评判标准
func (j *BaseJudger) GetCriteria() []Criteria {
	return j.criteria
}

// Validate 验证请求
func (j *BaseJudger) Validate(ctx context.Context, req *Request) error {
	if req.TaskID == "" {
		return fmt.Errorf("task_id is required")
	}
	if req.UserID == "" {
		return fmt.Errorf("user_id is required")
	}
	if req.Actual == "" {
		return fmt.Errorf("actual result is required")
	}
	return nil
}

// ExactMatchJudger 精确匹配评判器
type ExactMatchJudger struct {
	*BaseJudger
	caseSensitive bool
}

// NewExactMatchJudger 创建精确匹配评判器
func NewExactMatchJudger(caseSensitive bool) *ExactMatchJudger {
	return &ExactMatchJudger{
		BaseJudger:    NewBaseJudger(),
		caseSensitive: caseSensitive,
	}
}

// Judge 执行精确匹配评判
func (j *ExactMatchJudger) Judge(ctx context.Context, req *Request) (*Result, error) {
	start := time.Now()

	if err := j.Validate(ctx, req); err != nil {
		return nil, err
	}

	expected := req.Expected
	actual := req.Actual

	if !j.caseSensitive {
		expected = stringToLower(expected)
		actual = stringToLower(actual)
	}

	passed := expected == actual
	score := 0
	if passed {
		score = 100
	}

	feedback := "结果不匹配"
	if passed {
		feedback = "完全匹配"
	}

	return &Result{
		Score:    score,
		Passed:   passed,
		Feedback: feedback,
		Details: map[string]interface{}{
			"expected":       req.Expected,
			"actual":         req.Actual,
			"case_sensitive": j.caseSensitive,
		},
		JudgedAt: time.Now(),
		Duration: time.Since(start),
	}, nil
}

// SimilarityJudger 相似度评判器
type SimilarityJudger struct {
	*BaseJudger
	threshold float64
}

// NewSimilarityJudger 创建相似度评判器
func NewSimilarityJudger(threshold float64) *SimilarityJudger {
	if threshold <= 0 || threshold > 1 {
		threshold = 0.8
	}
	return &SimilarityJudger{
		BaseJudger: NewBaseJudger(),
		threshold:  threshold,
	}
}

// Judge 执行相似度评判
func (j *SimilarityJudger) Judge(ctx context.Context, req *Request) (*Result, error) {
	start := time.Now()

	if err := j.Validate(ctx, req); err != nil {
		return nil, err
	}

	similarity := calculateSimilarity(req.Expected, req.Actual)
	score := int(similarity * 100)
	passed := similarity >= j.threshold

	feedback := fmt.Sprintf("相似度: %.2f%%", similarity*100)
	if passed {
		feedback += "，通过"
	} else {
		feedback += "，未达到阈值"
	}

	return &Result{
		Score:    score,
		Passed:   passed,
		Feedback: feedback,
		Details: map[string]interface{}{
			"expected":       req.Expected,
			"actual":         req.Actual,
			"similarity":     similarity,
			"threshold":      j.threshold,
		},
		JudgedAt: time.Now(),
		Duration: time.Since(start),
	}, nil
}

// LLMJudgeResult LLM评判结果格式
type LLMJudgeResult struct {
	Score    int    `json:"score"`
	Passed   bool   `json:"passed"`
	Feedback string `json:"feedback"`
	Reason   string `json:"reason"`
}

// LLMJudger LLM评判器 - 使用模型进行评判
type LLMJudger struct {
	*BaseJudger
	client LLMClient
	prompt string
}

// LLMClient LLM客户端接口
type LLMClient interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// NewLLMJudger 创建LLM评判器
func NewLLMJudger(client LLMClient) *LLMJudger {
	return &LLMJudger{
		BaseJudger: NewBaseJudger(),
		client:     client,
		prompt:     defaultLLMJudgePrompt(),
	}
}

// SetPrompt 设置自定义评判prompt
func (j *LLMJudger) SetPrompt(prompt string) {
	j.prompt = prompt
}

// Judge 使用LLM进行评判
func (j *LLMJudger) Judge(ctx context.Context, req *Request) (*Result, error) {
	start := time.Now()

	if err := j.Validate(ctx, req); err != nil {
		return nil, err
	}

	prompt := fmt.Sprintf(j.prompt, req.Expected, req.Actual, req.Input)

	response, err := j.client.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("llm judge failed: %w", err)
	}

	var llmResult LLMJudgeResult
	if err := json.Unmarshal([]byte(response), &llmResult); err != nil {
		// 如果解析失败，使用文本结果
		llmResult = LLMJudgeResult{
			Score:    0,
			Passed:   false,
			Feedback: response,
			Reason:   "解析失败",
		}
	}

	return &Result{
		Score:    llmResult.Score,
		Passed:   llmResult.Passed,
		Feedback: llmResult.Feedback,
		Details: map[string]interface{}{
			"expected": req.Expected,
			"actual":   req.Actual,
			"reason":   llmResult.Reason,
			"raw_response": response,
		},
		JudgedAt: time.Now(),
		Duration: time.Since(start),
	}, nil
}

// CompositeJudger 组合评判器 - 组合多个评判器
type CompositeJudger struct {
	*BaseJudger
	judgers []Judger
	weights []float64
}

// NewCompositeJudger 创建组合评判器
func NewCompositeJudger() *CompositeJudger {
	return &CompositeJudger{
		BaseJudger: NewBaseJudger(),
		judgers:    make([]Judger, 0),
		weights:    make([]float64, 0),
	}
}

// AddJudger 添加评判器
func (j *CompositeJudger) AddJudger(judger Judger, weight float64) {
	j.judgers = append(j.judgers, judger)
	j.weights = append(j.weights, weight)
}

// Judge 执行组合评判
func (j *CompositeJudger) Judge(ctx context.Context, req *Request) (*Result, error) {
	start := time.Now()

	if len(j.judgers) == 0 {
		return nil, fmt.Errorf("no judgers configured")
	}

	totalScore := 0.0
	totalWeight := 0.0
	allPassed := true
	feedbacks := make([]string, 0)
	details := make(map[string]interface{})

	for i, judger := range j.judgers {
		result, err := judger.Judge(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("judger %d failed: %w", i, err)
		}

		weight := j.weights[i]
		totalScore += float64(result.Score) * weight
		totalWeight += weight
		allPassed = allPassed && result.Passed
		feedbacks = append(feedbacks, result.Feedback)
		details[fmt.Sprintf("judger_%d", i)] = result.Details
	}

	finalScore := int(totalScore / totalWeight)

	return &Result{
		Score:    finalScore,
		Passed:   allPassed,
		Feedback: fmt.Sprintf("组合评判完成，共%d个评判器", len(j.judgers)),
		Details:  details,
		JudgedAt: time.Now(),
		Duration: time.Since(start),
	}, nil
}

// Helper functions

func stringToLower(s string) string {
	result := make([]rune, len(s))
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			result[i] = r + ('a' - 'A')
		} else {
			result[i] = r
		}
	}
	return string(result)
}

// calculateSimilarity 计算字符串相似度 (Levenshtein距离)
func calculateSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}
	if len(s1) == 0 || len(s2) == 0 {
		return 0.0
	}

	dist := levenshteinDistance(s1, s2)
	maxLen := len(s1)
	if len(s2) > maxLen {
		maxLen = len(s2)
	}

	return 1.0 - float64(dist)/float64(maxLen)
}

func levenshteinDistance(s1, s2 string) int {
	r1 := []rune(s1)
	r2 := []rune(s2)

	m, n := len(r1), len(r2)
	if m == 0 {
		return n
	}
	if n == 0 {
		return m
	}

	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

	for i := 0; i <= m; i++ {
		dp[i][0] = i
	}
	for j := 0; j <= n; j++ {
		dp[0][j] = j
	}

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			cost := 0
			if r1[i-1] != r2[j-1] {
				cost = 1
			}
			dp[i][j] = min(dp[i-1][j]+1, min(dp[i][j-1]+1, dp[i-1][j-1]+cost))
		}
	}

	return dp[m][n]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func defaultLLMJudgePrompt() string {
	return `请评判以下结果是否符合预期。

输入:
%s

预期输出:
%s

实际输出:
%s

请以JSON格式返回评判结果，包含以下字段:
- score: 分数(0-100)
- passed: 是否通过(boolean)
- feedback: 评价反馈
- reason: 评分理由
`
}
