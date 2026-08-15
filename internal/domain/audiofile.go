package domain

import (
	"errors"
	"time"
)

type AudioFileStatus string

const (
	AudioFileStatusQueued     AudioFileStatus = "queued"
	AudioFileStatusGenerating AudioFileStatus = "generating"
	AudioFileStatusReady      AudioFileStatus = "ready"
	AudioFileStatusFailed     AudioFileStatus = "failed"
)

var (
	ErrAudioFileScriptIDRequired   = errors.New("audio file: script id is required")
	ErrAudioFileStorageURLRequired = errors.New("audio file: storage URL is required")
	ErrAudioFileInvalidDuration    = errors.New("audio file: duration must be positive")
	ErrAudioFileNotQueued          = errors.New("audio file: must be queued to start generating")
	ErrAudioFileNotGenerating      = errors.New("audio file: must be generating to complete or fail")
	ErrAudioFileNotFailed          = errors.New("audio file: must be failed to retry")
)

// --- Value Object ---

type GeneratedAudio struct {
	storageURL    string
	timestampsURL string
	duration      time.Duration
}

func NewGeneratedAudio(storageURL, timestampsURL string, duration time.Duration) (GeneratedAudio, error) {
	if storageURL == "" {
		return GeneratedAudio{}, ErrAudioFileStorageURLRequired
	}
	if duration <= 0 {
		return GeneratedAudio{}, ErrAudioFileInvalidDuration
	}
	return GeneratedAudio{storageURL: storageURL, timestampsURL: timestampsURL, duration: duration}, nil
}

func (g GeneratedAudio) StorageURL() string      { return g.storageURL }
func (g GeneratedAudio) TimestampsURL() string   { return g.timestampsURL }
func (g GeneratedAudio) Duration() time.Duration { return g.duration }

// --- Entity ---

type AudioFile struct {
	id            string
	scriptID      string
	voiceID       string
	status        AudioFileStatus
	audio         GeneratedAudio
	failureReason string
}

func NewAudioFile(scriptID, voiceID string) (*AudioFile, error) {
	if scriptID == "" {
		return nil, ErrAudioFileScriptIDRequired
	}
	return &AudioFile{
		id:       newID(),
		scriptID: scriptID,
		voiceID:  voiceID,
		status:   AudioFileStatusQueued,
	}, nil
}

// ReconstructAudioFile rebâtit un AudioFile depuis des données déjà valides
// (une ligne Postgres) — même rôle que ReconstructPlace/ReconstructScript.
func ReconstructAudioFile(id, scriptID, voiceID string, status AudioFileStatus, audio GeneratedAudio, failureReason string) *AudioFile {
	return &AudioFile{
		id:            id,
		scriptID:      scriptID,
		voiceID:       voiceID,
		status:        status,
		audio:         audio,
		failureReason: failureReason,
	}
}

func (a *AudioFile) MarkGenerating() error {
	if a.status != AudioFileStatusQueued {
		return ErrAudioFileNotQueued
	}
	a.status = AudioFileStatusGenerating
	return nil
}

func (a *AudioFile) MarkReady(audio GeneratedAudio) error {
	if a.status != AudioFileStatusGenerating {
		return ErrAudioFileNotGenerating
	}
	a.status = AudioFileStatusReady
	a.audio = audio
	return nil
}

func (a *AudioFile) MarkFailed(reason string) error {
	if a.status != AudioFileStatusGenerating {
		return ErrAudioFileNotGenerating
	}
	a.status = AudioFileStatusFailed
	a.failureReason = reason
	return nil
}

func (a *AudioFile) Retry() error {
	if a.status != AudioFileStatusFailed {
		return ErrAudioFileNotFailed
	}
	a.status = AudioFileStatusQueued
	a.failureReason = ""
	return nil
}

// --- lecture ---

func (a *AudioFile) ID() string              { return a.id }
func (a *AudioFile) ScriptID() string        { return a.scriptID }
func (a *AudioFile) VoiceID() string         { return a.voiceID }
func (a *AudioFile) Status() AudioFileStatus { return a.status }
func (a *AudioFile) Audio() GeneratedAudio   { return a.audio }
func (a *AudioFile) FailureReason() string   { return a.failureReason }
