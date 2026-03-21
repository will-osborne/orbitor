//go:build cgo_whisper

package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/gen2brain/malgo"
	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

const (
	localSTTSampleRate    = 16000
	localSTTPartialPeriod = 2 * time.Second
	localSTTMinSamples    = localSTTSampleRate / 2 // 0.5s minimum before first inference
)

// Process-level model singleton — loading whisper takes ~500ms so we keep it resident.
var (
	localSTTOnce     sync.Once
	localSTTModel    whisper.Model
	localSTTModelErr error
)

func ensureLocalSTTModel() (whisper.Model, error) {
	localSTTOnce.Do(func() {
		path, err := ensureWhisperTinyModel()
		if err != nil {
			localSTTModelErr = err
			return
		}
		localSTTModel, localSTTModelErr = whisper.New(path)
	})
	return localSTTModel, localSTTModelErr
}

// prewarmLocalSTTModelCmd loads the whisper model in the background at startup
// so the first dictation attempt doesn't block on model loading.
func prewarmLocalSTTModelCmd() tea.Cmd {
	return func() tea.Msg {
		_, _ = ensureLocalSTTModel()
		return nil
	}
}

// localSTTSession manages a single push-to-talk dictation session using local
// whisper inference. Audio is accumulated from the mic and whisper runs on the
// full buffer every localSTTPartialPeriod seconds, emitting partial results.
type localSTTSession struct {
	model    whisper.Model
	wctx     whisper.Context
	wctxMu   sync.Mutex

	audioBuf []float32
	bufMu    sync.Mutex

	device   *malgo.Device
	mctx     *malgo.AllocatedContext

	stopCh   chan struct{}
	loopDone chan struct{}
	extCh    chan<- tea.Msg

	lastText string // last partial text, for dedup
}

func startLocalSTTSession(extCh chan tea.Msg) (*localSTTSession, error) {
	model, err := ensureLocalSTTModel()
	if err != nil {
		return nil, fmt.Errorf("whisper model: %w", err)
	}
	wctx, err := model.NewContext()
	if err != nil {
		return nil, fmt.Errorf("whisper context: %w", err)
	}
	_ = wctx.SetLanguage("en")
	wctx.SetThreads(4)

	s := &localSTTSession{
		model:    model,
		wctx:     wctx,
		stopCh:   make(chan struct{}),
		loopDone: make(chan struct{}),
		extCh:    extCh,
	}

	mctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, func(string) {})
	if err != nil {
		return nil, fmt.Errorf("audio context: %w", err)
	}

	cfg := malgo.DefaultDeviceConfig(malgo.Capture)
	cfg.Capture.Format = malgo.FormatF32
	cfg.Capture.Channels = 1
	cfg.SampleRate = localSTTSampleRate
	cfg.Alsa.NoMMap = 1

	device, err := malgo.InitDevice(mctx.Context, cfg, malgo.DeviceCallbacks{
		Data: func(_, input []byte, framecount uint32) {
			n := int(framecount)
			if len(input) < n*4 {
				return
			}
			f32 := make([]float32, n)
			for i := range f32 {
				bits := binary.LittleEndian.Uint32(input[i*4 : i*4+4])
				f32[i] = math.Float32frombits(bits)
			}
			s.bufMu.Lock()
			s.audioBuf = append(s.audioBuf, f32...)
			s.bufMu.Unlock()
		},
	})
	if err != nil {
		_ = mctx.Uninit()
		mctx.Free()
		return nil, fmt.Errorf("audio device: %w", err)
	}

	if err := device.Start(); err != nil {
		device.Uninit()
		_ = mctx.Uninit()
		mctx.Free()
		return nil, fmt.Errorf("audio start: %w", err)
	}

	s.device = device
	s.mctx = mctx

	go func() {
		defer close(s.loopDone)
		s.inferenceLoop()
	}()

	return s, nil
}

func (s *localSTTSession) inferenceLoop() {
	ticker := time.NewTicker(localSTTPartialPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.runInference(false)
		}
	}
}

func (s *localSTTSession) runInference(final bool) {
	s.bufMu.Lock()
	if len(s.audioBuf) < localSTTMinSamples {
		s.bufMu.Unlock()
		if final {
			s.extCh <- sttResultMsg{external: true}
		}
		return
	}
	samples := make([]float32, len(s.audioBuf))
	copy(samples, s.audioBuf)
	s.bufMu.Unlock()

	s.wctxMu.Lock()
	defer s.wctxMu.Unlock()

	if err := s.wctx.Process(samples, nil, nil, nil); err != nil {
		if final {
			s.extCh <- sttResultMsg{err: err, external: true}
		}
		return
	}

	var sb strings.Builder
	for {
		seg, err := s.wctx.NextSegment()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		t := strings.TrimSpace(seg.Text)
		if t != "" {
			sb.WriteString(t)
			sb.WriteByte(' ')
		}
	}
	text := strings.TrimSpace(sb.String())

	if final {
		s.extCh <- sttResultMsg{text: text, external: true}
		return
	}
	if text == "" || text == s.lastText {
		return
	}
	s.lastText = text
	s.extCh <- sttPartialMsg{text: text, external: true}
}

// stop halts the audio capture, waits for the inference loop to finish, then
// runs a final inference on all accumulated audio and sends the result.
func (s *localSTTSession) stop() {
	close(s.stopCh)
	<-s.loopDone

	if s.device != nil {
		s.device.Uninit()
		s.device = nil
	}
	if s.mctx != nil {
		_ = s.mctx.Uninit()
		s.mctx.Free()
		s.mctx = nil
	}

	s.runInference(true)
}
