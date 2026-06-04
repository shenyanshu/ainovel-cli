package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

func TestSaveFoundationPersistsPlanningTier(t *testing.T) {
	dir := t.TempDir()
	store := store.NewStore(dir)
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	tool := NewSaveFoundationTool(store)
	args, err := json.Marshal(map[string]any{
		"type":    "premise",
		"content": "# 测试书名\n\n## 题材和基调\n测试",
		"scale":   "long",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	meta, err := store.RunMeta.Load()
	if err != nil {
		t.Fatalf("LoadRunMeta: %v", err)
	}
	if meta == nil {
		t.Fatal("expected run meta to exist")
	}
	if meta.PlanningTier != domain.PlanningTierLong {
		t.Fatalf("expected planning tier %q, got %q", domain.PlanningTierLong, meta.PlanningTier)
	}
}

func TestSaveFoundationPremiseSetsNovelName(t *testing.T) {
	dir := t.TempDir()
	store := store.NewStore(dir)
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := store.Progress.Init("novel", 0); err != nil {
		t.Fatalf("Init progress: %v", err)
	}

	tool := NewSaveFoundationTool(store)
	args, err := json.Marshal(map[string]any{
		"type": "premise",
		"content": `# 长夜燃灯

## 题材和基调
东方玄幻，冷硬求生。`,
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	progress, err := store.Progress.Load()
	if err != nil {
		t.Fatalf("LoadProgress: %v", err)
	}
	if progress == nil {
		t.Fatal("expected progress")
	}
	if progress.NovelName != "长夜燃灯" {
		t.Fatalf("expected novel name set, got %q", progress.NovelName)
	}
}

func TestSaveFoundationOutlineClearsLayeredStateWhenDowngrading(t *testing.T) {
	dir := t.TempDir()
	store := store.NewStore(dir)
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := store.Progress.Init("test", 0); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	tool := NewSaveFoundationTool(store)

	layeredArgs, err := json.Marshal(map[string]any{
		"type":    "layered_outline",
		"content": `[{"index":1,"title":"第一卷","theme":"主题","arcs":[{"index":1,"title":"第一弧","goal":"目标","chapters":[{"chapter":1,"title":"第一章","core_event":"开局","hook":"继续"}]}]}]`,
		"scale":   "long",
	})
	if err != nil {
		t.Fatalf("Marshal layered args: %v", err)
	}
	if _, err := tool.Execute(context.Background(), layeredArgs); err != nil {
		t.Fatalf("Execute layered outline: %v", err)
	}

	outlineArgs, err := json.Marshal(map[string]any{
		"type":    "outline",
		"content": `[{"chapter":1,"title":"第一章","core_event":"改为中篇","hook":"继续"}]`,
		"scale":   "mid",
	})
	if err != nil {
		t.Fatalf("Marshal outline args: %v", err)
	}
	if _, err := tool.Execute(context.Background(), outlineArgs); err != nil {
		t.Fatalf("Execute outline: %v", err)
	}

	progress, err := store.Progress.Load()
	if err != nil {
		t.Fatalf("LoadProgress: %v", err)
	}
	if progress == nil {
		t.Fatal("expected progress to exist")
	}
	if progress.Layered {
		t.Fatal("expected layered mode to be disabled")
	}
	if progress.CurrentVolume != 0 || progress.CurrentArc != 0 {
		t.Fatalf("expected volume/arc reset, got volume=%d arc=%d", progress.CurrentVolume, progress.CurrentArc)
	}

	volumes, err := store.Outline.LoadLayeredOutline()
	if err != nil {
		t.Fatalf("LoadLayeredOutline: %v", err)
	}
	if len(volumes) != 0 {
		t.Fatalf("expected layered outline cleared, got %d volumes", len(volumes))
	}

	meta, err := store.RunMeta.Load()
	if err != nil {
		t.Fatalf("LoadRunMeta: %v", err)
	}
	if meta == nil {
		t.Fatal("expected run meta to exist")
	}
	if meta.PlanningTier != domain.PlanningTierMid {
		t.Fatalf("expected planning tier %q, got %q", domain.PlanningTierMid, meta.PlanningTier)
	}
}

func TestSaveFoundationAppendVolume(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 0); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	tool := NewSaveFoundationTool(s)

	// 先创建初始 layered_outline（卷1）
	layeredArgs, _ := json.Marshal(map[string]any{
		"type": "layered_outline",
		"content": []map[string]any{{
			"index": 1, "title": "第一卷", "theme": "起步",
			"arcs": []map[string]any{{
				"index": 1, "title": "首弧", "goal": "目标",
				"chapters": []map[string]any{{"title": "第一章", "core_event": "开局", "hook": "继续"}},
			}},
		}},
		"scale": "long",
	})
	if _, err := tool.Execute(context.Background(), layeredArgs); err != nil {
		t.Fatalf("Execute layered: %v", err)
	}

	// append_volume：追加卷2
	appendArgs, _ := json.Marshal(map[string]any{
		"type": "append_volume",
		"content": map[string]any{
			"index": 2, "title": "第二卷", "theme": "升级",
			"arcs": []map[string]any{{
				"index": 1, "title": "弧一", "goal": "目标",
				"chapters": []map[string]any{{"title": "新章", "core_event": "推进", "hook": "钩子"}},
			}},
		},
	})
	res, err := tool.Execute(context.Background(), appendArgs)
	if err != nil {
		t.Fatalf("Execute append_volume: %v", err)
	}
	var result map[string]any
	json.Unmarshal(res, &result)
	if result["volume"] != float64(2) {
		t.Fatalf("expected volume=2, got %v", result["volume"])
	}

	// 验证大纲有 2 卷
	volumes, _ := s.Outline.LoadLayeredOutline()
	if len(volumes) != 2 {
		t.Fatalf("expected 2 volumes, got %d", len(volumes))
	}
	if volumes[1].Title != "第二卷" {
		t.Fatalf("expected title '第二卷', got %q", volumes[1].Title)
	}
}

func TestSaveFoundationAppendVolumeValidation(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 0); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	tool := NewSaveFoundationTool(s)

	// 初始卷
	layeredArgs, _ := json.Marshal(map[string]any{
		"type": "layered_outline",
		"content": []map[string]any{{
			"index": 1, "title": "第一卷", "theme": "起步",
			"arcs": []map[string]any{{
				"index": 1, "title": "首弧", "goal": "目标",
				"chapters": []map[string]any{{"title": "第一章", "core_event": "开局", "hook": "继续"}},
			}},
		}},
		"scale": "long",
	})
	tool.Execute(context.Background(), layeredArgs)

	// Index 不递增 → 应失败（结构性校验）
	appendArgs, _ := json.Marshal(map[string]any{
		"type": "append_volume",
		"content": map[string]any{
			"index": 1, "title": "重复 Index", "theme": "x",
			"arcs": []map[string]any{{
				"index": 1, "title": "弧一", "goal": "目标",
				"chapters": []map[string]any{{"title": "章", "core_event": "事件", "hook": "钩子"}},
			}},
		},
	})
	_, err := tool.Execute(context.Background(), appendArgs)
	if err == nil {
		t.Fatal("expected error when appending volume with non-increasing index")
	}
}

// TestSaveFoundationAppendVolumeRejectsAfterComplete 验证 Phase=Complete 后不允许 append_volume。
// 取代旧的"Final 卷拒绝追加"语义（Final 字段已删除）。
func TestSaveFoundationAppendVolumeRejectsAfterComplete(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 0); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := s.Progress.MarkComplete(); err != nil {
		t.Fatalf("MarkComplete: %v", err)
	}

	tool := NewSaveFoundationTool(s)
	appendArgs, _ := json.Marshal(map[string]any{
		"type": "append_volume",
		"content": map[string]any{
			"index": 1, "title": "尝试续写", "theme": "x",
			"arcs": []map[string]any{{
				"index": 1, "title": "弧", "goal": "g",
				"chapters": []map[string]any{{"title": "章", "core_event": "e", "hook": "h"}},
			}},
		},
	})
	if _, err := tool.Execute(context.Background(), appendArgs); err == nil {
		t.Fatal("expected error when appending after Phase=Complete")
	}
}

func TestSaveFoundationReplanFromChapterReturnsRealFlow(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 0); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := s.Progress.UpdatePhase(domain.PhaseWriting); err != nil {
		t.Fatalf("UpdatePhase: %v", err)
	}
	if err := s.Progress.Save(&domain.Progress{Phase: domain.PhaseWriting, CompletedChapters: []int{1, 2, 3}}); err != nil {
		t.Fatalf("Save progress: %v", err)
	}
	if err := s.Outline.SaveOutline([]domain.OutlineEntry{{Chapter: 1, Title: "第一章"}, {Chapter: 2, Title: "第二章"}, {Chapter: 3, Title: "第三章"}}); err != nil {
		t.Fatalf("SaveOutline: %v", err)
	}
	if err := s.Outline.SaveLayeredOutline([]domain.VolumeOutline{{Index: 1, Title: "第一卷", Arcs: []domain.ArcOutline{{Index: 1, EstimatedChapters: 3, Chapters: []domain.OutlineEntry{{Chapter: 1, Title: "第一章"}, {Chapter: 2, Title: "第二章"}, {Chapter: 3, Title: "第三章"}}}}}}); err != nil {
		t.Fatalf("SaveLayeredOutline: %v", err)
	}

	tool := NewSaveFoundationTool(s)
	args, err := json.Marshal(map[string]any{
		"type":         "replan_from_chapter",
		"from_chapter": 5,
		"content": []map[string]any{{
			"index": 1, "title": "第一卷", "theme": "起步",
			"arcs": []map[string]any{{
				"index": 1, "title": "首弧", "goal": "目标",
				"chapters": []map[string]any{{"chapter": 1, "title": "第一章", "core_event": "开局", "hook": "继续"}, {"chapter": 2, "title": "第二章", "core_event": "推进", "hook": "继续"}, {"chapter": 3, "title": "第三章", "core_event": "收束", "hook": "继续"}},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(res, &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if result["flow"] == string(domain.FlowRewriting) {
		t.Fatalf("expected real flow instead of rewriting, got %v", result["flow"])
	}
	if rewrites, ok := result["pending_rewrites"].([]any); ok && len(rewrites) != 0 {
		t.Fatalf("expected empty pending rewrites, got %v", rewrites)
	}
}

func TestSaveFoundationReplanFromChapterSetsRewritingFlow(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 0); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := s.Progress.UpdatePhase(domain.PhaseWriting); err != nil {
		t.Fatalf("UpdatePhase: %v", err)
	}
	if err := s.Progress.Save(&domain.Progress{Phase: domain.PhaseWriting, CompletedChapters: []int{1, 2}}); err != nil {
		t.Fatalf("Save progress: %v", err)
	}
	if err := s.Outline.SaveOutline([]domain.OutlineEntry{{Chapter: 1, Title: "第一章"}, {Chapter: 2, Title: "第二章"}}); err != nil {
		t.Fatalf("SaveOutline: %v", err)
	}
	if err := s.Outline.SaveLayeredOutline([]domain.VolumeOutline{{Index: 1, Title: "第一卷", Arcs: []domain.ArcOutline{{Index: 1, EstimatedChapters: 2, Chapters: []domain.OutlineEntry{{Chapter: 1, Title: "第一章"}, {Chapter: 2, Title: "第二章"}}}}}}); err != nil {
		t.Fatalf("SaveLayeredOutline: %v", err)
	}

	tool := NewSaveFoundationTool(s)
	args, err := json.Marshal(map[string]any{
		"type":         "replan_from_chapter",
		"from_chapter": 1,
		"content": []map[string]any{{
			"index": 1, "title": "第一卷", "theme": "起步",
			"arcs": []map[string]any{{
				"index": 1, "title": "首弧", "goal": "目标",
				"chapters": []map[string]any{{"chapter": 1, "title": "第一章", "core_event": "开局", "hook": "继续"}, {"chapter": 2, "title": "第二章", "core_event": "推进", "hook": "继续"}},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(res, &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if result["flow"] != string(domain.FlowRewriting) {
		t.Fatalf("expected rewriting flow, got %v", result["flow"])
	}
	if rewrites, ok := result["pending_rewrites"].([]any); !ok || len(rewrites) == 0 {
		t.Fatalf("expected pending rewrites, got %v", result["pending_rewrites"])
	}
}

func TestSaveFoundationUpdateCompass(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	tool := NewSaveFoundationTool(s)
	args, _ := json.Marshal(map[string]any{
		"type": "update_compass",
		"content": map[string]any{
			"ending_direction": "主角面对最终抉择",
			"open_threads":     []string{"线索A", "关系B"},
			"estimated_scale":  "预计 4-6 卷",
		},
	})
	_, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute update_compass: %v", err)
	}

	compass, err := s.Outline.LoadCompass()
	if err != nil {
		t.Fatalf("LoadCompass: %v", err)
	}
	if compass == nil || compass.EndingDirection != "主角面对最终抉择" {
		t.Fatalf("unexpected compass: %+v", compass)
	}
	if len(compass.OpenThreads) != 2 {
		t.Fatalf("expected 2 open threads, got %d", len(compass.OpenThreads))
	}
}

func TestSaveFoundationUpdateCompassOverridesLastUpdated(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Save(&domain.Progress{
		NovelName:         "光斑",
		Phase:             domain.PhaseWriting,
		CompletedChapters: []int{1, 2, 3, 5, 4}, // 乱序，验证取 max 而非 len
	}); err != nil {
		t.Fatalf("Save progress: %v", err)
	}

	tool := NewSaveFoundationTool(s)
	args, _ := json.Marshal(map[string]any{
		"type": "update_compass",
		"content": map[string]any{
			"ending_direction": "主角面对最终抉择",
			"open_threads":     []string{"线索A"},
			"last_updated":     0, // LLM 通常忘填或留 0
		},
	})
	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute update_compass: %v", err)
	}

	compass, err := s.Outline.LoadCompass()
	if err != nil {
		t.Fatalf("LoadCompass: %v", err)
	}
	if compass.LastUpdated != 5 {
		t.Fatalf("expected LastUpdated=5 (max of CompletedChapters), got %d", compass.LastUpdated)
	}
}

func TestSaveFoundationReplanFromChapter(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 8); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	progress, err := s.Progress.Load()
	if err != nil {
		t.Fatalf("LoadProgress: %v", err)
	}
	progress.Phase = domain.PhaseWriting
	progress.Flow = domain.FlowWriting
	progress.CompletedChapters = []int{1, 2, 3, 4, 5, 6, 7, 8}
	progress.CurrentChapter = 9
	progress.TotalChapters = 8
	progress.TotalWordCount = 8000
	progress.ChapterWordCounts = map[int]int{1: 1000, 2: 1000, 3: 1000, 4: 1000, 5: 1000, 6: 1000, 7: 1000, 8: 1000}
	progress.StrandHistory = []string{"s1", "s2", "s3", "s4", "s5", "s6", "s7", "s8"}
	progress.HookHistory = []string{"h1", "h2", "h3", "h4", "h5", "h6", "h7", "h8"}
	if err := s.Progress.Save(progress); err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}
	if err := s.Outline.SaveLayeredOutline(testReplanVolumes(8)); err != nil {
		t.Fatalf("SaveLayeredOutline: %v", err)
	}
	if err := s.Outline.SaveOutline(domain.FlattenOutline(testReplanVolumes(8))); err != nil {
		t.Fatalf("SaveOutline: %v", err)
	}

	tool := NewSaveFoundationTool(s)
	args, err := json.Marshal(map[string]any{
		"type":         "replan_from_chapter",
		"content":      testReplanVolumes(8),
		"from_chapter": 1,
		"reason":       "整体重构",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(res, &result); err != nil {
		t.Fatalf("Unmarshal result: %v", err)
	}
	if got := result["from_chapter"]; got != float64(1) {
		t.Fatalf("expected from_chapter=1, got %v", got)
	}
	if got := result["flow"]; got != string(domain.FlowRewriting) {
		t.Fatalf("expected flow rewriting, got %v", got)
	}

	updated, err := s.Progress.Load()
	if err != nil {
		t.Fatalf("LoadProgress2: %v", err)
	}
	if updated.Flow != domain.FlowRewriting {
		t.Fatalf("expected rewriting flow, got %q", updated.Flow)
	}
	if len(updated.PendingRewrites) != 8 || updated.PendingRewrites[0] != 1 || updated.PendingRewrites[7] != 8 {
		t.Fatalf("unexpected pending rewrites: %v", updated.PendingRewrites)
	}
	if len(updated.CompletedChapters) != 0 {
		t.Fatalf("expected completed chapters cleared, got %v", updated.CompletedChapters)
	}
	if updated.CurrentChapter != 1 {
		t.Fatalf("expected current chapter 1, got %d", updated.CurrentChapter)
	}
	if updated.TotalChapters != 8 {
		t.Fatalf("expected total chapters 8, got %d", updated.TotalChapters)
	}
	if updated.TotalWordCount != 0 {
		t.Fatalf("expected total word count 0, got %d", updated.TotalWordCount)
	}
}

func TestSaveFoundationReplanFromChapterTrimsHistory(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 8); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	progress, _ := s.Progress.Load()
	progress.Phase = domain.PhaseWriting
	progress.Flow = domain.FlowWriting
	progress.CompletedChapters = []int{1, 2, 3, 4, 5, 6, 7, 8}
	progress.CurrentChapter = 9
	progress.ChapterWordCounts = map[int]int{1: 100, 2: 200, 3: 300, 4: 400, 5: 500, 6: 600, 7: 700, 8: 800}
	progress.TotalWordCount = 3600
	progress.StrandHistory = []string{"s1", "s2", "s3", "s4", "s5", "s6", "s7", "s8"}
	progress.HookHistory = []string{"h1", "h2", "h3", "h4", "h5", "h6", "h7", "h8"}
	if err := s.Progress.Save(progress); err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}
	if err := s.Outline.SaveLayeredOutline(testReplanVolumes(8)); err != nil {
		t.Fatalf("SaveLayeredOutline: %v", err)
	}
	if err := s.Outline.SaveOutline(domain.FlattenOutline(testReplanVolumes(8))); err != nil {
		t.Fatalf("SaveOutline: %v", err)
	}

	tool := NewSaveFoundationTool(s)
	args, _ := json.Marshal(map[string]any{"type": "replan_from_chapter", "content": testReplanVolumes(8), "from_chapter": 5})
	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	updated, _ := s.Progress.Load()
	if len(updated.PendingRewrites) != 4 || updated.PendingRewrites[0] != 5 || updated.PendingRewrites[3] != 8 {
		t.Fatalf("unexpected pending rewrites: %v", updated.PendingRewrites)
	}
	if got := updated.CompletedChapters; len(got) != 4 || got[0] != 1 || got[3] != 4 {
		t.Fatalf("unexpected completed chapters: %v", got)
	}
	if len(updated.ChapterWordCounts) != 4 || updated.TotalWordCount != 1000 {
		t.Fatalf("unexpected word counts: %+v total=%d", updated.ChapterWordCounts, updated.TotalWordCount)
	}
	if len(updated.StrandHistory) != 4 || len(updated.HookHistory) != 4 {
		t.Fatalf("unexpected history trim: strand=%v hook=%v", updated.StrandHistory, updated.HookHistory)
	}
}

// progress 未初始化（init/premise/outline 等非 writing/complete 阶段）时 replan 应被拒。
// 注意 complete 阶段是允许的（解冻路径，见 UnfreezesCompleted），这里覆盖的是另一侧边界。
func TestSaveFoundationReplanFromChapterRejectsNonWritingPhase(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 8); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	progress, _ := s.Progress.Load()
	progress.Phase = domain.PhaseOutline
	if err := s.Progress.Save(progress); err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}

	tool := NewSaveFoundationTool(s)
	args, _ := json.Marshal(map[string]any{"type": "replan_from_chapter", "content": testReplanVolumes(8), "from_chapter": 1})
	if _, err := tool.Execute(context.Background(), args); err == nil {
		t.Fatal("expected error when replan in non-writing/non-complete phase")
	}
	after, _ := s.Progress.Load()
	if after.Phase != domain.PhaseOutline {
		t.Fatalf("expected progress unchanged, got phase=%q", after.Phase)
	}
}

func TestSaveFoundationReplanFromChapterRejectedByPendingQueue(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 8); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	progress, _ := s.Progress.Load()
	progress.Phase = domain.PhaseWriting
	progress.Flow = domain.FlowWriting
	progress.PendingRewrites = []int{2}
	if err := s.Progress.Save(progress); err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}

	tool := NewSaveFoundationTool(s)
	args, _ := json.Marshal(map[string]any{"type": "replan_from_chapter", "content": testReplanVolumes(8), "from_chapter": 1})
	if _, err := tool.Execute(context.Background(), args); err == nil {
		t.Fatal("expected error when pending rewrites exist")
	}
}

// 死锁回归：用户正写着第 5 章（in-progress）时发现要从头重写，replan 必须放行，
// 而不是被"有未完成章节"挡死。这是线上报的核心 bug。
func TestSaveFoundationReplanFromChapterAllowsInProgressWithinRange(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 8); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	progress, _ := s.Progress.Load()
	progress.Phase = domain.PhaseWriting
	progress.Flow = domain.FlowWriting
	progress.CompletedChapters = []int{1, 2, 3, 4}
	progress.InProgressChapter = 5
	if err := s.Progress.Save(progress); err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}

	tool := NewSaveFoundationTool(s)
	args, _ := json.Marshal(map[string]any{"type": "replan_from_chapter", "content": testReplanVolumes(8), "from_chapter": 1})
	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("expected replan to succeed with in-progress chapter, got: %v", err)
	}
	after, _ := s.Progress.Load()
	if after.InProgressChapter != 0 {
		t.Fatalf("expected in-progress cleared, got %d", after.InProgressChapter)
	}
	if after.Flow != domain.FlowRewriting {
		t.Fatalf("expected rewriting flow, got %s", after.Flow)
	}
	if len(after.PendingRewrites) != 4 {
		t.Fatalf("expected 4 pending rewrites, got %v", after.PendingRewrites)
	}
}

// 边界：正写第 5 章且 replan 起点正好是第 5 章（latestCompleted=4）。affected 为空、
// 不切 rewriting，但 in-progress 被清空，下一步由 Router 从第 5 章全新重写。
func TestSaveFoundationReplanFromChapterInProgressEqualsStart(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 8); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	progress, _ := s.Progress.Load()
	progress.Phase = domain.PhaseWriting
	progress.Flow = domain.FlowWriting
	progress.CompletedChapters = []int{1, 2, 3, 4}
	progress.InProgressChapter = 5
	if err := s.Progress.Save(progress); err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}

	tool := NewSaveFoundationTool(s)
	args, _ := json.Marshal(map[string]any{"type": "replan_from_chapter", "content": testReplanVolumes(8), "from_chapter": 5})
	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("expected replan to succeed, got: %v", err)
	}
	after, _ := s.Progress.Load()
	if after.InProgressChapter != 0 {
		t.Fatalf("expected in-progress cleared, got %d", after.InProgressChapter)
	}
	if len(after.PendingRewrites) != 0 {
		t.Fatalf("expected no pending rewrites, got %v", after.PendingRewrites)
	}
	if after.NextChapter() != 5 {
		t.Fatalf("expected next chapter 5, got %d", after.NextChapter())
	}
}

