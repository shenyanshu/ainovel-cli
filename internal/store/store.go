package store

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"sync"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/errs"
)

// Store 是状态管理的组合根，持有所有子存储。
type Store struct {
	dir string

	Progress    *ProgressStore
	Outline     *OutlineStore
	Drafts      *DraftStore
	Summaries   *SummaryStore
	RunMeta     *RunMetaStore
	Signals     *SignalStore
	Runtime     *RuntimeStore
	Characters  *CharacterStore
	Cast        *CastStore
	World       *WorldStore
	Checkpoints *CheckpointStore
	Sessions    *SessionStore
	Usage       *UsageStore
	Simulation  *SimulationStore

	crossMu sync.Mutex // 保护跨域原子操作
}

// NewStore 创建状态管理器，dir 为小说输出根目录。
func NewStore(dir string) *Store {
	io := newIO(dir)
	outline := NewOutlineStore(io)
	return &Store{
		dir:         dir,
		Progress:    NewProgressStore(newIO(dir)),
		Outline:     outline,
		Drafts:      NewDraftStore(newIO(dir)),
		Summaries:   NewSummaryStore(newIO(dir), outline),
		RunMeta:     NewRunMetaStore(newIO(dir)),
		Signals:     NewSignalStore(newIO(dir)),
		Runtime:     NewRuntimeStore(newIO(dir)),
		Characters:  NewCharacterStore(newIO(dir), outline),
		Cast:        NewCastStore(newIO(dir)),
		World:       NewWorldStore(newIO(dir)),
		Checkpoints: NewCheckpointStore(io),
		Sessions:    NewSessionStore(newIO(dir)),
		Usage:       NewUsageStore(newIO(dir)),
		Simulation:  NewSimulationStore(newIO(dir)),
	}
}

// Dir 返回输出根目录。
func (s *Store) Dir() string { return s.dir }

// CheckConsistency 对事实层做一次浅层校验，用于启动/恢复时生成 warning。
// 纯只读：不修正数据，仅返回可读的问题描述。调用方决定如何展示（log / UI）。
// 为避免扫全目录带来的 IO 开销，只校验 Progress 的关键点：
//   - 最后一个完成章节必须在 chapters/ 下存在终稿
//   - Layered 模式下，当前 Volume/Arc 必须能在 layered_outline 中找到
func (s *Store) CheckConsistency() []string {
	var warnings []string
	progress, err := s.Progress.Load()
	if err != nil || progress == nil {
		return warnings
	}
	if n := len(progress.CompletedChapters); n > 0 {
		lastCh := progress.CompletedChapters[n-1]
		if text, err := s.Drafts.LoadChapterText(lastCh); err == nil && text == "" {
			warnings = append(warnings, fmt.Sprintf("progress 标记第 %d 章已完成，但 chapters/%02d.md 不存在或为空", lastCh, lastCh))
		}
	}
	if progress.Layered && progress.CurrentVolume > 0 && progress.CurrentArc > 0 {
		volumes, err := s.Outline.LoadLayeredOutline()
		if err == nil && len(volumes) > 0 {
			found := false
			for _, v := range volumes {
				if v.Index != progress.CurrentVolume {
					continue
				}
				for _, a := range v.Arcs {
					if a.Index == progress.CurrentArc {
						found = true
						break
					}
				}
				break
			}
			if !found {
				warnings = append(warnings, fmt.Sprintf("progress 当前 V%d A%d 在分层大纲中找不到对应条目", progress.CurrentVolume, progress.CurrentArc))
			}
		}
	}
	return warnings
}

// FoundationMissing 返回基础设定中尚缺的项，按用于 Prompt/Reminder 的稳定顺序排列。
// 长篇模式（已有 layered_outline）额外要求 compass。
func (s *Store) FoundationMissing() []string {
	var missing []string
	if p, _ := s.Outline.LoadPremise(); p == "" {
		missing = append(missing, "premise")
	}
	if o, _ := s.Outline.LoadOutline(); len(o) == 0 {
		missing = append(missing, "outline")
	}
	if c, _ := s.Characters.Load(); len(c) == 0 {
		missing = append(missing, "characters")
	}
	if r, _ := s.World.LoadWorldRules(); len(r) == 0 {
		missing = append(missing, "world_rules")
	}
	if layered, _ := s.Outline.LoadLayeredOutline(); len(layered) > 0 {
		if c, _ := s.Outline.LoadCompass(); c == nil {
			missing = append(missing, "compass")
		}
	}
	return missing
}

// Init 创建所需的子目录结构。
func (s *Store) Init() error {
	return s.Progress.io.EnsureDirs([]string{
		"chapters", "summaries", "drafts", "reviews", "meta", "meta/runtime", "meta/runtime/tasks", "meta/sessions", "meta/sessions/agents",
	})
}

// ── 跨域协调方法 ──

