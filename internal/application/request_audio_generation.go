package application

import (
	"context"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

func ReviewAndRequestAudio(
	ctx context.Context,
	scriptRepo ports.ScriptRepository,
	audioFileRepo ports.AudioFileRepository,
	publisher ports.AudioJobPublisher,
	scriptID, reviewer, voiceID string,
) error {
	script, err := scriptRepo.FindByID(ctx, scriptID)
	if err != nil {
		return err
	}
	if err := script.MarkReviewed(reviewer); err != nil {
		return err
	}
	if err := scriptRepo.Save(ctx, script); err != nil {
		return err
	}

	audioFile, err := domain.NewAudioFile(script.ID(), voiceID)
	if err != nil {
		return err
	}
	if err := audioFileRepo.Save(ctx, audioFile); err != nil {
		return err
	}

	return publisher.PublishTTSJob(ctx, audioFile.ID(), script.ID(), script.Text().String(), script.Language().String(), voiceID)
}
