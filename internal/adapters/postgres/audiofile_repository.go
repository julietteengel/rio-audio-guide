package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"rioaudioguide/backend/internal/domain"
)

type AudioFileRepository struct {
	pool *pgxpool.Pool
}

func NewAudioFileRepository(pool *pgxpool.Pool) *AudioFileRepository {
	return &AudioFileRepository{pool: pool}
}

func (r *AudioFileRepository) Save(ctx context.Context, audioFile *domain.AudioFile) error {
	audio := audioFile.Audio()
	_, err := r.pool.Exec(ctx, `
		INSERT INTO audio_files (id, script_id, voice_id, status, storage_url, timestamps_url, duration_ms, failure_reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			storage_url = EXCLUDED.storage_url,
			timestamps_url = EXCLUDED.timestamps_url,
			duration_ms = EXCLUDED.duration_ms,
			failure_reason = EXCLUDED.failure_reason,
			updated_at = now()
	`, audioFile.ID(), audioFile.ScriptID(), audioFile.VoiceID(), string(audioFile.Status()),
		audio.StorageURL(), audio.TimestampsURL(), audio.Duration().Milliseconds(), audioFile.FailureReason())
	return err
}

func (r *AudioFileRepository) FindByID(ctx context.Context, id string) (*domain.AudioFile, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, script_id, voice_id, status, COALESCE(storage_url, ''),
		       COALESCE(timestamps_url, ''), COALESCE(duration_ms, 0), COALESCE(failure_reason, '')
		FROM audio_files WHERE id = $1
	`, id)

	var audioFileID, scriptID, voiceID, status, storageURL, timestampsURL, failureReason string
	var durationMs int64
	if err := row.Scan(&audioFileID, &scriptID, &voiceID, &status, &storageURL, &timestampsURL,
		&durationMs, &failureReason); err != nil {
		return nil, err
	}

	// A queued/generating/failed AudioFile has no audio yet — storageURL/duration
	// are legitimately empty, so build the value object only when there's something
	// to build. NewGeneratedAudio would otherwise reject the empty storage URL.
	var audio domain.GeneratedAudio
	if storageURL != "" {
		var err error
		audio, err = domain.NewGeneratedAudio(storageURL, timestampsURL, time.Duration(durationMs)*time.Millisecond)
		if err != nil {
			return nil, err
		}
	}

	return domain.ReconstructAudioFile(audioFileID, scriptID, voiceID, domain.AudioFileStatus(status), audio, failureReason), nil
}