// ExpandArc 将骨架弧展开为详细章节（Outline + Progress 联动）。
func (s *Store) ExpandArc(volumeIdx, arcIdx int, chapters []domain.OutlineEntry) error {
	s.crossMu.Lock()
	defer s.crossMu.Unlock()

	s.Outline.io.mu.Lock()
	defer s.Outline.io.mu.Unlock()

	volumes, err := s.Outline.expandArcUnlocked(volumeIdx, arcIdx, chapters)
	if err != nil {
		return err
	}

	s.Progress.io.mu.Lock()
	defer s.Progress.io.mu.Unlock()

	p, err := s.Progress.loadUnlocked()
	if err != nil {
		return err
	}
	if p == nil {
		p = &domain.Progress{}
	}
	p.TotalChapters = domain.TotalChapters(volumes)
	return s.Progress.saveUnlocked(p)
}

// AppendVolume 追加新卷到分层大纲末尾（Outline + Progress 联动）。
func (s *Store) AppendVolume(vol domain.VolumeOutline) error {
	s.crossMu.Lock()
	defer s.crossMu.Unlock()

	s.Outline.io.mu.Lock()
	defer s.Outline.io.mu.Unlock()

	volumes, err := s.Outline.appendVolumeUnlocked(vol)
	if err != nil {
		return err
	}

	s.Progress.io.mu.Lock()
	defer s.Progress.io.mu.Unlock()

	p, err := s.Progress.loadUnlocked()
	if err != nil {
		return err
	}
	if p == nil {
		p = &domain.Progress{}
	}
	p.TotalChapters = domain.TotalChapters(volumes)
	return s.Progress.saveUnlocked(p)
}

// ReplanFromChapter 从指定章节开始重规划全书。
// 事务内同时覆盖分层大纲与进度，保证“已完成章节”和“新大纲归属”不会分叉。
func (s *Store) ReplanFromChapter(fromChapter int, volumes []domain.VolumeOutline, reason string) (affected []int, err error) {
	if fromChapter < 1 {
		return nil, fmt.Errorf("fromChapter must be >= 1: %w", errs.ErrToolArgs)
	}

	s.crossMu.Lock()
	defer s.crossMu.Unlock()

	checkpointProgress := mustLoadProgress(s)
	if checkpointProgress != nil {
		label := "replan-from-" + strconv.Itoa(fromChapter)
		ts, err := s.RunMeta.SaveCheckpoint(label, checkpointProgress)
		if err != nil {
			return nil, err
		}
		if err := s.RunMeta.SaveReplanCheckpoint(ts, label); err != nil {
			return nil, err
		}
	}

	if err := s.replanOutlinesUnlocked(fromChapter, volumes, reason, &affected); err != nil {
		return nil, err
	}
	if _, err := s.Checkpoints.AppendArtifact(domain.GlobalScope(), "replan_from_chapter", "layered_outline.json"); err != nil {
		return nil, err
	}
	return affected, nil
}

func (s *Store) replanOutlinesUnlocked(fromChapter int, volumes []domain.VolumeOutline, reason string, affected *[]int) error {
	s.Outline.io.mu.Lock()
	defer s.Outline.io.mu.Unlock()

	s.Progress.io.mu.Lock()
	defer s.Progress.io.mu.Unlock()

	p, err := s.Progress.loadUnlocked()
	if err != nil {
		return err
	}
	if p == nil || p.Phase != domain.PhaseWriting {
		return fmt.Errorf("replan_from_chapter 仅允许在 writing 阶段调用: %w", errs.ErrToolPrecondition)
	}
	if len(p.PendingRewrites) > 0 {
		return fmt.Errorf("已有返工队列，先处理完再 replan: %w", errs.ErrToolPrecondition)
	}
	if p.InProgressChapter > 0 {
		return fmt.Errorf("有未完成章节，先提交或放弃再 replan: %w", errs.ErrToolPrecondition)
	}
	latestCompleted := p.LatestCompleted()
	if total := domain.TotalChapters(volumes); total < latestCompleted {
		return fmt.Errorf("新大纲章数不足以覆盖已完成高水位 %d: %w", latestCompleted, errs.ErrToolPrecondition)
	}

	*affected = make([]int, 0)
	for ch := fromChapter; ch <= latestCompleted; ch++ {
		*affected = append(*affected, ch)
	}
	// flow 合法性在写任何大纲文件之前校验：销毁性操作一旦校验失败必须零文件改动，
	// 否则会留下“大纲已覆盖、progress 未动”的半落盘中间态。
	switchFlow := len(*affected) > 0
	if switchFlow {
		if err := domain.ValidateFlowTransition(p.Flow, domain.FlowRewriting); err != nil {
			return err
		}
	}

	oldLayered, oldFlat, err := s.loadReplanOutlinesUnlocked()
	if err != nil {
		return err
	}
	if err := s.Outline.saveLayeredOutlineUnlocked(volumes); err != nil {
		return err
	}
	restore := func() error { return s.restoreReplanOutlinesUnlocked(oldLayered, oldFlat) }
	if err := s.Outline.saveOutlineUnlocked(domain.FlattenOutline(volumes)); err != nil {
		if restoreErr := restore(); restoreErr != nil {
			return fmt.Errorf("save outline failed and restore failed: %w; restore: %v", err, restoreErr)
		}
		return err
	}

	applyReplanProgress(p, fromChapter, volumes, *affected, reason)
	if switchFlow {
		p.Flow = domain.FlowRewriting
	}
	if err := s.Progress.saveUnlocked(p); err != nil {
		if restoreErr := restore(); restoreErr != nil {
			return fmt.Errorf("save progress failed and restore failed: %w; restore: %v", err, restoreErr)
		}
		return err
	}
	return nil
}

