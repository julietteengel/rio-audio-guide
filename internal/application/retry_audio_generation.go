package application

import (
	"context"

	"rioaudioguide/backend/internal/ports"
)

// RetryAudioGeneration re-queues a failed AudioFile with the same voice_id
// it already had (no need to resupply one) -- audioFile.Retry() enforces
// failed->queued via the domain guard, same shape as ReviewAndRequestAudio's
// first publish, just skipping the "mark script reviewed" step since it's
// already done.
func RetryAudioGeneration(
	ctx context.Context,
	scriptRepo ports.ScriptRepository,
	audioFileRepo ports.AudioFileRepository,
	publisher ports.AudioJobPublisher,
	audioFileID string,
) error {
	audioFile, err := audioFileRepo.FindByID(ctx, audioFileID)
	if err != nil {
		return err
	}
	if err := audioFile.Retry(); err != nil {
		return err
	}
	if err := audioFileRepo.Save(ctx, audioFile); err != nil {
		return err
	}

	script, err := scriptRepo.FindByID(ctx, audioFile.ScriptID())
	if err != nil {
		return err
	}

	return publisher.PublishTTSJob(ctx, audioFile.ID(), script.ID(), script.Text().String(), script.Language().String(), audioFile.VoiceID())
}
