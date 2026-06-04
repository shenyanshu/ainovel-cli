package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/voocel/ainovel-cli/internal/host"
	"github.com/voocel/ainovel-cli/internal/utils"
)

// customModelOption 是模型候选列表末尾的“自定义”哨兵项标签。
// 选中它后允许直接输入一个未登记的新模型名，解决新 provider 候选为空时无法分配模型的问题。
const customModelOption = "+ 自定义…"

type modelSwitchFocus int

const (
	modelFocusRole modelSwitchFocus = iota
	modelFocusProvider
	modelFocusModel
)

type modelRoleOption struct {
	Key   string
	Label string
}

var modelRoleOptions = []modelRoleOption{
	{Key: "default", Label: "默认"},
	{Key: "coordinator", Label: "Coordinator"},
	{Key: "architect", Label: "Architect"},
	{Key: "writer", Label: "Writer"},
	{Key: "editor", Label: "Editor"},
}

type modelSwitchState struct {
	focus       modelSwitchFocus
	roleIdx     int
	providerIdx int
	modelIdx    int
	providers   []string
	models      []string
	message     string
	typing      bool   // 是否处于自定义模型名输入态
	customInput string // 自定义模型名输入缓冲
}

func newModelSwitchState(rt *host.Host, roleHint string) *modelSwitchState {
	state := &modelSwitchState{
		providers: rt.ConfiguredProviders(),
	}
	if len(state.providers) == 0 {
		state.message = "当前没有可用 provider"
	}

	roleHint = normalizeRoleKey(roleHint)
	for i, opt := range modelRoleOptions {
		if opt.Key == roleHint {
			state.roleIdx = i
			break
		}
	}
	state.syncSelection(rt)
	return state
}

func normalizeRoleKey(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "", "default":
		return "default"
	case "coordinator", "architect", "writer", "editor":
		return strings.ToLower(strings.TrimSpace(role))
	default:
		return ""
	}
}

func (s *modelSwitchState) role() string {
	return modelRoleOptions[s.roleIdx].Key
}

func (s *modelSwitchState) roleLabel() string {
	return modelRoleOptions[s.roleIdx].Label
}

func (s *modelSwitchState) provider() string {
	if len(s.providers) == 0 || s.providerIdx < 0 || s.providerIdx >= len(s.providers) {
		return ""
	}
	return s.providers[s.providerIdx]
}

func (s *modelSwitchState) model() string {
	if len(s.models) == 0 || s.modelIdx < 0 || s.modelIdx >= len(s.models) {
		return ""
	}
	return s.models[s.modelIdx]
}

func (s *modelSwitchState) moveFocus(delta int) {
	total := 3
	s.focus = modelSwitchFocus((int(s.focus) + delta + total) % total)
}

func (s *modelSwitchState) cycle(delta int, rt *host.Host) {
	switch s.focus {
	case modelFocusRole:
		total := len(modelRoleOptions)
		s.roleIdx = (s.roleIdx + delta + total) % total
		s.syncSelection(rt)
	case modelFocusProvider:
		if len(s.providers) == 0 {
			return
		}
		total := len(s.providers)
		s.providerIdx = (s.providerIdx + delta + total) % total
		s.syncModels(rt, "")
	case modelFocusModel:
		if len(s.models) == 0 {
			return
		}
		total := len(s.models)
		s.modelIdx = (s.modelIdx + delta + total) % total
	}
}

func (s *modelSwitchState) syncSelection(rt *host.Host) {
	provider, model, _ := rt.CurrentModelSelection(s.role())
	if len(s.providers) > 0 {
		s.providerIdx = 0
		for i, candidate := range s.providers {
			if candidate == provider {
				s.providerIdx = i
				break
			}
		}
	}
	s.syncModels(rt, model)
	s.message = ""
}

func (s *modelSwitchState) syncModels(rt *host.Host, preferred string) {
	// 候选列表末尾恒定追加“自定义”哨兵项，使任何 provider（含候选为空的新 provider）
	// 都能进入手动输入分配新模型。
	s.models = append(rt.ConfiguredModels(s.provider()), customModelOption)
	s.modelIdx = 0
	preferred = strings.TrimSpace(preferred)
	for i, model := range s.models {
		if model == preferred {
			s.modelIdx = i
			return
		}
	}
}

// isCustomSelected 判断模型字段当前是否停在“自定义”哨兵项上。
func (s *modelSwitchState) isCustomSelected() bool {
	return s.model() == customModelOption
}

func (s *modelSwitchState) apply(rt *host.Host) error {
	if len(s.providers) == 0 {
		return fmt.Errorf("当前没有可用 provider")
	}
	model := s.model()
	if model == customModelOption {
		model = utils.CleanInputLine(s.customInput)
		if model == "" {
			return fmt.Errorf("请输入模型名")
		}
	}
	if model == "" {
		return fmt.Errorf("provider %q 没有已配置模型", s.provider())
	}
	return rt.SwitchModel(s.role(), s.provider(), model)
}

