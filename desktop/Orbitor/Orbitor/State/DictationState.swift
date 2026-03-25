import Foundation
import Speech
import AVFoundation
import AppKit

@Observable
final class DictationState {
    var isRecording = false
    var transcribedText = ""

    private var audioEngine: AVAudioEngine?
    private var recognitionRequest: SFSpeechAudioBufferRecognitionRequest?
    private var recognitionTask: SFSpeechRecognitionTask?
    private let speechRecognizer = SFSpeechRecognizer(locale: Locale(identifier: "en-US"))
    private var authorized = false

    // Space-hold detection via NSEvent monitor
    private var eventMonitor: Any?
    private var spaceDownTime: Date?
    private var spaceIsHeld = false
    private let holdThreshold: TimeInterval = 0.4 // hold 400ms to activate
    /// Called when dictation finishes — set by PromptInputView to append text.
    var onDictationComplete: ((String) -> Void)?
    /// Called to insert a space when user does a quick tap (not a hold).
    var onInsertSpace: (() -> Void)?
    /// Whether the prompt input is empty (only start dictation when empty).
    var promptIsEmpty = true
    /// Whether we swallowed the initial space press and are waiting to see if it's a hold.
    private var pendingSpace = false

    init() {
        SFSpeechRecognizer.requestAuthorization { [weak self] status in
            DispatchQueue.main.async {
                self?.authorized = (status == .authorized)
            }
        }
    }

    var isAvailable: Bool {
        authorized && (speechRecognizer?.isAvailable ?? false)
    }

    /// Install an NSEvent local monitor that intercepts space key holds.
    func installEventMonitor() {
        guard eventMonitor == nil else { return }
        eventMonitor = NSEvent.addLocalMonitorForEvents(matching: [.keyDown, .keyUp]) { [weak self] event in
            guard let self else { return event }
            return self.handleKeyEvent(event)
        }
    }

    func removeEventMonitor() {
        if let monitor = eventMonitor {
            NSEvent.removeMonitor(monitor)
            eventMonitor = nil
        }
    }

    private func handleKeyEvent(_ event: NSEvent) -> NSEvent? {
        // Only intercept unmodified space
        guard event.keyCode == 49, event.modifierFlags.intersection(.deviceIndependentFlagsMask).isEmpty else {
            return event
        }

        if event.type == .keyDown {
            if isRecording {
                // Swallow spaces while recording
                return nil
            }
            if event.isARepeat {
                // Check if we've held long enough
                if !spaceIsHeld, let downTime = spaceDownTime,
                   Date().timeIntervalSince(downTime) >= holdThreshold,
                   isAvailable {
                    spaceIsHeld = true
                    pendingSpace = false
                    DispatchQueue.main.async { self.startRecording() }
                }
                return (spaceIsHeld || pendingSpace) ? nil : event
            }
            // Initial press — swallow if prompt is empty and dictation could activate
            spaceDownTime = Date()
            spaceIsHeld = false
            if promptIsEmpty && isAvailable {
                pendingSpace = true
                return nil
            }
            return event
        }

        if event.type == .keyUp {
            if isRecording || spaceIsHeld {
                spaceIsHeld = false
                spaceDownTime = nil
                pendingSpace = false
                DispatchQueue.main.async {
                    let result = self.stopRecording()
                    if !result.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                        self.onDictationComplete?(result)
                    }
                }
                return nil
            }
            // Quick tap — insert the space we swallowed
            if pendingSpace {
                pendingSpace = false
                spaceDownTime = nil
                DispatchQueue.main.async {
                    self.onInsertSpace?()
                }
                return nil
            }
            spaceDownTime = nil
            spaceIsHeld = false
            return event
        }

        return event
    }

    @MainActor
    func startRecording() {
        guard isAvailable, !isRecording else { return }

        let audioEngine = AVAudioEngine()
        let request = SFSpeechAudioBufferRecognitionRequest()
        request.shouldReportPartialResults = true

        self.audioEngine = audioEngine
        self.recognitionRequest = request
        self.transcribedText = ""
        self.isRecording = true

        recognitionTask = speechRecognizer?.recognitionTask(with: request) { [weak self] result, error in
            guard let self else { return }
            if let result {
                DispatchQueue.main.async {
                    self.transcribedText = result.bestTranscription.formattedString
                }
            }
            if error != nil || (result?.isFinal ?? false) {
                DispatchQueue.main.async {
                    _ = self.stopRecording()
                }
            }
        }

        let inputNode = audioEngine.inputNode
        let format = inputNode.outputFormat(forBus: 0)
        inputNode.installTap(onBus: 0, bufferSize: 1024, format: format) { buffer, _ in
            request.append(buffer)
        }

        do {
            audioEngine.prepare()
            try audioEngine.start()
        } catch {
            _ = stopRecording()
        }
    }

    @MainActor
    @discardableResult
    func stopRecording() -> String {
        guard isRecording else { return transcribedText }
        isRecording = false
        audioEngine?.stop()
        audioEngine?.inputNode.removeTap(onBus: 0)
        recognitionRequest?.endAudio()
        recognitionTask?.cancel()
        audioEngine = nil
        recognitionRequest = nil
        recognitionTask = nil
        return transcribedText
    }
}
