// internal/adapters/awspolly/generator.go
package awspolly

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/polly"
	"github.com/aws/aws-sdk-go-v2/service/polly/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"golang.org/x/sync/errgroup"

	"rioaudioguide/backend/internal/ports"
)

// pollyAPI n'expose que les deux méthodes utilisées ici -- *polly.Client les
// satisfait déjà structurellement. Permet de tester avec un faux client Go
// plutôt que de simuler la signature de requêtes du SDK v2 (impossible
// proprement via httptest, contrairement à l'appel REST direct d'ElevenLabs).
type pollyAPI interface {
	StartSpeechSynthesisTask(ctx context.Context, params *polly.StartSpeechSynthesisTaskInput, optFns ...func(*polly.Options)) (*polly.StartSpeechSynthesisTaskOutput, error)
	GetSpeechSynthesisTask(ctx context.Context, params *polly.GetSpeechSynthesisTaskInput, optFns ...func(*polly.Options)) (*polly.GetSpeechSynthesisTaskOutput, error)
}

// s3GetObjectAPI : seule méthode nécessaire pour relire le fichier de marks
// une fois sa tâche terminée (calcul de durée, Task 4). *s3.Client la
// satisfait déjà.
type s3GetObjectAPI interface {
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

type Generator struct {
	polly        pollyAPI
	s3           s3GetObjectAPI
	bucket       string
	pollInterval time.Duration
}

func NewGenerator(pollyClient pollyAPI, s3Client s3GetObjectAPI, bucket string) *Generator {
	return &Generator{polly: pollyClient, s3: s3Client, bucket: bucket, pollInterval: 2 * time.Second}
}

// runTask lance une StartSpeechSynthesisTask et attend sa complétion en
// polling GetSpeechSynthesisTask. Renvoie la clé S3 (pas l'URL complète --
// Generate, Task 4, assemble le "s3://bucket/clé" final) du fichier produit.
func (g *Generator) runTask(ctx context.Context, text, languageCode, voiceID string, format types.OutputFormat, marks []types.SpeechMarkType, keyPrefix string) (string, error) {
	out, err := g.polly.StartSpeechSynthesisTask(ctx, &polly.StartSpeechSynthesisTaskInput{
		Engine:             types.EngineNeural,
		LanguageCode:       types.LanguageCode(languageCode),
		OutputFormat:       format,
		OutputS3BucketName: &g.bucket,
		OutputS3KeyPrefix:  &keyPrefix,
		SpeechMarkTypes:    marks,
		Text:               &text,
		TextType:           types.TextTypeSsml,
		VoiceId:            types.VoiceId(voiceID),
	})
	if err != nil {
		return "", classifyStartError(err)
	}

	taskID := out.SynthesisTask.TaskId
	if taskID == nil {
		return "", fmt.Errorf("awspolly: task response missing TaskId")
	}
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(g.pollInterval):
		}

		got, err := g.polly.GetSpeechSynthesisTask(ctx, &polly.GetSpeechSynthesisTaskInput{TaskId: taskID})
		if err != nil {
			return "", fmt.Errorf("awspolly: poll task %s: %w", *taskID, err)
		}

		switch got.SynthesisTask.TaskStatus {
		case types.TaskStatusCompleted:
			if got.SynthesisTask.OutputUri == nil {
				return "", fmt.Errorf("awspolly: completed task %s missing OutputUri", *taskID)
			}
			return keyFromOutputURI(*got.SynthesisTask.OutputUri, g.bucket)
		case types.TaskStatusFailed:
			reason := "unknown reason"
			if got.SynthesisTask.TaskStatusReason != nil {
				reason = *got.SynthesisTask.TaskStatusReason
			}
			// Une tâche Failed ne se corrige pas en la relançant à l'identique
			// -- même texte/voix/langue -- d'où PermanentError plutôt qu'une
			// erreur simple qui déclencherait un retry côté worker.go.
			// StatusCode à 0 : Polly ne donne pas de code HTTP pour un échec
			// de tâche asynchrone (contrairement au 4xx synchrone
			// d'ElevenLabs), seulement cette raison textuelle.
			return "", &ports.PermanentError{StatusCode: 0, Body: "awspolly: task failed: " + reason}
		}
		// scheduled/inProgress : reboucle.
	}
}

