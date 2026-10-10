package agent

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	green  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#2f6e49", Dark: "#7fd1a0"})
	white  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#181b19", Dark: "#f5f7f5"})
	muted  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#5f6b64", Dark: "#929e96"})
	amber  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#8c550f", Dark: "#f5b15c"})
	red    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#a43731", Dark: "#f08a7e"})
	badges = map[int]lipgloss.Style{
		1: lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#5f6b64", Dark: "#b5b9b7"}).Background(lipgloss.AdaptiveColor{Light: "#edf0ed", Dark: "#303431"}).Bold(true).Padding(0, 1),
		2: lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#2f6e49", Dark: "#7fd1a0"}).Background(lipgloss.AdaptiveColor{Light: "#e3efe6", Dark: "#27362d"}).Bold(true).Padding(0, 1),
		3: lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#315da8", Dark: "#82b7f2"}).Background(lipgloss.AdaptiveColor{Light: "#e7eefb", Dark: "#283342"}).Bold(true).Padding(0, 1),
		4: lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#8c550f", Dark: "#f5b15c"}).Background(lipgloss.AdaptiveColor{Light: "#fff1db", Dark: "#40301f"}).Bold(true).Padding(0, 1),
		5: lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#a43731", Dark: "#f08a7e"}).Background(lipgloss.AdaptiveColor{Light: "#fde9e6", Dark: "#392e2c"}).Bold(true).Padding(0, 1),
	}
	replayStyle   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#5f6b64", Dark: "#b5b9b7"}).Background(lipgloss.AdaptiveColor{Light: "#edf0ed", Dark: "#303431"}).Padding(0, 1)
	spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
)

type terminalUI struct {
	mu      sync.Mutex
	color   bool
	waiting chan struct{}
	frame   int
}

var screen = newTerminalUI()

func newTerminalUI() *terminalUI {
	info, err := os.Stdout.Stat()
	color := err == nil && info.Mode()&os.ModeCharDevice != 0 && os.Getenv("NO_COLOR") == "" && !strings.EqualFold(os.Getenv("TERM"), "dumb")
	return &terminalUI{color: color}
}

func (ui *terminalUI) render(style lipgloss.Style, text string) string {
	if !ui.color {
		return text
	}
	return style.Render(text)
}

func (ui *terminalUI) cell(style lipgloss.Style, text string, width int) string {
	characters := []rune(text)
	if len(characters) > width {
		text = string(characters[:width-1]) + "…"
	}
	if !ui.color {
		return fmt.Sprintf("%-*s", width, text)
	}
	return style.Width(width).Render(text)
}

func (ui *terminalUI) badge(text string, class int) string {
	if !ui.color {
		return " " + text + " "
	}
	style, ok := badges[class]
	if !ok {
		style = badges[1]
	}
	return style.Render(text)
}

func (ui *terminalUI) spinnerLine() string {
	return "  " + ui.render(green, spinnerFrames[ui.frame%len(spinnerFrames)]) + "  " + ui.render(muted, "waiting for webhooks") + ui.render(muted, "  ·  Ctrl+C to stop")
}

func (ui *terminalUI) clearWaiting() {
	if ui.waiting != nil {
		fmt.Print("\r\x1b[2K")
	}
}

func (ui *terminalUI) printBlock(block string) {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	ui.clearWaiting()
	fmt.Println(block)
	if ui.waiting != nil {
		fmt.Print(ui.spinnerLine())
	}
}

func (ui *terminalUI) startWaiting() {
	ui.mu.Lock()
	if !ui.color {
		fmt.Println("  waiting for webhooks")
		ui.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	ui.waiting = stop
	ui.frame = 0
	fmt.Print(ui.spinnerLine())
	ui.mu.Unlock()
	go func() {
		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				ui.mu.Lock()
				if ui.waiting != stop {
					ui.mu.Unlock()
					return
				}
				ui.frame++
				ui.clearWaiting()
				fmt.Print(ui.spinnerLine())
				ui.mu.Unlock()
			}
		}
	}()
}

func (ui *terminalUI) stopWaiting() {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	if ui.waiting != nil {
		close(ui.waiting)
		ui.clearWaiting()
		fmt.Println()
		ui.waiting = nil
	}
}

func (ui *terminalUI) connected(slug, target, expiry string) {
	ui.printBlock(ui.render(green, "✓") + " " + ui.render(white.Bold(true), "Connected") + "  " + ui.render(muted, slug) + "\n" +
		"  forwarding to " + ui.render(white, displayTarget(target)) + "  " + ui.render(muted, "· "+expiry) + "\n")
	ui.startWaiting()
}

func (ui *terminalUI) targetChanged(target string) {
	ui.printBlock(ui.render(green, "✓") + " " + ui.render(white.Bold(true), "Target switched") + "\n" +
		"  forwarding to " + ui.render(white, displayTarget(target)))
}

func (ui *terminalUI) request(method, path string, replay bool, status int, elapsed time.Duration, requestError error) {
	replayCell := strings.Repeat(" ", 8)
	if replay {
		replayCell = ui.render(replayStyle, "replay")
		if !ui.color {
			replayCell = ui.cell(muted, "replay", 8)
		}
	}
	code := ui.badge(fmt.Sprint(status), status/100)
	if requestError != nil {
		code = ui.badge("ERR", 5)
	}
	row := lipgloss.JoinHorizontal(lipgloss.Top,
		ui.render(muted, time.Now().Format("15:04:05")), "  ", ui.cell(white, method, 6), "  ",
		ui.cell(white, path, 30), "  ", replayCell, "  ", code, "  ",
		ui.render(muted, fmt.Sprintf("%d ms", elapsed.Milliseconds())))
	if requestError != nil {
		row += "\n  " + ui.render(red, requestError.Error())
	}
	ui.printBlock(row)
}

func (ui *terminalUI) warning(message string) {
	ui.printBlock(ui.render(amber, "!") + " " + message)
}

func durationLabel(duration time.Duration) string {
	minutes := int(duration.Round(time.Minute) / time.Minute)
	if minutes < 1 {
		return "<1m"
	}
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%dh%02dm", minutes/60, minutes%60)
}

func displayTarget(target string) string {
	return strings.TrimPrefix(strings.TrimPrefix(target, "http://"), "https://")
}