func (s *Store) loadReplanOutlinesUnlocked() ([]domain.VolumeOutline, []domain.OutlineEntry, error) {
	layered, err := s.Outline.loadLayeredOutlineUnlocked()
	if err != nil {
		return nil, nil, err
	}
	flat, err := s.Outline.loadOutlineUnlocked()
	if err != nil {
		return nil, nil, err
	}
	return layered, flat, nil
}

func (s *Store) restoreReplanOutlinesUnlocked(layered []domain.VolumeOutline, flat []domain.OutlineEntry) error {
	if err := s.Outline.saveLayeredOutlineUnlocked(layered); err != nil {
		return err
	}
	return s.Outline.saveOutlineUnlocked(flat)
}

func mustLoadProgress(s *Store) *domain.Progress {
	p, _ := s.Progress.Load()
	return p
}

func applyReplanProgress(p *domain.Progress, fromChapter int, volumes []domain.VolumeOutline, affected []int, reason string) {
	p.TotalChapters = domain.TotalChapters(volumes)
	p.Layered = true
	p.CompletedChapters = filterCompletedBefore(p.CompletedChapters, fromChapter)
	p.StrandHistory = trimHistoryByChapter(p.StrandHistory, fromChapter)
	p.HookHistory = trimHistoryByChapter(p.HookHistory, fromChapter)
	p.ChapterWordCounts = trimChapterWordCounts(p.ChapterWordCounts, fromChapter)
	p.TotalWordCount = sumChapterWordCounts(p.ChapterWordCounts)
	p.InProgressChapter = 0
	p.CompletedScenes = nil
	p.CurrentChapter = fromChapter
	p.PendingRewrites = affected
	p.RewriteReason = reason
	vol, arc, err := locateReplanChapter(volumes, fromChapter)
	if err == nil {
		p.CurrentVolume = vol
		p.CurrentArc = arc
	}
}

func filterCompletedBefore(chapters []int, fromChapter int) []int {
	var kept []int
	for _, ch := range chapters {
		if ch < fromChapter {
			kept = append(kept, ch)
		}
	}
	return kept
}

func trimHistoryByChapter(history []string, fromChapter int) []string {
	if len(history) >= fromChapter {
		return slices.Clone(history[:fromChapter-1])
	}
	return slices.Clone(history)
}

func trimChapterWordCounts(counts map[int]int, fromChapter int) map[int]int {
	if len(counts) == 0 {
		return nil
	}
	kept := make(map[int]int, len(counts))
	for chapter, count := range counts {
		if chapter < fromChapter {
			kept[chapter] = count
		}
	}
	return kept
}

func sumChapterWordCounts(counts map[int]int) int {
	total := 0
	for _, count := range counts {
		total += count
	}
	return total
}

func locateReplanChapter(volumes []domain.VolumeOutline, chapter int) (int, int, error) {
	return locateChapter(volumes, chapter)
}

// ClearHandledSteer 原子性清除 PendingSteer 并重置 FlowSteering 状态
// （RunMeta + Progress 联动）。
func (s *Store) ClearHandledSteer() error {
	s.crossMu.Lock()
	defer s.crossMu.Unlock()

	s.RunMeta.io.mu.Lock()
	defer s.RunMeta.io.mu.Unlock()

	meta, err := s.RunMeta.loadUnlocked()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if meta != nil && meta.PendingSteer != "" {
		meta.PendingSteer = ""
		if err := s.RunMeta.saveUnlocked(*meta); err != nil {
			return err
		}
	}

	s.Progress.io.mu.Lock()
	defer s.Progress.io.mu.Unlock()

	p, err := s.Progress.loadUnlocked()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if p != nil && p.Flow == domain.FlowSteering {
		if err := domain.ValidateFlowTransition(p.Flow, domain.FlowWriting); err != nil {
			return err
		}
		p.Flow = domain.FlowWriting
		if err := s.Progress.saveUnlocked(p); err != nil {
			return err
		}
	}
	return nil
}