// classifyStartError distingue les erreurs Polly non récupérables (mauvaise
// config, requête rejetée -- retenter à l'identique ne changerait rien) des
// erreurs transitoires (throttling, réseau) que le retry générique de
// worker.go doit gérer. Liste tirée de la doc API StartSpeechSynthesisTask
// (docs.aws.amazon.com/polly/latest/APIReference/API_StartSpeechSynthesisTask.html#API_StartSpeechSynthesisTask_Errors) --
// toutes ces exceptions signalent une requête mal formée, jamais un problème
// transitoire.
func classifyStartError(err error) error {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "EngineNotSupportedException", "InvalidS3BucketException", "InvalidS3KeyException",
			"InvalidSampleRateException", "InvalidSnsTopicArnException", "InvalidSsmlException",
			"LanguageNotSupportedException", "LexiconNotFoundException",
			"MarksNotSupportedForFormatException", "SsmlMarksNotSupportedForTextTypeException",
			"TextLengthExceededException":
			return &ports.PermanentError{StatusCode: 0, Body: "awspolly: " + apiErr.ErrorMessage()}
		}
	}
	return fmt.Errorf("awspolly: start task: %w", err)
}

// keyFromOutputURI extrait la clé S3 de l'OutputUri renvoyé par Polly (URL
// HTTPS), en path-style (https://s3.<region>.amazonaws.com/<bucket>/<clé>)
// ou virtual-hosted-style (https://<bucket>.s3.<region>.amazonaws.com/<clé>)
// -- AWS ne garantit pas lequel des deux formats est utilisé, donc les deux
// sont gérés plutôt que de deviner le nom de fichier nous-mêmes.
func keyFromOutputURI(outputURI, bucket string) (string, error) {
	u, err := url.Parse(outputURI)
	if err != nil {
		return "", fmt.Errorf("awspolly: parse OutputUri %q: %w", outputURI, err)
	}
	if prefix := "/" + bucket + "/"; strings.HasPrefix(u.Path, prefix) {
		return strings.TrimPrefix(u.Path, prefix), nil
	}
	if strings.HasPrefix(u.Host, bucket+".") {
		return strings.TrimPrefix(u.Path, "/"), nil
	}
	return "", fmt.Errorf("awspolly: OutputUri %q does not reference bucket %q", outputURI, bucket)
}

// pollTimeout borne l'attente d'une tâche Polly (audio ou marks). Sans ça,
// le ctx qui atteint runTask est celui, process-lifetime, du worker
// (signal.NotifyContext dans cmd/worker/main.go) -- une tâche qui n'atteint
// jamais un état terminal bloquerait le worker indéfiniment (traitement
// strictement séquentiel, un seul handle() à la fois). 20 minutes : large
// marge au-dessus de la plus longue narration observée sur le corpus
// (11 378 caractères). Un timeout ici redescend comme une erreur simple
// (non permanente), que le retry existant de worker.go (maxTTSAttempts)
// gère déjà correctement. Remplace le &http.Client{Timeout: 5 * time.Minute}
// de l'ancien adaptateur ElevenLabs, jamais réinstauré quand l'appel
// synchrone a été remplacé par ce polling asynchrone.
const pollTimeout = 20 * time.Minute

// defaultSpeechRate pilote le débit de parole via SSML -- 90% (10% plus lent
// que le débit par défaut du moteur Neural), retenu après un premier test
// réel jugé "parle trop vite" (Cristo Redentor FR, 26/08).
const defaultSpeechRate = "90%"

// speechRateByLanguage surcharge defaultSpeechRate par langue -- nécessaire
// depuis qu'un même pourcentage se lit différemment selon la voix Neural
// utilisée : 90% (réglé sur la voix française Remi) a été jugé trop lent une
// fois entendu sur la voix portugaise Vitória (test réel, 14/09). Seules les
// langues qui ont besoin d'un réglage différent du défaut apparaissent ici.
var speechRateByLanguage = map[string]string{
	"pt": "100%",
}

