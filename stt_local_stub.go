//go:build !cgo_whisper

package main

import (
	"fmt"

	"github.com/charmbracelet/bubbletea"
)

type localSTTSession struct{}

func startLocalSTTSession(_ chan tea.Msg) (*localSTTSession, error) {
	return nil, fmt.Errorf("local whisper STT not available in this build")
}

func (s *localSTTSession) stop() {}

func prewarmLocalSTTModelCmd() tea.Cmd { return nil }