// 拒绝：正写第 3 章却想从第 5 章起 replan，会丢弃未被覆盖的第 3 章半成品，
// 在新大纲里留下"存在却永不重写"的空洞，必须拒绝且 progress 不变。
func TestSaveFoundationReplanFromChapterRejectsStartAfterInProgress(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 8); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	progress, _ := s.Progress.Load()
	progress.Phase = domain.PhaseWriting
	progress.Flow = domain.FlowWriting
	progress.CompletedChapters = []int{1, 2}
	progress.InProgressChapter = 3
	if err := s.Progress.Save(progress); err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}

	tool := NewSaveFoundationTool(s)
	args, _ := json.Marshal(map[string]any{"type": "replan_from_chapter", "content": testReplanVolumes(8), "from_chapter": 5})
	if _, err := tool.Execute(context.Background(), args); err == nil {
		t.Fatal("expected error when replan starts after in-progress chapter")
	}
	after, _ := s.Progress.Load()
	if after.InProgressChapter != 3 || len(after.PendingRewrites) != 0 || after.Flow != domain.FlowWriting {
		t.Fatalf("expected progress unchanged, got %+v", after)
	}
}

// 解冻：书已被（误）标完结后，用户要全书重写。replan 必须能在 complete 阶段执行，
// 并把 Phase 复位回 writing——否则误标完结会导致全书永久冻结、无法补救。
func TestSaveFoundationReplanFromChapterUnfreezesCompleted(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 8); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	progress, _ := s.Progress.Load()
	progress.Phase = domain.PhaseComplete
	progress.Flow = domain.FlowWriting
	progress.CompletedChapters = []int{1, 2, 3, 4, 5, 6, 7, 8}
	if err := s.Progress.Save(progress); err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}

	tool := NewSaveFoundationTool(s)
	args, _ := json.Marshal(map[string]any{"type": "replan_from_chapter", "content": testReplanVolumes(8), "from_chapter": 1})
	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("expected replan to unfreeze completed book, got: %v", err)
	}
	after, _ := s.Progress.Load()
	if after.Phase != domain.PhaseWriting {
		t.Fatalf("expected phase reset to writing, got %s", after.Phase)
	}
	if after.Flow != domain.FlowRewriting {
		t.Fatalf("expected rewriting flow, got %s", after.Flow)
	}
	if len(after.PendingRewrites) != 8 {
		t.Fatalf("expected 8 pending rewrites, got %v", after.PendingRewrites)
	}
}

