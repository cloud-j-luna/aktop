package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var (
	// ANSI terminal palette colors (respects terminal theme like Catppuccin)
	// Uses the terminal's 16-color palette (0-15)
	primaryColor   = lipgloss.ANSIColor(1)  // Red
	accentColor    = lipgloss.ANSIColor(9)  // Bright Red
	secondaryColor = lipgloss.ANSIColor(5)  // Magenta/Mauve
	successColor   = lipgloss.ANSIColor(2)  // Green
	warningColor   = lipgloss.ANSIColor(3)  // Yellow
	errorColor     = lipgloss.ANSIColor(9)  // Bright Red
	mutedColor     = lipgloss.ANSIColor(8)  // Bright Black (Surface)
	textColor      = lipgloss.ANSIColor(7)  // White (Text)
	brightText     = lipgloss.ANSIColor(15) // Bright White
	borderColor    = lipgloss.ANSIColor(8)  // Bright Black (Surface)

	// Title style
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accentColor).
			MarginBottom(1)

	// Header style for section headers
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(borderColor).
			PaddingBottom(1).
			MarginBottom(1)

	// Label style for field labels
	labelStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Width(12)

	// Value style for field values
	valueStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(brightText)

	// Progress bar styles
	progressBarWidth = 40

	progressFullStyle = lipgloss.NewStyle().
				Foreground(primaryColor)

	progressEmptyStyle = lipgloss.NewStyle().
				Foreground(mutedColor)

	// Percentage styles based on threshold
	percentLowStyle = lipgloss.NewStyle().
			Foreground(warningColor)

	percentHighStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(successColor)

	// Grid styles (dots and version indicators stay green)
	gridVotedStyle = lipgloss.NewStyle().
			Foreground(successColor)

	gridNotVotedStyle = lipgloss.NewStyle().
				Foreground(mutedColor)

	// Error style
	errorStyle = lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true)

	// Help style
	helpStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			MarginTop(1)

	// Status bar style
	statusBarStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			MarginTop(1)

	// Muted text style (for general muted text)
	mutedStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	// Proposer style (star indicator)
	proposerStyle = lipgloss.NewStyle().
			Foreground(warningColor).
			Bold(true)

	// Tab styles
	tabActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(brightText).
			Background(primaryColor).
			Padding(0, 1)

	tabInactiveStyle = lipgloss.NewStyle().
				Foreground(mutedColor).
				Padding(0, 1)

	// Moniker style
	monikerStyle = lipgloss.NewStyle().
			Foreground(textColor)

	// Highlight style for selected rows
	highlightStyle = lipgloss.NewStyle().
			Foreground(brightText).
			Bold(true)

	// Detail view styles
	detailHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(primaryColor)

	detailLabelStyle = lipgloss.NewStyle().
				Foreground(mutedColor).
				Width(10)

	detailValueStyle = lipgloss.NewStyle().
				Foreground(textColor)
)

// ProgressBar renders a progress bar with the given percentage (0-1)
func ProgressBar(percent float64, width int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 1 {
		percent = 1
	}

	filled := int(float64(width) * percent)
	empty := width - filled

	bar := progressFullStyle.Render(repeatChar('█', filled)) +
		progressEmptyStyle.Render(repeatChar('░', empty))

	return bar
}

// repeatChar repeats a character n times
func repeatChar(char rune, n int) string {
	if n <= 0 {
		return ""
	}
	result := make([]rune, n)
	for i := range result {
		result[i] = char
	}
	return string(result)
}

// FormatPercent formats a percentage with color based on threshold
func FormatPercent(percent float64) string {
	pctStr := lipgloss.NewStyle().Width(6).Render(
		fmt.Sprintf("%5.1f%%", percent*100),
	)

	if percent >= 0.667 {
		return percentHighStyle.Render(pctStr)
	}
	return percentLowStyle.Render(pctStr)
}

// FormatVoteGrid formats the bit array pattern into a colored grid
func FormatVoteGrid(pattern string, width int) string {
	if pattern == "" {
		return mutedStyle.Render("No vote data")
	}

	var result string
	for i, char := range pattern {
		if i > 0 && i%width == 0 {
			result += "\n"
		}
		if char == 'x' {
			result += gridVotedStyle.Render("●")
		} else {
			result += gridNotVotedStyle.Render("○")
		}
	}
	return result
}