func (m Model) handleModelSwitchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.modelSwitch == nil {
		return m, nil
	}
	state := m.modelSwitch

	if state.typing {
		return m.handleCustomModelKey(msg, state)
	}

	switch msg.Type {
	case tea.KeyEsc:
		m.modelSwitch = nil
		if m.mode != modeDone {
			return m, m.textarea.Focus()
		}
		return m, nil
	case tea.KeyTab, tea.KeyDown:
		state.moveFocus(1)
		return m, nil
	case tea.KeyShiftTab, tea.KeyUp:
		state.moveFocus(-1)
		return m, nil
	case tea.KeyLeft:
		state.cycle(-1, m.runtime)
		return m, nil
	case tea.KeyRight:
		state.cycle(1, m.runtime)
		return m, nil
	case tea.KeyEnter:
		// 只要当前模型选中“自定义”哨兵项，Enter 就先进入输入态而非直接应用。
		if state.isCustomSelected() {
			state.typing = true
			state.message = ""
			return m, nil
		}
		return m.applyModelSwitch(state)
	default:
		return m, nil
	}
}

// handleCustomModelKey 处理自定义模型名输入态下的按键。
func (m Model) handleCustomModelKey(msg tea.KeyMsg, state *modelSwitchState) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		state.typing = false
		state.customInput = ""
		state.message = ""
		return m, nil
	case tea.KeyEnter:
		return m.applyModelSwitch(state)
	case tea.KeyBackspace:
		if n := len(state.customInput); n > 0 {
			runes := []rune(state.customInput)
			state.customInput = string(runes[:len(runes)-1])
		}
		return m, nil
	case tea.KeyRunes, tea.KeySpace:
		state.customInput += utils.CleanInputRunes(msg.Runes)
		return m, nil
	default:
		return m, nil
	}
}

// applyModelSwitch 应用当前选择并关闭切换框，失败时把错误回显在框内。
func (m Model) applyModelSwitch(state *modelSwitchState) (tea.Model, tea.Cmd) {
	if err := state.apply(m.runtime); err != nil {
		state.message = err.Error()
		return m, nil
	}
	m.modelSwitch = nil
	if m.mode != modeDone {
		return m, tea.Batch(m.textarea.Focus(), fetchSnapshot(m.runtime))
	}
	return m, fetchSnapshot(m.runtime)
}

func renderModelSwitchBar(width int, state *modelSwitchState) string {
	if state == nil || width <= 0 {
		return ""
	}

	title := lipgloss.NewStyle().
		Foreground(colorMuted).
		Bold(true).
		Render("/model 切换模型")

	row1 := renderModelField("角色", state.roleLabel(), state.focus == modelFocusRole)
	row2 := renderModelField("Provider", state.provider(), state.focus == modelFocusProvider)
	modelValue := state.model()
	if state.typing {
		modelValue = state.customInput + "▏"
	}
	row3 := renderModelField("模型", modelValue, state.focus == modelFocusModel)
	hintText := "Tab 切字段   ←→ 切选项   Enter 应用   Esc 取消"
	if state.typing {
		hintText = "输入模型名   Enter 确认   Esc 返回"
	}
	hint := lipgloss.NewStyle().
		Foreground(colorDim).
		Italic(true).
		Render(hintText)
	lines := []string{
		row1,
		row2,
		row3,
		hint,
	}
	if state.message != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(colorError).Italic(true).Render(truncate(state.message, width-8)))
	}

	content := strings.Join(lines, "\n")
	boxW := lipgloss.Width(content) + 8
	maxW := width - 2
	if maxW > 68 {
		maxW = 68
	}
	if boxW > maxW {
		boxW = maxW
	}
	if boxW < 56 {
		boxW = 56
	}

	innerW := boxW - 2
	if innerW < 16 {
		innerW = 16
	}
	sepW := innerW - lipgloss.Width(title) - 3
	if sepW < 0 {
		sepW = 0
	}
	lineStyle := lipgloss.NewStyle().Foreground(colorDim)
	topBorder := lineStyle.Render("┌─ ") + title + lineStyle.Render(" "+strings.Repeat("─", sepW)+"┐")
	bottomBorder := lineStyle.Render("└" + strings.Repeat("─", innerW) + "┘")

	body := make([]string, 0, len(lines))
	for _, line := range lines {
		padding := innerW - lipgloss.Width(line)
		if padding < 0 {
			padding = 0
		}
		body = append(body, lineStyle.Render("│")+line+strings.Repeat(" ", padding)+lineStyle.Render("│"))
	}

	return strings.Join(append(append([]string{topBorder}, body...), bottomBorder), "\n")
}

func renderModelField(label, value string, focused bool) string {
	if strings.TrimSpace(value) == "" {
		value = "未设置"
	}
	labelText := lipgloss.NewStyle().
		Foreground(colorMuted).
		Width(12).
		Render(label + ":")
	style := lipgloss.NewStyle().Padding(0, 1).Foreground(bodyTextColor)
	if focused {
		style = style.Foreground(colorAccent).Bold(true).Underline(true)
	}
	return labelText + style.Render("["+value+"]")
}