func TestSaveFoundationReplanFromChapterCapacityValidation(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 8); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	progress, _ := s.Progress.Load()
	progress.Phase = domain.PhaseWriting
	progress.Flow = domain.FlowWriting
	progress.CompletedChapters = []int{1, 2, 3, 4, 5, 6, 7, 8}
	if err := s.Progress.Save(progress); err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}

	tool := NewSaveFoundationTool(s)
	args, _ := json.Marshal(map[string]any{"type": "replan_from_chapter", "content": testReplanVolumes(4), "from_chapter": 1})
	if _, err := tool.Execute(context.Background(), args); err == nil {
		t.Fatal("expected error when new outline cannot cover completed high water mark")
	}
	after, _ := s.Progress.Load()
	if len(after.PendingRewrites) != 0 || after.Flow != domain.FlowWriting {
		t.Fatalf("expected progress unchanged, got %+v", after)
	}
}

func TestSaveFoundationReplanFromChapterKeepsLaterOutlineChanges(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 8); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	progress, _ := s.Progress.Load()
	progress.Phase = domain.PhaseWriting
	progress.Flow = domain.FlowWriting
	progress.CompletedChapters = []int{1, 2, 3, 4, 5, 6, 7, 8}
	progress.CurrentChapter = 9
	if err := s.Progress.Save(progress); err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}
	volumes := testReplanVolumes(10)
	if err := s.Outline.SaveLayeredOutline(volumes); err != nil {
		t.Fatalf("SaveLayeredOutline: %v", err)
	}
	if err := s.Outline.SaveOutline(domain.FlattenOutline(volumes)); err != nil {
		t.Fatalf("SaveOutline: %v", err)
	}
	tool := NewSaveFoundationTool(s)
	args, _ := json.Marshal(map[string]any{"type": "replan_from_chapter", "content": volumes, "from_chapter": 5})
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var result map[string]any
	_ = json.Unmarshal(res, &result)
	if got := result["affected_chapters"]; got == nil {
		t.Fatal("expected affected chapters in result")
	}
}