func speechRateFor(language string) string {
	if rate, ok := speechRateByLanguage[language]; ok {
		return rate
	}
	return defaultSpeechRate
}

// xmlEscaper échappe le minimum requis par SSML (&, <, >) -- le texte source
// est de la prose ordinaire (ponctuation, guillemets français), donc le
// risque est faible, mais un "&" ou un "<" littéral dans une narration
// casserait le XML sans cet échappement.
var xmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// wrapSSML enrobe le texte brut en SSML avec le débit configuré pour cette
// langue -- nécessaire pour piloter <prosody rate>, qu'un TextType=text en
// clair ne permet pas.
func wrapSSML(text, language string) string {
	return `<speak><prosody rate="` + speechRateFor(language) + `">` + xmlEscaper.Replace(text) + `</prosody></speak>`
}

func (g *Generator) Generate(ctx context.Context, text, language, voiceID string) (string, string, time.Duration, error) {
	code, err := languageCode(language)
	if err != nil {
		return "", "", 0, err
	}

	ctx, cancel := context.WithTimeout(ctx, pollTimeout)
	defer cancel()

	ssmlText := wrapSSML(text, language)
	var audioKey, marksKey string
	group, gctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		key, err := g.runTask(gctx, ssmlText, code, voiceID, types.OutputFormatMp3, nil, "audio/"+voiceID+"/")
		audioKey = key
		return err
	})
	group.Go(func() error {
		key, err := g.runTask(gctx, ssmlText, code, voiceID, types.OutputFormatJson, []types.SpeechMarkType{types.SpeechMarkTypeWord}, "timestamps/"+voiceID+"/")
		marksKey = key
		return err
	})
	if err := group.Wait(); err != nil {
		return "", "", 0, err
	}

	// Estimation de secours (durationFromMarks) sur le texte brut, pas le
	// SSML -- le compte de mots ne doit pas inclure les balises.
	duration := g.durationFromMarks(ctx, marksKey, text)
	return "s3://" + g.bucket + "/" + audioKey, "s3://" + g.bucket + "/" + marksKey, duration, nil
}

type wordMark struct {
	Time int64  `json:"time"`
	Type string `json:"type"`
}

// durationFromMarks lit le fichier NDJSON de marks juste écrit par Polly et
// prend le "time" (ms) du dernier mot -- plus précis que l'estimation par
// nombre de mots utilisée par le stub d'origine et par ElevenLabs. Ne
// remonte jamais d'erreur : un fichier de marks illisible ne doit pas faire
// échouer toute la génération, juste dégrader la précision de la durée.
func (g *Generator) durationFromMarks(ctx context.Context, marksKey, fallbackText string) time.Duration {
	out, err := g.s3.GetObject(ctx, &s3.GetObjectInput{Bucket: &g.bucket, Key: &marksKey})
	if err != nil {
		return estimateDuration(fallbackText)
	}
	defer func() { _ = out.Body.Close() }()

	var lastTime int64
	scanner := bufio.NewScanner(out.Body)
	for scanner.Scan() {
		var mark wordMark
		if err := json.Unmarshal(scanner.Bytes(), &mark); err != nil {
			continue
		}
		if mark.Type == "word" && mark.Time > lastTime {
			lastTime = mark.Time
		}
	}
	if scanner.Err() != nil || lastTime <= 0 {
		return estimateDuration(fallbackText)
	}
	return time.Duration(lastTime) * time.Millisecond
}

// estimateDuration : même formule que l'estimation ElevenLabs/stub -- ~5
// caractères par mot, ~400ms par mot, plancher à 1 mot pour éviter une durée
// nulle que domain.NewGeneratedAudio rejette.
func estimateDuration(text string) time.Duration {
	wordCount := max(1, len(text)/5)
	return time.Duration(wordCount) * 400 * time.Millisecond
}
