package agent

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiWhite  = "\x1b[38;2;255;255;255m"
	ansiGreen  = "\x1b[38;2;127;209;160m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[38;2;240;138;126m"
	ansiMuted  = "\x1b[90m"
	ansiReplay = "\x1b[38;2;181;185;183;48;2;48;52;49m"
	ansiOkBg   = "\x1b[1;38;2;127;209;160;48;2;39;54;45m"
	ansiInfoBg = "\x1b[1;38;2;130;183;242;48;2;40;51;66m"
	ansiWarnBg = "\x1b[1;38;2;245;177;92;48;2;64;48;31m"
	ansiErrBg  = "\x1b[1;38;2;240;138;126;48;2;57;46;44m"
)

func terminalOutput() bool {
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled || strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return false
	}
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func paint(value, style string) string {
	if !terminalOutput() {
		return value
	}
	return style + value + ansiReset
}

func statusBadge(status int) string {
	style := ansiMuted
	switch status / 100 {
	case 2:
		style = ansiOkBg
	case 3:
		style = ansiInfoBg
	case 4:
		style = ansiWarnBg
	case 5:
		style = ansiErrBg
	}
	return paint(fmt.Sprintf(" %d ", status), style)
}

func errorBadge() string {
	return paint(" ERR ", ansiErrBg)
}

func replayBadge() string {
	if !terminalOutput() {
		return column("", 8)
	}
	return ansiReplay + " replay " + ansiReset
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

func column(value string, width int) string {
	characters := []rune(value)
	if len(characters) > width {
		value = string(characters[:width-1]) + "…"
	}
	return fmt.Sprintf("%-*s", width, value)
}