func testReplanVolumes(totalChapters int) []domain.VolumeOutline {
	chapters := make([]domain.OutlineEntry, 0, totalChapters)
	for i := 1; i <= totalChapters; i++ {
		chapters = append(chapters, domain.OutlineEntry{Title: fmt.Sprintf("第%d章", i), CoreEvent: "事件"})
	}
	return []domain.VolumeOutline{{Index: 1, Title: "第一卷", Theme: "主题", Arcs: []domain.ArcOutline{{Index: 1, Title: "首弧", Goal: "目标", Chapters: chapters}}}}
}

func TestSaveFoundationUpdateCompassRequiresDirection(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	tool := NewSaveFoundationTool(s)
	args, _ := json.Marshal(map[string]any{
		"type":    "update_compass",
		"content": map[string]any{"estimated_scale": "3 卷"},
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error when ending_direction is empty")
	}
}

func TestSaveFoundationAcceptsDirectJSONArrayContent(t *testing.T) {
	dir := t.TempDir()
	store := store.NewStore(dir)
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	tool := NewSaveFoundationTool(store)
	args, err := json.Marshal(map[string]any{
		"type": "outline",
		"content": []map[string]any{
			{
				"chapter":    1,
				"title":      "第一章",
				"core_event": "主角登场",
				"hook":       "继续",
				"scenes":     []string{"场景一", "场景二"},
			},
		},
		"scale": "short",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	outline, err := store.Outline.LoadOutline()
	if err != nil {
		t.Fatalf("LoadOutline: %v", err)
	}
	if len(outline) != 1 || outline[0].Title != "第一章" {
		t.Fatalf("unexpected outline: %+v", outline)
	}
}

// completeBookSetup 建一份处于 writing 阶段的最小 Store，用于 complete_book 系列测试。
// complete_book 不校验 layered_outline 章节齐全（判定责任在 LLM 的"完结判定清单"），
// 工具层只校验 PendingRewrites 为空、progress 已初始化。
func completeBookSetup(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 0); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	_ = s.Progress.UpdatePhase(domain.PhaseWriting)
	return s
}

func TestSaveFoundationCompleteBookPushesPhaseComplete(t *testing.T) {
	s := completeBookSetup(t)
	tool := NewSaveFoundationTool(s)
	args, _ := json.Marshal(map[string]any{
		"type": "complete_book", "content": map[string]any{},
	})
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute complete_book: %v", err)
	}
	var result map[string]any
	_ = json.Unmarshal(res, &result)
	if result["book_complete"] != true {
		t.Fatalf("expected book_complete=true, got %+v", result)
	}
	if result["phase"] != string(domain.PhaseComplete) {
		t.Fatalf("expected phase=complete, got %v", result["phase"])
	}
	progress, _ := s.Progress.Load()
	if progress.Phase != domain.PhaseComplete {
		t.Fatalf("expected progress.Phase=complete, got %s", progress.Phase)
	}
}

func TestSaveFoundationCompleteBookRejectsBeforeWriting(t *testing.T) {
	// 规划阶段误调 complete_book 必须被拒，否则会直接跳过整本写作。
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 0); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	_ = s.Progress.UpdatePhase(domain.PhasePremise)
	_ = s.Progress.UpdatePhase(domain.PhaseOutline)
	tool := NewSaveFoundationTool(s)
	args, _ := json.Marshal(map[string]any{
		"type": "complete_book", "content": map[string]any{},
	})
	if _, err := tool.Execute(context.Background(), args); err == nil {
		t.Fatal("expected error when phase != writing")
	}
	progress, _ := s.Progress.Load()
	if progress.Phase != domain.PhaseOutline {
		t.Fatalf("phase should remain outline, got %s", progress.Phase)
	}
}

func TestSaveFoundationCompleteBookRejectsWithPendingRewrites(t *testing.T) {
	s := completeBookSetup(t)
	if err := s.Progress.SetPendingRewrites([]int{2}, "尾章节奏过快"); err != nil {
		t.Fatalf("SetPendingRewrites: %v", err)
	}
	tool := NewSaveFoundationTool(s)
	args, _ := json.Marshal(map[string]any{
		"type": "complete_book", "content": map[string]any{},
	})
	if _, err := tool.Execute(context.Background(), args); err == nil {
		t.Fatal("expected error when PendingRewrites non-empty")
	}
	progress, _ := s.Progress.Load()
	if progress.Phase == domain.PhaseComplete {
		t.Fatalf("phase should not be Complete with PendingRewrites: %s", progress.Phase)
	}
}
