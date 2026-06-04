package bootstrap

import "testing"

// RememberModel 把切换用到的新模型登记进 provider.models，是 TUI 自定义模型持久化的核心。
func TestRememberModel(t *testing.T) {
	cfg := Config{
		Providers: map[string]ProviderConfig{
			"openrouter": {Models: []string{"a"}},
		},
	}

	// 新模型应被追加
	cfg.RememberModel("openrouter", "b")
	if got := cfg.CandidateModels("openrouter"); len(got) != 2 || got[1] != "b" {
		t.Fatalf("expected [a b], got %v", got)
	}

	// 重复登记应幂等
	cfg.RememberModel("openrouter", "b")
	if got := cfg.Providers["openrouter"].Models; len(got) != 2 {
		t.Fatalf("expected dedupe, got %v", got)
	}

	// 未知 provider 应无操作
	cfg.RememberModel("missing", "x")
	if _, ok := cfg.Providers["missing"]; ok {
		t.Fatalf("missing provider should not be created")
	}

	// 空值应无操作
	cfg.RememberModel("openrouter", "  ")
	if got := cfg.Providers["openrouter"].Models; len(got) != 2 {
		t.Fatalf("blank model should be ignored, got %v", got)
	}
}
