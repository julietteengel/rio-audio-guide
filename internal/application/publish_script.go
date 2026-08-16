package application

import (
	"context"
	"time"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

func StartAudioGeneration(ctx context.Context, audioFileRepo ports.AudioFileRepository, audioFileID string) error {
	audioFile, err := audioFileRepo.FindByID(ctx, audioFileID)
	if err != nil {
		return err
	}
	if err := audioFile.MarkGenerating(); err != nil {
		return err
	}
	return audioFileRepo.Save(ctx, audioFile)
}

func CompleteAudioGeneration(
	ctx context.Context,
	scriptRepo ports.ScriptRepository,
	audioFileRepo ports.AudioFileRepository,
	audioFileID, storageURL, timestampsURL string,
	duration time.Duration,
) error {
	audioFile, err := audioFileRepo.FindByID(ctx, audioFileID)
	if err != nil {
		return err
	}

	audio, err := domain.NewGeneratedAudio(storageURL, timestampsURL, duration)
	if err != nil {
		return err
	}
	if err := audioFile.MarkReady(audio); err != nil {
		return err
	}
	if err := audioFileRepo.Save(ctx, audioFile); err != nil {
		return err
	}

	script, err := scriptRepo.FindByID(ctx, audioFile.ScriptID())
	if err != nil {
		return err
	}
	if err := script.Publish(); err != nil {
		return err
	}
	return scriptRepo.Save(ctx, script)
}
